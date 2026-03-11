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

type syncTask struct {
	Key     string // "STATION" or "STATION / M1"
	Station string
	IP      string
	Port    int
	UnitID  byte
	Endian  modbus.Endianness
	PtrAddr uint16
	DBAddr  uint16
}

func expandTasks(stations []config.StationConfig) []syncTask {
	var tasks []syncTask
	for _, s := range stations {
		if len(s.Medidores) > 0 {
			for _, m := range s.Medidores {
				tasks = append(tasks, syncTask{
					Key:     fmt.Sprintf("%s / %s", s.Name, m.Name),
					Station: s.Name, IP: s.IP, Port: s.Port, UnitID: s.ID,
					Endian: s.Endian, PtrAddr: m.PointerAddress, DBAddr: m.DBAddress,
				})
			}
		} else {
			tasks = append(tasks, syncTask{
				Key:     s.Name,
				Station: s.Name, IP: s.IP, Port: s.Port, UnitID: s.ID,
				Endian: s.Endian, PtrAddr: s.PointerAddress, DBAddr: s.DBAddress,
			})
		}
	}
	return tasks
}

// ─── Full sync handler ────────────────────────────────────────────────────────

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
		// Per-IP semaphore: max 2 concurrent connections per device.
		ipSems := make(map[string]chan struct{})
		for _, t := range tasks {
			if _, ok := ipSems[t.IP]; !ok {
				ipSems[t.IP] = make(chan struct{}, 2)
			}
		}

		var wg sync.WaitGroup
		for _, t := range tasks {
			t := t
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem := ipSems[t.IP]
				sem <- struct{}{}
				defer func() { <-sem }()
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

// ─── Core sync ────────────────────────────────────────────────────────────────

const syncTotal = 840

func syncStation(sid string, task syncTask) {
	start := time.Now()

	// 1. Load cached records from DB (for delta tracking)
	cached := map[int]idb.StationRecord{}
	if DB != nil {
		if m, err := idb.GetTaskRecords(DB, task.Key); err == nil {
			cached = m
		}
	}

	announceProgress(sid, task, 0, len(deltaPtrs(cached, -1)), "conectando…")

	// 2. Connect to device
	client := modbus.NewModbusClient(task.IP, task.Port, task.UnitID, task.Endian)
	client.Silent = true
	if err := client.Connect(); err != nil {
		if len(cached) > 0 {
			records := historyRecords(task)
			sendFinal(sid, task, records, "", start, "caché (sin conexión)")
		} else {
			sendFinal(sid, task, nil, "Conexión fallida: "+err.Error(), start, "")
		}
		return
	}
	defer client.Close()

	// 3. Read current pointer
	currentPtr := -1
	if ptrData, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, task.PtrAddr, 1, nil); err == nil && len(ptrData) >= 2 {
		v := int(binary.BigEndian.Uint16(ptrData[0:2]))
		if v >= 0 && v < syncTotal {
			currentPtr = v
		}
	}

	// 4. Determine which pointers to fetch
	ptrs := deltaPtrs(cached, currentPtr)
	if len(ptrs) == 0 {
		records := historyRecords(task)
		sendFinal(sid, task, records, "", start, "caché al día")
		return
	}

	// 5. Fetch sequentially (1 worker — safe, no shared-connection race)
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
			// Decode date/time from first two float32 (ROC MMDDYY / HHMM format)
			modes := modbus.DecodeAllModes(data)
			if fecha, hora, _, ok := modbus.DecodeROCDateTime(modes, task.Endian); ok {
				rec.Fecha = fecha
				rec.Hora = hora
			}
		}
		fresh[p] = rec

		n := atomic.AddInt32(&done, 1)
		if n%5 == 0 || int(n) == total {
			announceProgress(sid, task, int(n), total, "")
		}
	}

	// 6. Persist to DB
	if DB != nil {
		batch := make([]idb.StationRecord, 0, len(fresh))
		for _, r := range fresh {
			batch = append(batch, r)
		}
		_ = idb.UpsertRecords(DB, task.Key, batch)
		_ = idb.UpsertHistory(DB, task.Key, batch)

		if currentPtr >= 0 {
			_ = idb.UpsertTaskMeta(DB, idb.TaskMeta{
				TaskKey: task.Key,
				RefPtr:  currentPtr,
				RefTime: time.Now().Unix(),
			})
		}
	}

	// 7. Re-decode cached records with fresh fecha/hora if missing
	for ptr, r := range cached {
		if _, inFresh := fresh[ptr]; !inFresh && r.Hex != "" && r.Fecha == "" {
			raw := hexDecodeStr(r.Hex)
			modes := modbus.DecodeAllModes(raw)
			if fecha, hora, _, ok := modbus.DecodeROCDateTime(modes, task.Endian); ok {
				r.Fecha = fecha
				r.Hora = hora
				cached[ptr] = r
			}
		}
	}

	// 8. Broadcast all history records (chronological, unlimited)
	records := buildHistory(cached, fresh, task.Endian)
	note := fmt.Sprintf("%d nuevos, %d en caché", len(fresh), len(cached))
	sendFinal(sid, task, records, "", start, note)
}

