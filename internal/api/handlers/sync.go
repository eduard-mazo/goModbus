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

// allPtrs returns every circular-buffer index (0..syncTotal-1).
func allPtrs() []int {
	ptrs := make([]int, syncTotal)
	for i := range ptrs {
		ptrs[i] = i
	}
	return ptrs
}

// timeDeltaPtrs calculates which circular-buffer ptrs are missing from the DB.
// It decodes the current pointer's record to get T_current, then compares with
// T_last (the most recent record in station_history) to find the hour delta.
func timeDeltaPtrs(lastFecha, lastHora string, currentPtr int, currentData []byte, endian modbus.Endianness) []int {
	// Decode the timestamp of the most recently written device record
	modes := modbus.DecodeAllModes(currentData)
	_, _, currentTS, ok := modbus.DecodeROCDateTime(modes, endian)
	if !ok || currentTS <= 0 {
		return allPtrs() // can't decode → full sync
	}

	// Parse the last DB record's timestamp
	lastT, err := time.ParseInLocation("2006-01-02 15:04", lastFecha+" "+lastHora, time.Local)
	if err != nil {
		return allPtrs()
	}

	deltaHours := int((currentTS - lastT.Unix()) / 3600)
	if deltaHours <= 0 {
		return nil // already up to date
	}
	if deltaHours >= syncTotal {
		return allPtrs() // more than one full rotation — full sync
	}

	// Generate the deltaHours ptrs ending at currentPtr (circular wrap)
	ptrs := make([]int, deltaHours)
	for i := 0; i < deltaHours; i++ {
		ptrs[i] = (currentPtr - deltaHours + 1 + i + syncTotal*10) % syncTotal
	}
	return ptrs
}

// DB is injected from main.go after the database is opened.
var DB *sql.DB

// ─── Task model ──────────────────────────────────────────────────────────────

type SyncRequest struct {
	Stations []string `json:"stations"`
}

type syncTask struct {
	Key                string // "STATION" or "STATION / M1"
	Station            string
	IP                 string
	Port               int
	UnitID             byte
	PtrEndian          modbus.Endianness // endian used to decode the pointer register
	DBEndian           modbus.Endianness // endian used to decode historical data
	PtrAddr            uint16
	DBAddr             uint16
	DataRegistersCount uint16 // 1 = pointer is uint16; 2 = pointer is float32 (2 regs)
}

