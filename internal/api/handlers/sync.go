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
// Tasks for the same IP share a per-IP semaphore (max 2 concurrent); tasks
// for different IPs run fully in parallel.
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
		// Per-IP semaphore: max 2 concurrent tasks per device.
		// ROC field units have limited simultaneous TCP connections;
		// allowing 2 gives parallelism without overwhelming the device.
		ipSems := make(map[string]chan struct{})
		for _, t := range tasks {
			if _, ok := ipSems[t.IP]; !ok {
				ipSems[t.IP] = make(chan struct{}, 2)
			}
		}

		var wg sync.WaitGroup
		for _, t := range tasks {
			t := t // capture loop variable
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem := ipSems[t.IP]
				sem <- struct{}{}        // acquire slot
				defer func() { <-sem }() // release slot
				syncStation(sid, t)
			}()
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
//  2. Connect and read the current pointer (+ device date/time if available).
//  3. Determine which pointers need fetching (new since last sync + failed).
//  4. Fetch only the needed pointers sequentially (1 worker, safe connection use).
//  5. Persist to DB and return the full 840-record set.
//
// ROC historical records are returned as raw float data bytes with NO embedded
// date/time header. Timestamps are computed on the frontend from RefPtr + RefTime.
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
			meta, _ := idb.GetTaskMeta(DB, task.Key)
			refPtr, refTime := -1, int64(0)
			if meta != nil {
				refPtr, refTime = meta.RefPtr, meta.RefTime
			}
			sendFinal(sid, task, records, "", start, "caché (sin conexión)", refPtr, refTime)
		} else {
			sendFinal(sid, task, nil, "Conexión fallida: "+err.Error(), start, "", -1, 0)
		}
		return
	}
	defer client.Close()

	// ── 3. Read current pointer (and optionally device date/time) ─────────────
	// Try reading 3 registers from PtrAddr: [ptr, date_raw, time_raw].
	// Many ROC 809 units place the current-record timestamp in the two registers
	// immediately following the history pointer register.
	currentPtr := -1
	refPtr := -1
	refTime := int64(0)

	if ptrData, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, task.PtrAddr, 3, nil); err == nil && len(ptrData) >= 6 {
		v := int(binary.BigEndian.Uint16(ptrData[0:2]))
		if v >= 0 && v < syncTotal {
			currentPtr = v
			refPtr = v
		}
		dateRaw := binary.BigEndian.Uint16(ptrData[2:4])
		timeRaw := binary.BigEndian.Uint16(ptrData[4:6])
		if t := decodeROCDateTime(dateRaw, timeRaw); t != nil {
			refTime = t.Unix()
		}
	} else if ptrData, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, task.PtrAddr, 1, nil); err == nil && len(ptrData) >= 2 {
		v := int(binary.BigEndian.Uint16(ptrData[0:2]))
		if v >= 0 && v < syncTotal {
			currentPtr = v
			refPtr = v
		}
	}

	// Fall back to server time (truncated to the hour) if device clock is unavailable.
	if refTime == 0 {
		refTime = time.Now().Truncate(time.Hour).Unix()
	}

	// ── 4. Determine which pointers to fetch ──────────────────────────────────
	ptrs := deltaPtrs(cached, currentPtr)
	if len(ptrs) == 0 {
		records := mergeRecords(cached, nil)
		persistMeta(task.Key, refPtr, refTime)
		sendFinal(sid, task, records, "", start, "caché al día", refPtr, refTime)
		return
	}

	// ── 5. Fetch using a single worker (sequential, safe on one connection) ───
	fresh := make(map[int]idb.StationRecord, len(ptrs))
	var done int32
	total := len(ptrs)

	for _, p := range ptrs {
		data, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, task.DBAddr, uint16(p), nil)
		rec := idb.StationRecord{Ptr: p}
		if err == nil {
			rec.Valid = true
			rec.Hex = fmt.Sprintf("%X", data)
			rec.RawHex = buildRawHex(task.UnitID, modbus.FCReadHoldingRegisters, data)
		}
		fresh[p] = rec

		n := atomic.AddInt32(&done, 1)
		if n%5 == 0 || int(n) == total {
			announceProgress(sid, task, int(n), total, "")
		}
	}

	// ── 6. Persist to DB ──────────────────────────────────────────────────────
	if DB != nil {
		batch := make([]idb.StationRecord, 0, len(fresh))
		for _, r := range fresh {
			batch = append(batch, r)
		}
		_ = idb.UpsertRecords(DB, task.Key, batch)
		persistMeta(task.Key, refPtr, refTime)
	}

	// ── 7. Build full 840-record response ─────────────────────────────────────
	records := mergeRecords(cached, fresh)
	sendFinal(sid, task, records, "", start,
		fmt.Sprintf("%d nuevos, %d en caché", len(fresh), len(cached)),
		refPtr, refTime)
}

