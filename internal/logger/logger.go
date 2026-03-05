package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

var (
	logBuffer  []LogMessage
	logMutex   sync.Mutex
	LogChan    = make(chan LogMessage, 512)
	fileLogger *log.Logger
	logFile    *os.File

	clients   = make(map[Client]bool)
	clientsMu sync.Mutex
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

func CloseLogger() {
	if logFile != nil {
		logFile.Close()
	}
}

// RegisterClient adds a WebSocket client to receive broadcast messages.
func RegisterClient(c Client) {
	clientsMu.Lock()
	clients[c] = true
	clientsMu.Unlock()
}

// UnregisterClient removes a client from the broadcast set.
func UnregisterClient(c Client) {
	clientsMu.Lock()
	delete(clients, c)
	clientsMu.Unlock()
}

// StartBroadcaster fans out LogChan messages to all registered clients.
// Session-scoped messages (msg.SID != "") are delivered only to the owning client.
func StartBroadcaster() {
	go func() {
		for msg := range LogChan {
			clientsMu.Lock()
			for c := range clients {
				if msg.SID != "" && c.SessionID() != msg.SID {
					continue
				}
				c.Send(msg)
			}
			clientsMu.Unlock()
		}
	}()
}

// GetLogHistory returns a copy of the current ring buffer (for WS replay on connect).
func GetLogHistory() []LogMessage {
	logMutex.Lock()
	h := make([]LogMessage, len(logBuffer))
	copy(h, logBuffer)
	logMutex.Unlock()
	return h
}

// BroadcastLog sends a general (non-session) log message to all connected clients.
func BroadcastLog(level, message string, raw []byte, duration time.Duration, pv *float64, dbh string) {
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
	case LogChan <- msg:
	default:
	}
}

// SessionBroadcast sends a session-scoped message (only delivered to the owning client).
// Progress messages are NOT added to the ring buffer to avoid replaying them on reconnect.
func SessionBroadcast(sid string, msg LogMessage) {
	msg.SID = sid
	msg.Timestamp = time.Now().Format("15:04:05.000")
	fileLogger.Printf("[%s] %-5s %s | sid=%.8s", msg.Timestamp, msg.Level, msg.Message, sid)

	if msg.Progress == nil {
		logMutex.Lock()
		logBuffer = append(logBuffer, msg)
		if len(logBuffer) > 500 {
			logBuffer = logBuffer[1:]
		}
		logMutex.Unlock()
	}

	select {
	case LogChan <- msg:
	default:
	}
}
