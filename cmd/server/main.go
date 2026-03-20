package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	web "goModbus"
	"goModbus/internal/api"
	"goModbus/internal/api/handlers"
	"goModbus/internal/certgen"
	"goModbus/internal/db"
	"goModbus/internal/logger"
	"goModbus/internal/modbus"
)

// tlsFilter silencia los errores de TLS handshake del servidor HTTPS.
// "tls: unknown certificate" ocurre cuando el cliente no ha instalado el cert;
// no es un error del servidor, es el comportamiento esperado de TLS auto-firmado.
type tlsFilter struct{}

func (tlsFilter) Write(b []byte) (int, error) {
	s := string(b)
	if strings.Contains(s, "TLS handshake error") ||
		strings.Contains(s, "HTTP request to an HTTPS server") {
		return len(b), nil
	}
	os.Stderr.Write(b)
	return len(b), nil
}

func main() {
	defer logger.CloseLogger()

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

	logger.StartBroadcaster()

	if err := certgen.EnsureCerts("certs"); err != nil {
		fmt.Println("WARN: no se pudo generar certificado TLS:", err)
	}

	database, err := db.Open("modbus.db")
	if err != nil {
		fmt.Println("WARN: no se pudo abrir base de datos:", err)
	} else {
		defer database.Close()
		handlers.DB = database
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), cors.Default())

	// /cert — descarga el certificado TLS para instalación en el navegador/SO.
	// Una vez instalado, https://host:8443 no muestra advertencias.
	r.GET("/cert", func(c *gin.Context) {
		certPath := "certs/cert.pem"
		if _, err := os.Stat(certPath); err != nil {
			c.String(http.StatusNotFound, "certificado no generado aún")
			return
		}
		c.Header("Content-Disposition", `attachment; filename="roc-modbus-ca.pem"`)
		c.File(certPath)
	})

	// Frontend SPA
	r.GET("/", func(c *gin.Context) {
		if data, err := os.ReadFile("frontend/dist/index.html"); err == nil {
			c.Data(http.StatusOK, "text/html; charset=utf-8", data)
			return
		}
		sub, _ := fs.Sub(web.DistFS, "dist")
		http.FileServer(http.FS(sub)).ServeHTTP(c.Writer, c.Request)
	})

	r.GET("/assets/*filepath", func(c *gin.Context) {
		sub, _ := fs.Sub(web.DistFS, "dist")
		http.StripPrefix("/", http.FileServer(http.FS(sub))).ServeHTTP(c.Writer, c.Request)
	})

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

	distSub, _ := fs.Sub(web.DistFS, "dist")
	distFS := http.FS(distSub)
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api") || strings.HasPrefix(p, "/ws") {
			c.Status(http.StatusNotFound)
			return
		}
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

	// Scheduler: auto-sync de todas las estaciones cada hora en el minuto 5.
	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 5, 0, 0, now.Location())
			if !next.After(now) {
				next = next.Add(time.Hour)
			}
			time.Sleep(time.Until(next))
			logger.BroadcastLog("INFO",
				fmt.Sprintf("Auto-sync programado — %s", time.Now().Format("15:04")),
				nil, 0, nil, "")
			handlers.RunAutoSync()
		}
	}()

	// :8443 — HTTPS con certificado auto-firmado (instalar cert para evitar advertencias)
	errLog := log.New(tlsFilter{}, "", 0)
	go func() {
		srv := &http.Server{Addr: ":8443", Handler: r, ErrorLog: errLog}
		if err := srv.ListenAndServeTLS("certs/cert.pem", "certs/key.pem"); err != nil {
			fmt.Println("WARN :8443 TLS:", err)
		}
	}()

	// :8080 — HTTP plano (acceso sin certificado, sin advertencias del navegador)
	logger.BroadcastLog("INFO", "ROC Modbus Expert v4.0 | EPM | http://localhost:8080", nil, 0, nil, "")
	fmt.Println("ROC Modbus Expert v4.0 | EPM")
	fmt.Println("  HTTP : http://localhost:8080   ← acceso directo, sin advertencias")
	fmt.Println("  HTTPS: https://localhost:8443  ← instalar /cert para eliminar advertencia")
	fmt.Println("  Cert : http://localhost:8080/cert  ← descargar e instalar en el navegador/SO")

	if err := http.ListenAndServe(":8080", r); err != nil {
		fmt.Println("ERROR HTTP :8080:", err)
	}
}