// decodeROCDateTime interprets dateRaw and timeRaw using the ROC packed encoding:
//
//	dateRaw: bit[15:9]=year(+2000), bit[8:5]=month, bit[4:0]=day
//	timeRaw: bit[15:11]=hour, bit[10:5]=minute, bit[4:0]=second/2
//
// Returns nil if the decoded values are outside plausible ranges.
func decodeROCDateTime(dateRaw, timeRaw uint16) *time.Time {
	year := int((dateRaw>>9)&0x7F) + 2000
	month := int((dateRaw >> 5) & 0x0F)
	day := int(dateRaw & 0x1F)
	hour := int((timeRaw >> 11) & 0x1F)
	minute := int((timeRaw >> 5) & 0x3F)
	if month < 1 || month > 12 || day < 1 || day > 31 || year < 2020 || year > 2035 || hour > 23 || minute > 59 {
		return nil
	}
	t := time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)
	return &t
}

// buildRawHex reconstructs an approximate full Modbus TCP response frame hex string.
// TxID is set to 1 (approximate — exact value is not captured after the fact).
func buildRawHex(unitID, fc byte, data []byte) string {
	frame := make([]byte, 9+len(data))
	binary.BigEndian.PutUint16(frame[0:], 1)                      // transaction ID (approx)
	binary.BigEndian.PutUint16(frame[2:], 0)                      // protocol ID
	binary.BigEndian.PutUint16(frame[4:], uint16(2+len(data)))    // length
	frame[6] = unitID
	frame[7] = fc
	frame[8] = byte(len(data))
	copy(frame[9:], data)
	return fmt.Sprintf("%X", frame)
}

func persistMeta(taskKey string, refPtr int, refTime int64) {
	if DB != nil && refPtr >= 0 {
		_ = idb.UpsertTaskMeta(DB, idb.TaskMeta{TaskKey: taskKey, RefPtr: refPtr, RefTime: refTime})
	}
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
// The entire record data bytes are decoded as float32 signals (no date/time header).
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
			if sr.Valid && sr.Hex != "" {
				raw := hexDecode(sr.Hex)
				// All bytes are float32 data — no date/time header in ROC historical records.
				rec.Modes = modbus.DecodeAllModes(raw)
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

func sendFinal(sid string, task syncTask, records []modbus.HourRecord, errStr string, start time.Time, note string, refPtr int, refTime int64) {
	elapsed := time.Since(start).Milliseconds()
	prog := &logger.SyncProgress{
		Station: task.Key,
		Done:    syncTotal,
		Total:   syncTotal,
		Pct:     100,
		Records: records,
		RefPtr:  refPtr,
		RefTime: refTime,
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
			sr.RawHex = buildRawHex(req.ID, modbus.FCReadHoldingRegisters, data)
			rec.Modes = modbus.DecodeAllModes(data)
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

// ─── DB viewer endpoint ───────────────────────────────────────────────────────

// LoadFromDBHandler returns cached records from SQLite for the requested stations,
// without connecting to any device. Used by the frontend "Load from DB" feature.
func LoadFromDBHandler(c *gin.Context) {
	if DB == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "base de datos no disponible"})
		return
	}

	var req SyncRequest
	_ = c.ShouldBindJSON(&req)

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
	result := make(map[string]gin.H, len(tasks))

	for _, t := range tasks {
		cached, err := idb.GetTaskRecords(DB, t.Key)
		if err != nil || len(cached) == 0 {
			continue
		}
		meta, _ := idb.GetTaskMeta(DB, t.Key)
		refPtr, refTime := -1, int64(0)
		if meta != nil {
			refPtr, refTime = meta.RefPtr, meta.RefTime
		}
		records := mergeRecords(cached, nil)
		result[t.Key] = gin.H{
			"records":  records,
			"ref_ptr":  refPtr,
			"ref_time": refTime,
		}
	}

	c.JSON(http.StatusOK, result)
}
