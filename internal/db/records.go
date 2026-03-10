package db

import "database/sql"

// StationRecord is one ROC circular-buffer record cached in SQLite.
type StationRecord struct {
	Ptr     int
	DateRaw uint16
	TimeRaw uint16
	Hex     string
	Valid   bool
}

// GetTaskRecords returns all cached records for taskKey, keyed by ptr (0-839).
func GetTaskRecords(database *sql.DB, taskKey string) (map[int]StationRecord, error) {
	rows, err := database.Query(
		`SELECT ptr, date_raw, time_raw, hex, valid FROM station_records WHERE task_key = ?`,
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
		if err := rows.Scan(&r.Ptr, &r.DateRaw, &r.TimeRaw, &r.Hex, &v); err != nil {
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
		INSERT INTO station_records (task_key, ptr, date_raw, time_raw, hex, valid, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(task_key, ptr) DO UPDATE SET
			date_raw  = excluded.date_raw,
			time_raw  = excluded.time_raw,
			hex       = excluded.hex,
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
		if _, err := stmt.Exec(taskKey, r.Ptr, int(r.DateRaw), int(r.TimeRaw), r.Hex, v); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
