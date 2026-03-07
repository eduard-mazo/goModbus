package handlers

import (
	"encoding/binary"
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

// syncTask is a flat, self-contained description of one sync unit.
// A station with N medidores produces N tasks; a single-meter station produces 1.
type syncTask struct {
	Key     string            // display key used in progress events: "STATION" or "STATION / M1"
	Station string            // parent station name
	IP      string
	Port    int
	UnitID  byte
	Endian  modbus.Endianness
	PtrAddr uint16
	DBAddr  uint16
}

// expandTasks converts the filtered list of StationConfigs into syncTasks,
// expanding multi-medidor stations into one task per medidor.
func expandTasks(stations []config.StationConfig) []syncTask {
	var tasks []syncTask
	for _, s := range stations {
		if len(s.Medidores) > 0 {
			for _, m := range s.Medidores {
				tasks = append(tasks, syncTask{
					Key:     fmt.Sprintf("%s / %s", s.Name, m.Name),
					Station: s.Name,
					IP:      s.IP,
					Port:    s.Port,
					UnitID:  s.ID,
					Endian:  s.Endian,
					PtrAddr: m.PointerAddress,
					DBAddr:  m.DBAddress,
				})
			}
		} else {
			tasks = append(tasks, syncTask{
				Key:     s.Name,
				Station: s.Name,
				IP:      s.IP,
				Port:    s.Port,
				UnitID:  s.ID,
				Endian:  s.Endian,
				PtrAddr: s.PointerAddress,
				DBAddr:  s.DBAddress,
			})
		}
	}
	return tasks
}

// FullSyncHandler starts a background sync and returns immediately.
// Progress is streamed to the requesting client via WebSocket (SID-scoped).
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

	tasks := expandTasks(filtered)
	if len(tasks) == 0 {
		c.JSON(http.StatusOK, gin.H{"error": "no hay estaciones seleccionadas"})
		return
	}

	go func() {
		var wg sync.WaitGroup
		for _, t := range tasks {
			wg.Add(1)
			go func(task syncTask) {
				defer wg.Done()
				syncStation(sid, task)
			}(t)
		}
		wg.Wait()
		logger.SessionBroadcast(sid, logger.LogMessage{
			Level:   "INFO",
			Message: fmt.Sprintf("Sync global completado — %d tarea(s)", len(tasks)),
			Progress: &logger.SyncProgress{
				Station: "__done__",
				Done:    len(tasks),
				Total:   len(tasks),
				Pct:     100,
			},
		})
	}()

	c.JSON(http.StatusOK, gin.H{"status": "started", "tasks": len(tasks)})
}

// syncStation downloads all 840 records for one syncTask using a 2-worker pool.
func syncStation(sid string, task syncTask) {
	const total = 840
	const progressStep = 42 // emit progress every ~5%

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
			client := modbus.NewModbusClient(task.IP, task.Port, task.UnitID, task.Endian)
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
				data, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, task.DBAddr, uint16(j.ptr), nil)
				rec := modbus.HourRecord{Hour: j.ptr / 10, Ptr: uint16(j.ptr)}
				if err == nil {
					rec.Valid = true
					rec.Hex = fmt.Sprintf("%X", data)

					// Extract ROC date/time from first 4 bytes before signal payload
					payload := data
					if len(data) >= 4 {
						rec.DateRaw = binary.BigEndian.Uint16(data[0:2])
						rec.TimeRaw = binary.BigEndian.Uint16(data[2:4])
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
						Message: fmt.Sprintf("%s: %d/%d (%d%%)", task.Key, n, total, pct),
						Progress: &logger.SyncProgress{
							Station: task.Key,
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
		Station: task.Key,
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
		Message:  fmt.Sprintf("Sync Completo: %s (%d ms)", task.Key, elapsed),
		Progress: prog,
	})
}

// ─── Partial sync ────────────────────────────────────────────────────────────

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
				rec.DateRaw = binary.BigEndian.Uint16(data[0:2])
				rec.TimeRaw = binary.BigEndian.Uint16(data[2:4])
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
