package main

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"

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

	// Wire modbus log calls into the application logger
	modbus.LogFunc = logger.BroadcastLog

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

	logger.BroadcastLog("INFO", "ROC Modbus Expert v4.0 | EPM | https://localhost:8443", nil, 0, nil, "")
	fmt.Println("ROC Modbus Expert v4.0 | EPM")
	fmt.Println("  HTTPS: https://localhost:8443")
	fmt.Println("  HTTP redirect: http://localhost:8083")

	// HTTP → HTTPS redirect on :8083
	go http.ListenAndServe(":8083", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + r.Host + r.RequestURI
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}))

	// HTTPS on :8443
	if err := r.RunTLS(":8443", "certs/cert.pem", "certs/key.pem"); err != nil {
		fmt.Println("TLS falló, arrancando en HTTP :8083 como respaldo:", err)
		r.Run(":8083")
	}
}
