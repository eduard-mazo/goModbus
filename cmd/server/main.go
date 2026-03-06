package main

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	web "goModbus"
	"goModbus/internal/api"
	"goModbus/internal/certgen"
	"goModbus/internal/db"
	"goModbus/internal/logger"
	"goModbus/internal/modbus"
)

func main() {
	defer logger.CloseLogger()

	// Wire modbus log calls into the application logger.
	// When a session ID (sid) is provided, route only to that client's WebSocket.
	modbus.LogFunc = func(sid, level, message string, raw []byte, duration time.Duration, pv *float64, dbh string) {
		if sid != "" {
			msg := logger.LogMessage{Level: level, Message: message, PointerValue: pv, DataBlockHex: dbh}
			if len(raw) > 0 {
				msg.RawHex = fmt.Sprintf("%X", raw)
			}
			if duration > 0 {
				msg.Duration = fmt.Sprintf("%dms", duration.Milliseconds())
			}
			logger.SessionBroadcast(sid, msg)
		} else {
			logger.BroadcastLog(level, message, raw, duration, pv, dbh)
		}
	}

	// Start WebSocket broadcaster goroutine
	logger.StartBroadcaster()

	// Ensure TLS certificates exist (auto-generated on first run)
	if err := certgen.EnsureCerts("certs"); err != nil {
		fmt.Println("WARN: no se pudo generar certificado TLS:", err)
	}

	// Open SQLite database
	database, err := db.Open("modbus.db")
	if err != nil {
		fmt.Println("WARN: no se pudo abrir base de datos:", err)
	} else {
		defer database.Close()
		_ = database // available for future handler injection
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), cors.Default())

	// Serve frontend: try local disk first (dev mode), then embedded dist/
	r.GET("/", func(c *gin.Context) {
		if data, err := os.ReadFile("frontend/dist/index.html"); err == nil {
			c.Data(http.StatusOK, "text/html; charset=utf-8", data)
			return
		}
		sub, _ := fs.Sub(web.DistFS, "dist")
		http.FileServer(http.FS(sub)).ServeHTTP(c.Writer, c.Request)
	})

	// Serve Vite assets (JS, CSS, etc.)
	r.GET("/assets/*filepath", func(c *gin.Context) {
		sub, _ := fs.Sub(web.DistFS, "dist")
		http.StripPrefix("/", http.FileServer(http.FS(sub))).ServeHTTP(c.Writer, c.Request)
	})

	// Serve static files (fonts, shared CSS): disk-first for dev
	r.GET("/static/*filepath", func(c *gin.Context) {
		path := c.Param("filepath")
		localPath := "static" + path
		if _, err := os.Stat(localPath); err == nil {
			c.File(localPath)
			return
		}
		sub, _ := fs.Sub(web.StaticFS, "static")
		http.StripPrefix("/static", http.FileServer(http.FS(sub))).ServeHTTP(c.Writer, c.Request)
	})

	api.RegisterRoutes(r)

	// SPA fallback: for any unmatched path, first try to serve it as a dist/ file
	// (covers /favicon.svg, /favicon.ico, etc.), then fall back to index.html.
	distSub, _ := fs.Sub(web.DistFS, "dist")
	distFS := http.FS(distSub)
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		// API and WS paths get a plain 404.
		if strings.HasPrefix(p, "/api") || strings.HasPrefix(p, "/ws") {
			c.Status(http.StatusNotFound)
			return
		}
		// Try local disk first (dev mode), then embedded dist/
		rel := strings.TrimPrefix(p, "/")
		localFile := "frontend/dist/" + rel
		if _, err := os.Stat(localFile); err == nil {
			c.File(localFile)
			return
		}
		if f, err := distSub.Open(rel); err == nil {
			f.Close()
			http.FileServer(distFS).ServeHTTP(c.Writer, c.Request)
			return
		}
		// Serve index.html for SPA routes (e.g. /roc, /query)
		if data, err := os.ReadFile("frontend/dist/index.html"); err == nil {
			c.Data(http.StatusOK, "text/html; charset=utf-8", data)
			return
		}
		indexData, err := fs.ReadFile(distSub, "index.html")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexData)
	})

	logger.BroadcastLog("INFO", "ROC Modbus Expert v4.0 | EPM | https://localhost:8443", nil, 0, nil, "")
	fmt.Println("ROC Modbus Expert v4.0 | EPM")
	fmt.Println("  HTTPS: https://localhost:8443")
	fmt.Println("  HTTPS: https://localhost:8083")

	// Serve HTTPS on both ports with the same certificate.
	// :8443 is the primary port; :8083 is an alias so that users on either port get TLS.
	go func() {
		srv := &http.Server{Addr: ":8083", Handler: r}
		if err := srv.ListenAndServeTLS("certs/cert.pem", "certs/key.pem"); err != nil {
			fmt.Println("WARN :8083 TLS:", err)
		}
	}()

	if err := r.RunTLS(":8443", "certs/cert.pem", "certs/key.pem"); err != nil {
		fmt.Println("ERROR TLS :8443:", err)
	}
}
