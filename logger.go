package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// LogMessage is the structure broadcast to WebSocket clients and written to file
type LogMessage struct {
	Timestamp    string   `json:"ts"`
	Level        string   `json:"level"`
	Message      string   `json:"msg"`
	RawHex       string   `json:"raw,omitempty"`
	Duration     string   `json:"dur,omitempty"`
	PointerValue *float64 `json:"ptr,omitempty"`
	DataBlockHex string   `json:"dbhex,omitempty"`
}

var (
	logBuffer  []LogMessage
	logMutex   sync.Mutex
	logChan    = make(chan LogMessage, 256)
	fileLogger *log.Logger
	logFile    *os.File
	clientsMu  sync.Mutex
)

func init() {
	os.MkdirAll("logs", 0755)
	logFile, _ = os.OpenFile("logs/modbus_debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	fileLogger = log.New(logFile, "", log.LstdFlags|log.Lmicroseconds)
}

func closeLogger() {
	if logFile != nil {
		logFile.Close()
	}
}

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

	// Also print to stdout so the terminal shows the flow (DEBUG omitted to reduce noise)
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
