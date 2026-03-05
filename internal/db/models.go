package db

import "time"

// SyncSession represents one full-sync run across one or more stations.
type SyncSession struct {
	ID         int64
	StartedAt  time.Time
	FinishedAt *time.Time
	Stations   string // JSON array of station names
	TotalPtrs  int
	OkPtrs     int
}

// SyncRecord holds a single pointer's data from a sync session.
type SyncRecord struct {
	ID        int64
	SessionID int64
	Station   string
	Ptr       int
	HourLabel string // "00:00", "01:00" …
	Valid     bool
	Signals   string // JSON: {"flow":1.23,"pulses":4.56,...}
}

// QueryHistory records a generic Modbus query for auditing.
type QueryHistory struct {
	ID        int64
	QueriedAt time.Time
	Host      string
	Port      int
	UnitID    int
	FC        int
	Address   int
	Quantity  int
	ResultHex string
}
