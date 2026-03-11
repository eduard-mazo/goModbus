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
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sync_sessions (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			started_at  DATETIME NOT NULL,
			finished_at DATETIME,
			stations    TEXT NOT NULL,
			total_ptrs  INTEGER,
			ok_ptrs     INTEGER
		);

		CREATE TABLE IF NOT EXISTS sync_records (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id  INTEGER NOT NULL REFERENCES sync_sessions(id),
			station     TEXT NOT NULL,
			ptr         INTEGER NOT NULL,
			hour_label  TEXT,
			valid       INTEGER NOT NULL,
			signals     TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS station_records (
			task_key  TEXT    NOT NULL,
			ptr       INTEGER NOT NULL,
			date_raw  INTEGER NOT NULL DEFAULT 0,
			time_raw  INTEGER NOT NULL DEFAULT 0,
			hex       TEXT    NOT NULL DEFAULT '',
			valid     INTEGER NOT NULL DEFAULT 0,
			synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (task_key, ptr)
		);

		CREATE TABLE IF NOT EXISTS task_meta (
			task_key  TEXT    PRIMARY KEY,
			ref_ptr   INTEGER NOT NULL DEFAULT -1,
			ref_time  INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS query_history (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			queried_at  DATETIME NOT NULL,
			host        TEXT,
			port        INTEGER,
			unit_id     INTEGER,
			fc          INTEGER,
			address     INTEGER,
			quantity    INTEGER,
			result_hex  TEXT
		);
	`)
	if err != nil {
		return err
	}
	// Additive column migrations (ignore error if column already exists)
	db.Exec(`ALTER TABLE station_records ADD COLUMN raw_hex TEXT NOT NULL DEFAULT ''`)
	return nil
}
