package handlers

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"goModbus/internal/logger"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsClientConn holds a WebSocket connection with its own send channel and session ID.
// It implements logger.Client.
type wsClientConn struct {
	conn      *websocket.Conn
	ch        chan logger.LogMessage
	mu        sync.RWMutex
	sessionID string
}

func (c *wsClientConn) Send(msg logger.LogMessage) {
	select {
	case c.ch <- msg:
	default: // drop if client buffer full
	}
}

func (c *wsClientConn) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

func (c *wsClientConn) setSessionID(sid string) {
	c.mu.Lock()
	c.sessionID = sid
	c.mu.Unlock()
}

// WsHandler upgrades HTTP to WebSocket and streams log messages.
func WsHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &wsClientConn{conn: conn, ch: make(chan logger.LogMessage, 256)}

	// Replay history BEFORE registering so we don't double-deliver live messages.
	// Skip SID-tagged messages — progress events from a previous session are irrelevant.
	for _, msg := range logger.GetLogHistory() {
		if msg.SID != "" {
			continue
		}
		if err := conn.WriteJSON(msg); err != nil {
			conn.Close()
			return
		}
	}

	// Register to receive live messages.
	logger.RegisterClient(client)

	defer func() {
		logger.UnregisterClient(client)
		close(client.ch)
		conn.Close()
	}()

	// Reader goroutine: handles session registration messages from the browser.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var reg struct {
				Type string `json:"type"`
				SID  string `json:"sid"`
			}
			if err := conn.ReadJSON(&reg); err != nil {
				return
			}
			if reg.Type == "register" && reg.SID != "" {
				client.setSessionID(reg.SID)
			}
		}
	}()

	// Write loop: stream messages until client disconnects.
	for {
		select {
		case msg, ok := <-client.ch:
			if !ok {
				return
			}
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}
