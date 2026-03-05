package handlers

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"goModbus/internal/config"
	"goModbus/internal/logger"
	"goModbus/internal/modbus"
)

type SyncRequest struct {
	Stations []string `json:"stations"`
}

type StationSyncResult struct {
	Station string              `json:"station"`
	IP      string              `json:"ip"`
	Records []modbus.HourRecord `json:"records"`
	Elapsed int64               `json:"elapsed_ms"`
	Error   string              `json:"error,omitempty"`
}

// FullSyncHandler starts a background sync and returns immediately.
// Progress and results are delivered to the requesting client via WebSocket
// using session-scoped messages (SID from X-Session-ID header).
func FullSyncHandler(c *gin.Context) {
	var req SyncRequest
	_ = c.ShouldBindJSON(&req)

	sid := c.GetHeader("X-Session-ID")

	cfg, _ := config.LoadConfig(config.ConfigPath)
	var filtered []config.StationConfig
	if len(req.Stations) > 0 {
		for _, name := range req.Stations {
			for _, s := range cfg.Stations {
				if s.Name == name {
					filtered = append(filtered, s)
					break
				}
			}
		}
	} else {
		filtered = cfg.Stations
	}

	if len(filtered) == 0 {
		c.JSON(http.StatusOK, gin.H{"error": "no hay estaciones seleccionadas"})
		return
	}

	// Fire-and-forget: respond immediately, stream progress over WS
	go func() {
		var wg sync.WaitGroup
		for _, st := range filtered {
			wg.Add(1)
			go func(s config.StationConfig) {
				defer wg.Done()
				syncStation(sid, s)
			}(st)
		}
		wg.Wait()
		logger.SessionBroadcast(sid, logger.LogMessage{
			Level:   "INFO",
			Message: fmt.Sprintf("Sync global completado — %d estaciones", len(filtered)),
			Progress: &logger.SyncProgress{
				Station: "__done__",
				Done:    len(filtered),
				Total:   len(filtered),
				Pct:     100,
			},
		})
	}()

	c.JSON(http.StatusOK, gin.H{"status": "started", "stations": len(filtered)})
}

// syncStation executes a full 840-record download for one station.
func syncStation(sid string, s config.StationConfig) {
	const total = 840
	const progressStep = 42 // ~5% per update

	records := make([]modbus.HourRecord, total)
	var doneCount int32

	type job struct{ ptr int }
	jobs := make(chan job, total)
	for p := 0; p < total; p++ {
		jobs <- job{p}
	}
	close(jobs)

	stStart := time.Now()
	var connErr string
	var mu sync.Mutex

	var wgWorkers sync.WaitGroup
	for w := 0; w < 2; w++ {
		wgWorkers.Add(1)
		go func() {
			defer wgWorkers.Done()
			client := modbus.NewModbusClient(s.IP, s.Port, s.ID, s.Endian)
			client.Silent = true
			if err := client.Connect(); err != nil {
				mu.Lock()
				connErr = "Conexión fallida: " + err.Error()
				mu.Unlock()
				for range jobs {
				}
				return
			}
			defer client.Close()

			for j := range jobs {
				data, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, s.DBAddress, uint16(j.ptr), nil)
				rec := modbus.HourRecord{Hour: j.ptr / 10, Ptr: uint16(j.ptr)}
				if err == nil {
					rec.Valid = true
					rec.Hex = fmt.Sprintf("%X", data)
					payload := data
					if len(data) >= 4 {
						payload = data[4:]
					}
					rec.Modes = modbus.DecodeAllModes(payload)
					for i := range rec.Modes {
						rec.Modes[i].Sanitize()
					}
					floats := client.DecodeFloat32(payload)
					if len(floats) > 0 {
						rec.Value = modbus.SanitizeFloat(floats[0])
					}
				}
				records[j.ptr] = rec

				n := atomic.AddInt32(&doneCount, 1)
				if n%progressStep == 0 || int(n) == total {
					pct := int(n) * 100 / total
					logger.SessionBroadcast(sid, logger.LogMessage{
						Level:   "SYNC",
						Message: fmt.Sprintf("%s: %d/%d (%d%%)", s.Name, n, total, pct),
						Progress: &logger.SyncProgress{
							Station: s.Name,
							Done:    int(n),
							Total:   total,
							Pct:     pct,
						},
					})
				}
			}
		}()
	}
	wgWorkers.Wait()

	elapsed := time.Since(stStart).Milliseconds()
	prog := &logger.SyncProgress{
		Station: s.Name,
		Done:    total,
		Total:   total,
		Pct:     100,
		Records: records,
	}
	if connErr != "" {
		prog.Error = connErr
		prog.Done = 0
		prog.Pct = 0
		prog.Records = nil
	}
	logger.SessionBroadcast(sid, logger.LogMessage{
		Level:    "INFO",
		Message:  fmt.Sprintf("Sync Completo: %s (%d ms)", s.Name, elapsed),
		Progress: prog,
	})
	logger.BroadcastLog("INFO", fmt.Sprintf("Sync Completo: %s (%d ms)", s.Name, elapsed), nil, 0, nil, "")
}

type PartialSyncRequest struct {
	IP        string            `json:"ip"`
	Port      int               `json:"port"`
	ID        byte              `json:"slave_id"`
	Endian    modbus.Endianness `json:"endian"`
	DBAddress uint16            `json:"db_address"`
	Pointers  []uint16          `json:"pointers"`
}

func PartialSyncHandler(c *gin.Context) {
	var req PartialSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Pointers) == 0 {
		c.JSON(http.StatusOK, gin.H{"records": []modbus.HourRecord{}})
		return
	}

	client := modbus.NewModbusClient(req.IP, req.Port, req.ID, req.Endian)
	if err := client.Connect(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer client.Close()

	records := make([]modbus.HourRecord, 0, len(req.Pointers))
	for _, ptr := range req.Pointers {
		data, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, req.DBAddress, ptr, nil)
		rec := modbus.HourRecord{Hour: int(ptr / 10), Ptr: ptr}
		if err == nil {
			rec.Valid = true
			rec.Hex = fmt.Sprintf("%X", data)
			payload := data
			if len(data) >= 4 {
				payload = data[4:]
			}
			rec.Modes = modbus.DecodeAllModes(payload)
			for i := range rec.Modes {
				rec.Modes[i].Sanitize()
			}
			floats := client.DecodeFloat32(payload)
			if len(floats) > 0 {
				rec.Value = modbus.SanitizeFloat(floats[0])
			}
		}
		records = append(records, rec)
	}

	c.JSON(http.StatusOK, gin.H{"records": records})
}