// historyRecords loads all long-term history from DB for broadcast.
func historyRecords(task syncTask) []modbus.HourRecord {
	if DB == nil {
		return nil
	}
	hist, err := idb.GetHistory(DB, task.Key)
	if err != nil || len(hist) == 0 {
		return nil
	}
	out := make([]modbus.HourRecord, 0, len(hist))
	for _, h := range hist {
		raw := hexDecodeStr(h.Hex)
		modes := modbus.DecodeAllModes(raw)
		for i := range modes {
			modes[i].Sanitize()
		}
		_, _, ts, _ := modbus.DecodeROCDateTime(modes, task.Endian)
		out = append(out, modbus.HourRecord{
			Ptr:   uint16(h.Ptr),
			Hex:   h.Hex,
			Modes: modes,
			Valid: true,
			Fecha: h.Fecha,
			Hora:  h.Hora,
			TS:    ts,
		})
	}
	return out
}

// buildHistory merges cached + fresh maps into a chronologically-sorted slice.
// fresh overrides cached for the same ptr. Only valid records with fecha/hora included.
func buildHistory(cached, fresh map[int]idb.StationRecord, endian modbus.Endianness) []modbus.HourRecord {
	// Merge: fresh wins over cached
	merged := make(map[int]idb.StationRecord, len(cached)+len(fresh))
	for ptr, r := range cached {
		merged[ptr] = r
	}
	for ptr, r := range fresh {
		merged[ptr] = r
	}

	// Collect valid records with fecha/hora
	var out []modbus.HourRecord
	for _, r := range merged {
		if !r.Valid || r.Fecha == "" || r.Hora == "" {
			continue
		}
		raw := hexDecodeStr(r.Hex)
		modes := modbus.DecodeAllModes(raw)
		for i := range modes {
			modes[i].Sanitize()
		}
		_, _, ts, _ := modbus.DecodeROCDateTime(modes, endian)
		out = append(out, modbus.HourRecord{
			Ptr:   uint16(r.Ptr),
			Hex:   r.Hex,
			Modes: modes,
			Valid: true,
			Fecha: r.Fecha,
			Hora:  r.Hora,
			TS:    ts,
		})
	}

	// Sort chronologically
	sort.Slice(out, func(i, j int) bool {
		if out[i].Fecha != out[j].Fecha {
			return out[i].Fecha < out[j].Fecha
		}
		return out[i].Hora < out[j].Hora
	})
	return out
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

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

func buildRawHex(unitID, fc byte, data []byte) string {
	frame := make([]byte, 9+len(data))
	binary.BigEndian.PutUint16(frame[0:], 1)
	binary.BigEndian.PutUint16(frame[2:], 0)
	binary.BigEndian.PutUint16(frame[4:], uint16(2+len(data)))
	frame[6] = unitID
	frame[7] = fc
	frame[8] = byte(len(data))
	copy(frame[9:], data)
	return fmt.Sprintf("%X", frame)
}

func hexDecodeStr(s string) []byte {
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
	msg := fmt.Sprintf("Sync: %s (%d ms) — %d registros hist.", task.Key, elapsed, len(records))
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
	var records []modbus.HourRecord
	for _, ptr := range req.Pointers {
		data, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, req.DBAddress, ptr, nil)
		rec := modbus.HourRecord{Ptr: ptr}
		sr := idb.StationRecord{Ptr: int(ptr)}
		if err == nil {
			rec.Valid = true
			rec.Hex = fmt.Sprintf("%X", data)
			sr.Valid = true
			sr.Hex = rec.Hex
			sr.RawHex = buildRawHex(req.ID, modbus.FCReadHoldingRegisters, data)
			modes := modbus.DecodeAllModes(data)
			for i := range modes {
				modes[i].Sanitize()
			}
			if fecha, hora, ts, ok := modbus.DecodeROCDateTime(modes, req.Endian); ok {
				rec.Fecha, rec.Hora, rec.TS = fecha, hora, ts
				sr.Fecha, sr.Hora = fecha, hora
			}
			rec.Modes = modes
		}
		records = append(records, rec)
		dbBatch = append(dbBatch, sr)
	}

	if DB != nil && req.TaskKey != "" {
		_ = idb.UpsertRecords(DB, req.TaskKey, dbBatch)
		_ = idb.UpsertHistory(DB, req.TaskKey, dbBatch)
	}

	c.JSON(http.StatusOK, gin.H{"records": records})
}

// ─── DB viewer endpoint ───────────────────────────────────────────────────────

// LoadFromDBHandler returns all history records from SQLite for the requested
// stations without connecting to any device.
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
		records := historyRecords(t)
		if len(records) == 0 {
			continue
		}
		result[t.Key] = gin.H{"records": records}
	}

	c.JSON(http.StatusOK, result)
}
