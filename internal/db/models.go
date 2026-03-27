package db

import "time"

// StationRecord is one ROC circular-buffer record cached in SQLite.
// Stored in station_records (PK: task_key + ptr, max 840 rows/station).
// Used for delta-sync: compare fecha/hora against the device's live pointer
// to determine which slots are stale and need re-fetching.
type StationRecord struct {
	Ptr    int
	Fecha  string      // "YYYY-MM-DD" decoded from ROC float Modes[0]
	Hora   string      // "HH:MM"      decoded from ROC float Modes[1]
	Hex    string      // 40-byte data payload as hex (10 float32 in device endian)
	RawHex string      // full Modbus ADU as hex (MBAP header + payload)
	Valid  bool        // true when the FC03 read succeeded
	Datos  [10]float64 // pre-decoded values: [0]=fecha_f, [1]=hora_f, [2..9]=8 signals
}

// HistoryRecord is one entry in the unlimited long-term history table.
// Stored in station_history (PK: task_key + fecha + hora).
// One row per station-hour; accumulates beyond the 840-slot circular buffer.
type HistoryRecord struct {
	Ptr    int
	Fecha  string      // "YYYY-MM-DD"
	Hora   string      // "HH:MM"
	Hex    string      // 40-byte payload as hex; used to reconstruct Modes for the chart
	RawHex string      // full Modbus ADU
	Datos  [10]float64 // [0]=dato1 (fecha float), [1]=dato2 (hora float), [2..9]=signals S1-S8
}

// TaskMeta stores the circular-buffer reference point captured at sync time.
// Used by the frontend to derive real timestamps from ptr index:
//
//	ts_ms = (RefTime - (RefPtr - ptr + 840) % 840 * 3600) * 1000
type TaskMeta struct {
	TaskKey string
	RefPtr  int
	RefTime int64 // unix seconds
}

// QueryHistory records one generic Modbus query for auditing.
// Stored in query_history; populated by AppendQueryHistory.
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
