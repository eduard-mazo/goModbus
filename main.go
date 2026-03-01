package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"

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

	// Start WebSocket broadcaster: fans out logChan messages to all connected clients
	go func() {
		for msg := range logChan {
			clientsMu.Lock()
			for client := range logClients {
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
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})
	subFS, _ := fs.Sub(staticFS, "static")
	r.StaticFS("/static", http.FS(subFS))
	r.GET("/ws", wsHandler)

	api := r.Group("/api")
	{
		api.GET("/config", getConfigHandler)
		api.POST("/query", queryHandler)          // Generic Modbus query (FC01-FC16)
		api.POST("/roc", rocHandler)              // ROC pointer + history workflow
		api.POST("/roc/history24", rocHistory24Handler) // ROC 24-hour circular buffer fetch
		api.POST("/raw", rawHandler)                    // Send raw ADU frame as-is
	}

	broadcastLog("INFO", "ROC Modbus Expert v3.0 | EPM | http://localhost:8081", nil, 0, nil, "")
	fmt.Println("ROC Modbus Expert v3.0 | EPM | http://localhost:8081")
	r.Run(":8081")
}
