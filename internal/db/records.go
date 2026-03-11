package db

import "database/sql"

// StationRecord is one ROC circular-buffer record cached in SQLite.
// Hex holds the raw Modbus data payload bytes as hex string.
// RawHex holds the reconstructed full Modbus ADU (including MBAP header) as hex.
type StationRecord struct {
	Ptr    int
	Hex    string // data payload bytes (data after MBAP+FC+ByteCount)
	RawHex string // full Modbus response frame (MBAP + PDU)
	Valid  bool
}

// TaskMeta stores the circular-buffer reference point recorded at sync time.
// RefPtr is the device's current pointer at the moment of the sync.
// RefTime is the corresponding Unix timestamp (seconds) — from device clock or server time.
type TaskMeta struct {
	TaskKey string
	RefPtr  int
	RefTime int64
}

// GetTaskRecords returns all cached records for taskKey, keyed by ptr (0-839).
func GetTaskRecords(database *sql.DB, taskKey string) (map[int]StationRecord, error) {
	rows, err := database.Query(
		`SELECT ptr, hex, COALESCE(raw_hex,''), valid FROM station_records WHERE task_key = ?`,
		taskKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[int]StationRecord, 840)
	for rows.Next() {
		var r StationRecord
		var v int
		if err := rows.Scan(&r.Ptr, &r.Hex, &r.RawHex, &v); err != nil {
			return nil, err
		}
		r.Valid = v != 0
		m[r.Ptr] = r
	}
	return m, rows.Err()
}

// UpsertRecords persists a batch of records for taskKey in a single transaction.
func UpsertRecords(database *sql.DB, taskKey string, records []StationRecord) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO station_records (task_key, ptr, date_raw, time_raw, hex, raw_hex, valid, synced_at)
		VALUES (?, ?, 0, 0, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(task_key, ptr) DO UPDATE SET
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
		if _, err := stmt.Exec(taskKey, r.Ptr, r.Hex, rawHex, v); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// GetTaskMeta loads the reference pointer and timestamp for a task key.
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

// UpsertTaskMeta persists (or updates) the reference point for a task.
func UpsertTaskMeta(database *sql.DB, m TaskMeta) error {
	_, err := database.Exec(`
		INSERT INTO task_meta (task_key, ref_ptr, ref_time) VALUES (?, ?, ?)
		ON CONFLICT(task_key) DO UPDATE SET ref_ptr=excluded.ref_ptr, ref_time=excluded.ref_time`,
		m.TaskKey, m.RefPtr, m.RefTime,
	)
	return err
}
