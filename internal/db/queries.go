package db

import (
	"database/sql"
	"time"
)

// SaveSession inserts a new sync session and returns its ID.
func SaveSession(db *sql.DB, stations string, totalPtrs, okPtrs int) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO sync_sessions (started_at, finished_at, stations, total_ptrs, ok_ptrs)
		 VALUES (?, ?, ?, ?, ?)`,
		time.Now(), time.Now(), stations, totalPtrs, okPtrs,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SaveRecords bulk-inserts sync records for one session.
func SaveRecords(db *sql.DB, records []SyncRecord) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(
		`INSERT INTO sync_records (session_id, station, ptr, hour_label, valid, signals)
		 VALUES (?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		valid := 0
		if r.Valid {
			valid = 1
		}
		if _, err := stmt.Exec(r.SessionID, r.Station, r.Ptr, r.HourLabel, valid, r.Signals); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// GetSessionHistory returns the last N sync sessions (most recent first).
func GetSessionHistory(db *sql.DB, limit int) ([]SyncSession, error) {
	rows, err := db.Query(
		`SELECT id, started_at, finished_at, stations, total_ptrs, ok_ptrs
		 FROM sync_sessions ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []SyncSession
	for rows.Next() {
		var s SyncSession
		var finishedAt sql.NullTime
		if err := rows.Scan(&s.ID, &s.StartedAt, &finishedAt, &s.Stations, &s.TotalPtrs, &s.OkPtrs); err != nil {
			return nil, err
		}
		if finishedAt.Valid {
			s.FinishedAt = &finishedAt.Time
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// GetRecordsBySession returns all sync records for a given session ID.
func GetRecordsBySession(db *sql.DB, sessionID int64) ([]SyncRecord, error) {
	rows, err := db.Query(
		`SELECT id, session_id, station, ptr, hour_label, valid, signals
		 FROM sync_records WHERE session_id = ? ORDER BY ptr ASC`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []SyncRecord
	for rows.Next() {
		var r SyncRecord
		var valid int
		if err := rows.Scan(&r.ID, &r.SessionID, &r.Station, &r.Ptr, &r.HourLabel, &valid, &r.Signals); err != nil {
			return nil, err
		}
		r.Valid = valid == 1
		records = append(records, r)
	}
	return records, rows.Err()
}

// AppendQueryHistory records one generic Modbus query.
func AppendQueryHistory(db *sql.DB, h QueryHistory) error {
	_, err := db.Exec(
		`INSERT INTO query_history (queried_at, host, port, unit_id, fc, address, quantity, result_hex)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now(), h.Host, h.Port, h.UnitID, h.FC, h.Address, h.Quantity, h.ResultHex,
	)
	return err
}
