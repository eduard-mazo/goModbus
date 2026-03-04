package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// SyncProgress reports per-station progress streamed over WebSocket
type SyncProgress struct {
	Station string       `json:"station"`
	Done    int          `json:"done"`
	Total   int          `json:"total"`
	Pct     int          `json:"pct"`
	Records []HourRecord `json:"records,omitempty"` // populated only on final message
	Error   string       `json:"error,omitempty"`
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

var (
	logBuffer  []LogMessage
	logMutex   sync.Mutex
	logChan    = make(chan LogMessage, 512)
	fileLogger *log.Logger
	logFile    *os.File
	clientsMu  sync.Mutex
)

func init() {
	os.MkdirAll("logs", 0755)
	f, err := os.OpenFile("logs/modbus_debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		logFile = f
		fileLogger = log.New(logFile, "", log.LstdFlags|log.Lmicroseconds)
	} else {
		fileLogger = log.New(os.Stderr, "[LOG] ", log.LstdFlags|log.Lmicroseconds)
	}
}

func closeLogger() {
	if logFile != nil {
		logFile.Close()
	}
}

// broadcastLog sends a general (non-session) log message to all connected clients.
func broadcastLog(level, message string, raw []byte, duration time.Duration, pv *float64, dbh string) {
	msg := LogMessage{
		Timestamp:    time.Now().Format("15:04:05.000"),
		Level:        level,
		Message:      message,
		PointerValue: pv,
		DataBlockHex: dbh,
	}
	if len(raw) > 0 {
		msg.RawHex = fmt.Sprintf("%X", raw)
	}
	if duration > 0 {
		msg.Duration = fmt.Sprintf("%dms", duration.Milliseconds())
	}

	fileLogger.Printf("[%s] %-5s %s | hex=%s dur=%s", msg.Timestamp, msg.Level, msg.Message, msg.RawHex, msg.Duration)

	if level != "DEBUG" {
		if msg.Duration != "" {
			fmt.Printf("%s %-5s %s [%s]\n", msg.Timestamp, msg.Level, msg.Message, msg.Duration)
		} else {
			fmt.Printf("%s %-5s %s\n", msg.Timestamp, msg.Level, msg.Message)
		}
	}

	logMutex.Lock()
	logBuffer = append(logBuffer, msg)
	if len(logBuffer) > 500 {
		logBuffer = logBuffer[1:]
	}
	logMutex.Unlock()

	select {
	case logChan <- msg:
	default:
	}
}

// sessionBroadcast sends a session-scoped message (only delivered to the owning client).
// Progress messages are NOT added to the ring buffer to avoid replaying them on reconnect.
func sessionBroadcast(sid string, msg LogMessage) {
	msg.SID = sid
	msg.Timestamp = time.Now().Format("15:04:05.000")
	fileLogger.Printf("[%s] %-5s %s | sid=%.8s", msg.Timestamp, msg.Level, msg.Message, sid)

	if msg.Progress == nil {
		// Non-progress session messages go into the ring buffer
		logMutex.Lock()
		logBuffer = append(logBuffer, msg)
		if len(logBuffer) > 500 {
			logBuffer = logBuffer[1:]
		}
		logMutex.Unlock()
	}

	select {
	case logChan <- msg:
	default:
	}
}
