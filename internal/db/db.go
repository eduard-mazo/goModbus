package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open opens (or creates) the SQLite database at path and runs migrations.
func Open(path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("db: abrir %s: %w", path, err)
	}
	database.SetMaxOpenConns(1) // SQLite is single-writer
	if err := migrate(database); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

func migrate(db *sql.DB) error {
	// WAL mode: readers don't block writers; much better for concurrent HTTP requests.
	// synchronous=NORMAL: safe with WAL (no data loss on crash), faster than FULL.
	db.Exec(`PRAGMA journal_mode=WAL`)
	db.Exec(`PRAGMA synchronous=NORMAL`)

	// ── Core tables ────────────────────────────────────────────────────────────

	// station_records — circular-buffer mirror (max 840 rows/station).
	// PK (task_key, ptr). Used for delta-sync: compare stored fecha/hora
	// against the device's current pointer to know which slots to re-fetch.
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS station_records (
			task_key  TEXT     NOT NULL,
			ptr       INTEGER  NOT NULL,
			fecha     TEXT     NOT NULL DEFAULT '',
			hora      TEXT     NOT NULL DEFAULT '',
			hex       TEXT     NOT NULL DEFAULT '',
			raw_hex   TEXT     NOT NULL DEFAULT '',
			valid     INTEGER  NOT NULL DEFAULT 0,
			synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (task_key, ptr)
		)`)
	if err != nil {
		return fmt.Errorf("migrate station_records: %w", err)
	}

	// station_history — unlimited long-term storage.
	// PK (task_key, fecha, hora): one row per station-hour, accumulates forever.
	// dato1/dato2 = ROC float encoding of date/time (redundant but kept for SQL inspection).
	// dato3..dato10 = decoded signal values (S1=Min. Flujo Acum., …, S8=Energía).
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS station_history (
			task_key  TEXT     NOT NULL,
			ptr       INTEGER  NOT NULL DEFAULT 0,
			fecha     TEXT     NOT NULL,
			hora      TEXT     NOT NULL,
			hex       TEXT     NOT NULL DEFAULT '',
			raw_hex   TEXT     NOT NULL DEFAULT '',
			synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			dato1     REAL     NOT NULL DEFAULT 0,
			dato2     REAL     NOT NULL DEFAULT 0,
			dato3     REAL     NOT NULL DEFAULT 0,
			dato4     REAL     NOT NULL DEFAULT 0,
			dato5     REAL     NOT NULL DEFAULT 0,
			dato6     REAL     NOT NULL DEFAULT 0,
			dato7     REAL     NOT NULL DEFAULT 0,
			dato8     REAL     NOT NULL DEFAULT 0,
			dato9     REAL     NOT NULL DEFAULT 0,
			dato10    REAL     NOT NULL DEFAULT 0,
			PRIMARY KEY (task_key, fecha, hora)
		)`)
	if err != nil {
		return fmt.Errorf("migrate station_history: %w", err)
	}

	// task_meta — one row/station.
	// ref_ptr: device's circular-buffer pointer at sync time.
	// ref_time: unix seconds at sync time.
	// Frontend uses both to compute real timestamps:
	//   ts = (ref_time - (ref_ptr - ptr + 840) % 840 * 3600) * 1000 ms
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS task_meta (
			task_key  TEXT    PRIMARY KEY,
			ref_ptr   INTEGER NOT NULL DEFAULT -1,
			ref_time  INTEGER NOT NULL DEFAULT 0
		)`)
	if err != nil {
		return fmt.Errorf("migrate task_meta: %w", err)
	}

	// query_history — audit log for generic Modbus queries (tab "Consulta Modbus").
	// Populated by AppendQueryHistory (wired to handler in a future iteration).
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS query_history (
			id         INTEGER  PRIMARY KEY AUTOINCREMENT,
			queried_at DATETIME NOT NULL,
			host       TEXT,
			port       INTEGER,
			unit_id    INTEGER,
			fc         INTEGER,
			address    INTEGER,
			quantity   INTEGER,
			result_hex TEXT
		)`)
	if err != nil {
		return fmt.Errorf("migrate query_history: %w", err)
	}

	// ── Additive column migrations (errors ignored: column may already exist) ──
	db.Exec(`ALTER TABLE station_records ADD COLUMN raw_hex TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE station_records ADD COLUMN fecha TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE station_records ADD COLUMN hora  TEXT NOT NULL DEFAULT ''`)
	for i := 1; i <= 10; i++ {
		db.Exec(fmt.Sprintf(`ALTER TABLE station_history ADD COLUMN dato%d REAL NOT NULL DEFAULT 0`, i))
	}

	// ── Destructive migrations (errors ignored: already applied or column gone) ─

	// Drop tables that were designed but never populated — sync_records holds a FK
	// on sync_sessions so it must be dropped first.
	db.Exec(`DROP TABLE IF EXISTS sync_records`)
	db.Exec(`DROP TABLE IF EXISTS sync_sessions`)

	// Drop always-zero columns from station_records (legacy; replaced by fecha/hora TEXT).
	db.Exec(`ALTER TABLE station_records DROP COLUMN date_raw`)
	db.Exec(`ALTER TABLE station_records DROP COLUMN time_raw`)

	// Drop always-1 column from station_history (UpsertHistory skips invalid records).
	db.Exec(`ALTER TABLE station_history DROP COLUMN valid`)

	return nil
}
