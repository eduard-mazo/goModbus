package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

//go:embed index.html
var indexHTML []byte

//go:embed static
var staticFS embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	defer closeLogger()

	// Start WebSocket broadcaster: fans out logChan messages to connected clients.
	// Session-scoped messages (msg.SID != "") are delivered only to the owning client.
	go func() {
		for msg := range logChan {
			clientsMu.Lock()
			for client := range logClients {
				if msg.SID != "" && client.sessionID != msg.SID {
					continue // route session messages only to their owner
				}
				select {
				case client.ch <- msg:
				default: // drop if client buffer full
				}
			}
			clientsMu.Unlock()
		}
	}()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), cors.Default())

	r.GET("/", func(c *gin.Context) {
		// Try reading from disk first for development
		if data, err := os.ReadFile("index.html"); err == nil {
			c.Data(http.StatusOK, "text/html; charset=utf-8", data)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})

	// Serve static files: try local disk first, then fallback to embed
	r.GET("/static/*filepath", func(c *gin.Context) {
		path := c.Param("filepath")
		localPath := "static" + path
		if _, err := os.Stat(localPath); err == nil {
			c.File(localPath)
			return
		}
		
		// Fallback to embed
		subFS, _ := fs.Sub(staticFS, "static")
		http.StripPrefix("/static", http.FileServer(http.FS(subFS))).ServeHTTP(c.Writer, c.Request)
	})
	
	r.GET("/ws", wsHandler)

	api := r.Group("/api")
	{
		api.GET("/config", getConfigHandler)
		api.POST("/stations/full-sync", fullSyncHandler)      // Updated to POST to support body filtering
		api.POST("/stations/partial-sync", partialSyncHandler) // New endpoint for retries
		api.POST("/query", queryHandler)          // Generic Modbus query (FC01-FC16)
		api.POST("/roc", rocHandler)              // ROC pointer + history workflow
		api.POST("/roc/history24", rocHistory24Handler) // ROC 24-hour circular buffer fetch
		api.POST("/raw", rawHandler)                    // Send raw ADU frame as-is
	}

	broadcastLog("INFO", "ROC Modbus Expert v3.0 | EPM | http://localhost:8083", nil, 0, nil, "")
	fmt.Println("ROC Modbus Expert v3.0 | EPM | http://localhost:8083")
	r.Run(":8083")
}