func expandTasks(stations []config.StationConfig) []syncTask {
	var tasks []syncTask
	for _, s := range stations {
		// Resolve station-level endians (ptr/db may differ; fall back to Endian)
		stPtrEndian := s.PtrEndian
		if stPtrEndian == "" {
			stPtrEndian = s.Endian
		}
		stDBEndian := s.DBEndian
		if stDBEndian == "" {
			stDBEndian = s.Endian
		}
		drc := s.DataRegistersCount
		if drc == 0 {
			drc = 1
		}

		if len(s.Medidores) > 0 {
			for _, m := range s.Medidores {
				// Per-medidor endian overrides (if set in yaml)
				ptrEndian := stPtrEndian
				if m.PtrEndian != "" {
					ptrEndian = m.PtrEndian
				}
				dbEndian := stDBEndian
				if m.DBEndian != "" {
					dbEndian = m.DBEndian
				}
				tasks = append(tasks, syncTask{
					Key:                fmt.Sprintf("%s / %s", s.Name, m.Name),
					Station:            s.Name,
					IP:                 s.IP, Port: s.Port, UnitID: s.ID,
					PtrEndian:          ptrEndian,
					DBEndian:           dbEndian,
					PtrAddr:            m.PointerAddress,
					DBAddr:             m.DBAddress,
					DataRegistersCount: drc,
				})
			}
		} else {
			tasks = append(tasks, syncTask{
				Key:                s.Name,
				Station:            s.Name,
				IP:                 s.IP, Port: s.Port, UnitID: s.ID,
				PtrEndian:          stPtrEndian,
				DBEndian:           stDBEndian,
				PtrAddr:            s.PointerAddress,
				DBAddr:             s.DBAddress,
				DataRegistersCount: drc,
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

	// 1. Load cached station_records (for failed-ptr detection only)
	cached := map[int]idb.StationRecord{}
	if DB != nil {
		if m, err := idb.GetTaskRecords(DB, task.Key); err == nil {
			cached = m
		}
	}

	// 2. Get the latest timestamp already stored in station_history
	lastFecha, lastHora := "", ""
	hasHistory := false
	if DB != nil {
		if f, h, err := idb.GetLastHistoryTime(DB, task.Key); err == nil && f != "" {
			lastFecha, lastHora = f, h
			hasHistory = true
		}
	}

	announceProgress(sid, task, 0, syncTotal, "conectando…")

	// 3. Connect to device (endian is per-read: PtrEndian for ptr, DBEndian for data)
	client := modbus.NewModbusClient(task.IP, task.Port, task.UnitID, task.DBEndian)
	client.Silent = true // suppress per-frame INFO; errors always logged
	client.SID = sid     // route logs to the requesting session only
	if err := client.Connect(); err != nil {
		if hasHistory {
			records := historyRecords(task)
			sendFinal(sid, task, records, "", start, "caché (sin conexión)")
		} else {
			sendFinal(sid, task, nil, "Conexión fallida: "+err.Error(), start, "")
		}
		return
	}
	defer client.Close()

	// 4. Read current pointer using DataRegistersCount registers.
	//    qty=1 → uint16 pointer; qty=2 → float32 pointer (decoded with PtrEndian).
	currentPtr := -1
	ptrData, ptrReq, _, ptrErr := client.Execute(modbus.FCReadHoldingRegisters, task.PtrAddr, task.DataRegistersCount, nil)
	if ptrErr == nil {
		if task.DataRegistersCount >= 2 && len(ptrData) >= 4 {
			// Float32 pointer — decode with PtrEndian
			modes := modbus.DecodeAllModes(ptrData)
			if len(modes) > 0 {
				f := modes[0].Pick(task.PtrEndian)
				v := int(f)
				if f >= 0 && float32(v) == f && v < syncTotal {
					currentPtr = v
				}
			}
		} else if len(ptrData) >= 2 {
			// uint16 pointer — big-endian register
			v := int(binary.BigEndian.Uint16(ptrData[0:2]))
			if v >= 0 && v < syncTotal {
				currentPtr = v
			}
		}
		logger.SessionBroadcast(sid, logger.LogMessage{
			Level:   "DEBUG",
			Message: fmt.Sprintf("[%s] → %s:%d addr=%d qty=%d | TX: %X\n  ← ptr=%d | RX: %X", task.Key, task.IP, task.Port, task.PtrAddr, task.DataRegistersCount, ptrReq, currentPtr, ptrData),
		})
	} else {
		logger.SessionBroadcast(sid, logger.LogMessage{
			Level:   "ERROR",
			Message: fmt.Sprintf("[%s] → %s:%d addr=%d qty=%d ERROR: %v | TX: %X", task.Key, task.IP, task.Port, task.PtrAddr, task.DataRegistersCount, ptrErr, ptrReq),
		})
	}
	if currentPtr < 0 {
		if hasHistory {
			records := historyRecords(task)
			sendFinal(sid, task, records, "", start, "caché (no se pudo leer puntero)")
		} else {
			sendFinal(sid, task, nil, "No se pudo leer el puntero del equipo", start, "")
		}
		return
	}

	// 5. Read the record at currentPtr to get T_current (needed for delta calc)
	var currentPtrData []byte
	if currentPtr > 0 { // qty=0 is invalid Modbus; skip if ptr=0
		d, curReq, _, curErr := client.Execute(modbus.FCReadHoldingRegisters, task.DBAddr, uint16(currentPtr), nil)
		if curErr == nil {
			currentPtrData = d
			logger.SessionBroadcast(sid, logger.LogMessage{
				Level:   "DEBUG",
				Message: fmt.Sprintf("[%s] Registro ptr=%d leído | TX: %X | RX(%d): %X", task.Key, currentPtr, curReq, len(d), d),
			})
		} else {
			logger.SessionBroadcast(sid, logger.LogMessage{
				Level:   "ERROR",
				Message: fmt.Sprintf("[%s] Error leyendo registro ptr=%d addr=%d qty=%d | TX: %X", task.Key, currentPtr, task.DBAddr, currentPtr, curReq),
			})
		}
	} else {
		logger.SessionBroadcast(sid, logger.LogMessage{
			Level:   "WARN",
			Message: fmt.Sprintf("[%s] ptr=0 — omitiendo lectura de registro actual (qty=0 inválido), se hará full sync", task.Key),
		})
	}

	// 6. Compute which ptrs to fetch (time-based delta vs full sync)
	var ptrs []int
	if hasHistory && len(currentPtrData) > 0 {
		ptrs = timeDeltaPtrs(lastFecha, lastHora, currentPtr, currentPtrData, task.DBEndian)
	} else {
		ptrs = allPtrs() // no history or can't decode → full sync
	}

	// 7. Also add failed ptrs from station_records (retry)
	ptrSet := make(map[int]bool, len(ptrs))
	for _, p := range ptrs {
		ptrSet[p] = true
	}
	for ptr, r := range cached {
		if !r.Valid && !ptrSet[ptr] {
			ptrs = append(ptrs, ptr)
			ptrSet[ptr] = true
		}
	}
	sort.Ints(ptrs)

	if len(ptrs) == 0 {
		records := historyRecords(task)
		sendFinal(sid, task, records, "", start, "al día")
		return
	}

	// 8. Fetch sequentially
	fresh := make(map[int]idb.StationRecord, len(ptrs))
	var done int32
	total := len(ptrs)

	for _, p := range ptrs {
		var data []byte
		// Reuse the already-read record for currentPtr
		if p == currentPtr && len(currentPtrData) > 0 {
			data = currentPtrData
		} else if p == 0 {
			// qty=0 is invalid standard Modbus; log and skip
			logger.SessionBroadcast(sid, logger.LogMessage{
				Level:   "WARN",
				Message: fmt.Sprintf("[%s] ptr=0 omitido (qty=0 inválido en Modbus estándar)", task.Key),
			})
		} else {
			d, txFrame, _, err := client.Execute(modbus.FCReadHoldingRegisters, task.DBAddr, uint16(p), nil)
			if err == nil {
				data = d
			} else {
				logger.SessionBroadcast(sid, logger.LogMessage{
					Level:   "ERROR",
					Message: fmt.Sprintf("[%s] ptr=%d err: %v | TX: %X", task.Key, p, err, txFrame),
				})
			}
		}

		rec := idb.StationRecord{Ptr: p}
		if len(data) > 0 {
			rec.Valid = true
			rec.Hex = fmt.Sprintf("%X", data)
			rec.RawHex = buildRawHex(task.UnitID, modbus.FCReadHoldingRegisters, data)
			modes := modbus.DecodeAllModes(data)
			if fecha, hora, _, ok := modbus.DecodeROCDateTime(modes, task.DBEndian); ok {
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

	// 9. Persist to DB
	if DB != nil {
		batch := make([]idb.StationRecord, 0, len(fresh))
		for _, r := range fresh {
			batch = append(batch, r)
		}
		_ = idb.UpsertRecords(DB, task.Key, batch)
		_ = idb.UpsertHistory(DB, task.Key, batch)
		_ = idb.UpsertTaskMeta(DB, idb.TaskMeta{
			TaskKey: task.Key,
			RefPtr:  currentPtr,
			RefTime: time.Now().Unix(),
		})
	}

	// 10. Load full history (unlimited) and broadcast
	records := historyRecords(task)
	note := fmt.Sprintf("%d nuevos, ptr=%d", len(fresh), currentPtr)
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
		_, _, ts, _ := modbus.DecodeROCDateTime(modes, task.DBEndian)
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


// ─── Helpers ─────────────────────────────────────────────────────────────────


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
