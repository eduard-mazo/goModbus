package db

import "database/sql"

// StationRecord is one ROC circular-buffer record cached in SQLite.
// Used for delta-sync tracking (primary key: task_key + ptr).
type StationRecord struct {
	Ptr    int
	Fecha  string // "YYYY-MM-DD" decoded from float Modes[0]
	Hora   string // "HH:MM"      decoded from float Modes[1]
	Hex    string // data payload bytes as hex
	RawHex string // full Modbus ADU as hex
	Valid  bool
}

// HistoryRecord is one entry in the long-term history table.
// Primary key: (task_key, fecha, hora) — allows storing data beyond 840 slots.
type HistoryRecord struct {
	Ptr    int
	Fecha  string
	Hora   string
	Hex    string
	RawHex string
}

// TaskMeta stores the circular-buffer reference point recorded at sync time.
type TaskMeta struct {
	TaskKey string
	RefPtr  int
	RefTime int64 // unix seconds
}

// ─── station_records (delta tracking, 840-slot circular-buffer mirror) ────────

// GetTaskRecords returns all cached records for taskKey keyed by ptr (0-839).
func GetTaskRecords(database *sql.DB, taskKey string) (map[int]StationRecord, error) {
	rows, err := database.Query(
		`SELECT ptr, COALESCE(fecha,''), COALESCE(hora,''), hex, COALESCE(raw_hex,''), valid
		 FROM station_records WHERE task_key = ?`, taskKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[int]StationRecord, 840)
	for rows.Next() {
		var r StationRecord
		var v int
		if err := rows.Scan(&r.Ptr, &r.Fecha, &r.Hora, &r.Hex, &r.RawHex, &v); err != nil {
			return nil, err
		}
		r.Valid = v != 0
		m[r.Ptr] = r
	}
	return m, rows.Err()
}

// UpsertRecords persists a batch of records for taskKey (delta tracking table).
func UpsertRecords(database *sql.DB, taskKey string, records []StationRecord) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO station_records (task_key, ptr, fecha, hora, hex, raw_hex, valid, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(task_key, ptr) DO UPDATE SET
			fecha     = excluded.fecha,
			hora      = excluded.hora,
			hex       = excluded.hex,
			raw_hex   = excluded.raw_hex,
			valid     = excluded.valid,
			synced_at = CURRENT_TIMESTAMP`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range records {
		v := 0
		if r.Valid {
			v = 1
		}
		rawHex := r.RawHex
		if rawHex == "" {
			rawHex = r.Hex
		}
		if _, err := stmt.Exec(taskKey, r.Ptr, r.Fecha, r.Hora, r.Hex, rawHex, v); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ─── station_history (unlimited long-term storage) ────────────────────────────

// GetHistory returns all history records for taskKey ordered chronologically.
func GetHistory(database *sql.DB, taskKey string) ([]HistoryRecord, error) {
	rows, err := database.Query(
		`SELECT ptr, fecha, hora, hex, COALESCE(raw_hex,'')
		 FROM station_history WHERE task_key = ?
		 ORDER BY fecha ASC, hora ASC`, taskKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryRecord
	for rows.Next() {
		var r HistoryRecord
		if err := rows.Scan(&r.Ptr, &r.Fecha, &r.Hora, &r.Hex, &r.RawHex); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertHistory inserts valid records into the long-term history table.
// Records with empty fecha or hora are skipped (invalid records are not stored here).
func UpsertHistory(database *sql.DB, taskKey string, records []StationRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO station_history (task_key, ptr, fecha, hora, hex, raw_hex, valid, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(task_key, fecha, hora) DO UPDATE SET
			ptr       = excluded.ptr,
			hex       = excluded.hex,
			raw_hex   = excluded.raw_hex,
			synced_at = CURRENT_TIMESTAMP`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range records {
		if !r.Valid || r.Fecha == "" || r.Hora == "" {
			continue
		}
		rawHex := r.RawHex
		if rawHex == "" {
			rawHex = r.Hex
		}
		if _, err := stmt.Exec(taskKey, r.Ptr, r.Fecha, r.Hora, r.Hex, rawHex); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// GetLastHistoryTime returns the most recent (fecha, hora) from station_history
// for taskKey, ordered lexicographically (YYYY-MM-DD HH:MM sorts correctly).
func GetLastHistoryTime(database *sql.DB, taskKey string) (fecha, hora string, err error) {
	row := database.QueryRow(
		`SELECT fecha, hora FROM station_history WHERE task_key = ?
		 ORDER BY fecha DESC, hora DESC LIMIT 1`, taskKey,
	)
	err = row.Scan(&fecha, &hora)
	return
}

// ─── task_meta ────────────────────────────────────────────────────────────────

func GetTaskMeta(database *sql.DB, taskKey string) (*TaskMeta, error) {
	row := database.QueryRow(
		`SELECT ref_ptr, ref_time FROM task_meta WHERE task_key = ?`, taskKey,
	)
	var m TaskMeta
	m.TaskKey = taskKey
	if err := row.Scan(&m.RefPtr, &m.RefTime); err != nil {
		return nil, err
	}
	return &m, nil
}

func UpsertTaskMeta(database *sql.DB, m TaskMeta) error {
	_, err := database.Exec(`
		INSERT INTO task_meta (task_key, ref_ptr, ref_time) VALUES (?, ?, ?)
		ON CONFLICT(task_key) DO UPDATE SET ref_ptr=excluded.ref_ptr, ref_time=excluded.ref_time`,
		m.TaskKey, m.RefPtr, m.RefTime,
	)
	return err
}
