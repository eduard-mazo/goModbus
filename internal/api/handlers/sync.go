package handlers

import (
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"goModbus/internal/config"
	idb "goModbus/internal/db"
	"goModbus/internal/logger"
	"goModbus/internal/modbus"
)

// DB is injected from main.go after the database is opened.
var DB *sql.DB

// ─── Task model ──────────────────────────────────────────────────────────────

type SyncRequest struct {
	Stations []string `json:"stations"`
}

// syncTask is a flat, self-contained description of one sync unit.
// A station with N medidores produces N tasks; a single-meter station produces 1.
type syncTask struct {
	Key     string // "STATION" or "STATION / M1"
	Station string // parent station name
	IP      string
	Port    int
	UnitID  byte
	Endian  modbus.Endianness
	PtrAddr uint16
	DBAddr  uint16
}

// expandTasks converts filtered StationConfigs into syncTasks, expanding
// multi-medidor stations into one task per medidor.
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

// ─── Full sync handler ────────────────────────────────────────────────────────

// FullSyncHandler starts a background smart-delta sync and returns immediately.
// Tasks for the same IP run sequentially (ROC devices accept ≤2 TCP connections);
// tasks for different IPs run in parallel.
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
		// Group tasks by IP — tasks for the same device must run sequentially
		// because ROC field units only allow a limited number of simultaneous
		// Modbus TCP connections (typically 2, but practically 1 per session).
		tasksByIP := make(map[string][]syncTask)
		for _, t := range tasks {
			tasksByIP[t.IP] = append(tasksByIP[t.IP], t)
		}

		var wg sync.WaitGroup
		for _, group := range tasksByIP {
			wg.Add(1)
			go func(grp []syncTask) {
				defer wg.Done()
				for _, t := range grp { // sequential within same device
					syncStation(sid, t)
				}
			}(group)
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

// ─── Core sync function ───────────────────────────────────────────────────────

const syncTotal = 840

// syncStation performs a smart delta-sync for one task:
//  1. Load cached records from DB.
//  2. Connect and read the current pointer from the device.
//  3. Determine which pointers need fetching (new since last sync + failed).
//  4. Fetch only the needed pointers using a 2-worker pool.
//  5. Persist to DB and return the full 840-record set.
func syncStation(sid string, task syncTask) {
	start := time.Now()

	// ── 1. Load cached records ────────────────────────────────────────────────
	cached := map[int]idb.StationRecord{}
	if DB != nil {
		if m, err := idb.GetTaskRecords(DB, task.Key); err == nil {
			cached = m
		}
	}

	announceProgress(sid, task, 0, len(deltaPtrs(cached, -1)), "conectando…")

	// ── 2. Connect to device ──────────────────────────────────────────────────
	client := modbus.NewModbusClient(task.IP, task.Port, task.UnitID, task.Endian)
	client.Silent = true
	if err := client.Connect(); err != nil {
		if len(cached) > 0 {
			records := mergeRecords(cached, nil)
			sendFinal(sid, task, records, "", start, "caché (sin conexión)")
		} else {
			sendFinal(sid, task, nil, "Conexión fallida: "+err.Error(), start, "")
		}
		return
	}
	defer client.Close()

	// ── 3. Read current pointer from device ───────────────────────────────────
	currentPtr := -1
	if ptrData, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, task.PtrAddr, 1, nil); err == nil && len(ptrData) >= 2 {
		v := int(binary.BigEndian.Uint16(ptrData[0:2]))
		if v >= 0 && v < syncTotal {
			currentPtr = v
		}
	}

	// ── 4. Determine which pointers to fetch ──────────────────────────────────
	ptrs := deltaPtrs(cached, currentPtr)
	if len(ptrs) == 0 {
		records := mergeRecords(cached, nil)
		sendFinal(sid, task, records, "", start, "caché al día")
		return
	}

	// ── 5. Fetch using 2 workers ──────────────────────────────────────────────
	fresh := make(map[int]idb.StationRecord, len(ptrs))
	var mu sync.Mutex
	var done int32

	type job struct{ ptr int }
	jobs := make(chan job, len(ptrs))
	for _, p := range ptrs {
		jobs <- job{p}
	}
	close(jobs)

	total := len(ptrs)
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				data, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, task.DBAddr, uint16(j.ptr), nil)
				rec := idb.StationRecord{Ptr: j.ptr}
				if err == nil {
					rec.Valid = true
					rec.Hex = fmt.Sprintf("%X", data)
					if len(data) >= 4 {
						rec.DateRaw = binary.BigEndian.Uint16(data[0:2])
						rec.TimeRaw = binary.BigEndian.Uint16(data[2:4])
					}
				}
				mu.Lock()
				fresh[j.ptr] = rec
				mu.Unlock()

				n := atomic.AddInt32(&done, 1)
				if n%5 == 0 || int(n) == total {
					announceProgress(sid, task, int(n), total, "")
				}
			}
		}()
	}
	wg.Wait()

	// ── 6. Persist to DB ──────────────────────────────────────────────────────
	if DB != nil {
		batch := make([]idb.StationRecord, 0, len(fresh))
		for _, r := range fresh {
			batch = append(batch, r)
		}
		_ = idb.UpsertRecords(DB, task.Key, batch)
	}

	// ── 7. Build full 840-record response ─────────────────────────────────────
	records := mergeRecords(cached, fresh)
	cached_n := len(cached)
	sendFinal(sid, task, records, "", start,
		fmt.Sprintf("%d nuevos, %d en caché", len(fresh), cached_n))
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// deltaPtrs returns the sorted list of pointer indices that must be fetched.
// It includes pointers new since the last sync and previously-failed pointers.
// If currentPtr < 0 (unreadable) or cached is empty, all 840 are returned.
func deltaPtrs(cached map[int]idb.StationRecord, currentPtr int) []int {
	if len(cached) == 0 || currentPtr < 0 {
		all := make([]int, syncTotal)
		for i := range all {
			all[i] = i
		}
		return all
	}

	maxValid := -1
	var failed []int
	for ptr, r := range cached {
		if r.Valid && ptr > maxValid {
			maxValid = ptr
		}
		if !r.Valid {
			failed = append(failed, ptr)
		}
	}

	if maxValid < 0 {
		all := make([]int, syncTotal)
		for i := range all {
			all[i] = i
		}
		return all
	}

	var newPtrs []int
	if currentPtr >= maxValid {
		for p := maxValid + 1; p <= currentPtr; p++ {
			newPtrs = append(newPtrs, p)
		}
	} else {
		// Circular buffer wrapped around
		for p := maxValid + 1; p < syncTotal; p++ {
			newPtrs = append(newPtrs, p)
		}
		for p := 0; p <= currentPtr; p++ {
			newPtrs = append(newPtrs, p)
		}
	}

	seen := map[int]bool{}
	var result []int
	for _, p := range append(newPtrs, failed...) {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	sort.Ints(result)
	return result
}

// mergeRecords combines cached + fresh into the full 840-slot HourRecord slice.
// fresh overrides cached for the same pointer.
func mergeRecords(cached, fresh map[int]idb.StationRecord) []modbus.HourRecord {
	records := make([]modbus.HourRecord, syncTotal)
	for p := 0; p < syncTotal; p++ {
		rec := modbus.HourRecord{Hour: p / 10, Ptr: uint16(p)}
		var sr idb.StationRecord
		var ok bool
		if fresh != nil {
			sr, ok = fresh[p]
		}
		if !ok {
			sr, ok = cached[p]
		}
		if ok {
			rec.Valid = sr.Valid
			rec.Hex = sr.Hex
			rec.DateRaw = sr.DateRaw
			rec.TimeRaw = sr.TimeRaw
			if sr.Valid && sr.Hex != "" {
				raw := hexDecode(sr.Hex)
				payload := raw
				if len(raw) >= 4 {
					payload = raw[4:]
				}
				rec.Modes = modbus.DecodeAllModes(payload)
				for i := range rec.Modes {
					rec.Modes[i].Sanitize()
				}
			}
		}
		records[p] = rec
	}
	return records
}

func hexDecode(s string) []byte {
	b, _ := hex.DecodeString(s)
	return b
}

func announceProgress(sid string, task syncTask, done, total int, note string) {
	pct := 0
	if total > 0 {
		pct = done * 100 / total
	}
	msg := fmt.Sprintf("%s: %d/%d (%d%%)", task.Key, done, total, pct)
	if note != "" {
		msg = fmt.Sprintf("%s: %s", task.Key, note)
	}
	logger.SessionBroadcast(sid, logger.LogMessage{
		Level:   "SYNC",
		Message: msg,
		Progress: &logger.SyncProgress{
			Station: task.Key,
			Done:    done,
			Total:   total,
			Pct:     pct,
		},
	})
}

func sendFinal(sid string, task syncTask, records []modbus.HourRecord, errStr string, start time.Time, note string) {
	elapsed := time.Since(start).Milliseconds()
	prog := &logger.SyncProgress{
		Station: task.Key,
		Done:    syncTotal,
		Total:   syncTotal,
		Pct:     100,
		Records: records,
	}
	if errStr != "" {
		prog.Error = errStr
		prog.Done = 0
		prog.Pct = 0
		prog.Records = nil
	}
	msg := fmt.Sprintf("Sync: %s (%d ms)", task.Key, elapsed)
	if note != "" {
		msg += " — " + note
	}
	logger.SessionBroadcast(sid, logger.LogMessage{
		Level:    "INFO",
		Message:  msg,
		Progress: prog,
	})
}

// ─── Partial sync ─────────────────────────────────────────────────────────────

type PartialSyncRequest struct {
	TaskKey   string            `json:"task_key"`
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

	var dbBatch []idb.StationRecord
	records := make([]modbus.HourRecord, 0, len(req.Pointers))
	for _, ptr := range req.Pointers {
		data, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, req.DBAddress, ptr, nil)
		rec := modbus.HourRecord{Hour: int(ptr / 10), Ptr: ptr}
		sr := idb.StationRecord{Ptr: int(ptr)}
		if err == nil {
			rec.Valid = true
			rec.Hex = fmt.Sprintf("%X", data)
			sr.Valid = true
			sr.Hex = rec.Hex
			payload := data
			if len(data) >= 4 {
				rec.DateRaw = binary.BigEndian.Uint16(data[0:2])
				rec.TimeRaw = binary.BigEndian.Uint16(data[2:4])
				sr.DateRaw = rec.DateRaw
				sr.TimeRaw = rec.TimeRaw
				payload = data[4:]
			}
			rec.Modes = modbus.DecodeAllModes(payload)
			for i := range rec.Modes {
				rec.Modes[i].Sanitize()
			}
		}
		records = append(records, rec)
		dbBatch = append(dbBatch, sr)
	}

	if DB != nil && req.TaskKey != "" {
		_ = idb.UpsertRecords(DB, req.TaskKey, dbBatch)
	}

	c.JSON(http.StatusOK, gin.H{"records": records})
}
