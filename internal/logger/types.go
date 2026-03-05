package logger

import "goModbus/internal/modbus"

// SyncProgress reports per-station progress streamed over WebSocket
type SyncProgress struct {
	Station string              `json:"station"`
	Done    int                 `json:"done"`
	Total   int                 `json:"total"`
	Pct     int                 `json:"pct"`
	Records []modbus.HourRecord `json:"records,omitempty"` // populated only on final message
	Error   string              `json:"error,omitempty"`
}

// LogMessage is the structure broadcast to WebSocket clients and written to file
type LogMessage struct {
	Timestamp    string        `json:"ts"`
	Level        string        `json:"level"`
	Message      string        `json:"msg"`
	RawHex       string        `json:"raw,omitempty"`
	Duration     string        `json:"dur,omitempty"`
	PointerValue *float64      `json:"ptr,omitempty"`
	DataBlockHex string        `json:"dbhex,omitempty"`
	SID          string        `json:"sid,omitempty"`      // session ID — routes only to owner
	Progress     *SyncProgress `json:"progress,omitempty"` // non-nil for sync progress events
}

// Client is implemented by WebSocket connections to receive broadcast messages.
type Client interface {
	Send(msg LogMessage)
	SessionID() string
}
