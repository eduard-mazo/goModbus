package main

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Stations []StationConfig `yaml:"stations" json:"stations"`
}

type StationConfig struct {
	Name               string     `yaml:"name" json:"name"`
	IP                 string     `yaml:"ip" json:"ip"`
	Port               int        `yaml:"port" json:"port"`
	ID                 byte       `yaml:"id" json:"id"`
	PointerEndian      Endianness `yaml:"pointer_endian" json:"pointer_endian"`
	PointerAddress     uint16     `yaml:"pointer_address" json:"pointer_address"`
	DBEndian           Endianness `yaml:"db_endian" json:"db_endian"`
	DBAddress          uint16     `yaml:"db_address" json:"db_address"`
	DataRegistersCount uint16     `yaml:"data_registers_count" json:"data_registers_count"` // Qty para el Puntero
	DataType           string     `yaml:"data_type" json:"data_type"`
}

var (
	upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	configPath = "config.yaml"
	configMu   sync.RWMutex
)

func main() {
	defer closeLogger()
	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/", func(c *gin.Context) { c.File("index.html") })
	r.GET("/ws", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil { return }
		defer conn.Close()
		clientsMu.Lock()
		logClients[conn] = true
		clientsMu.Unlock()
		defer func() { clientsMu.Lock(); delete(logClients, conn); clientsMu.Unlock() }()
		logMutex.Lock()
		for _, msg := range logBuffer { conn.WriteJSON(msg) }
		logMutex.Unlock()
		for {
			select {
			case msg := <-logChan:
				clientsMu.Lock()
				for client := range logClients { client.WriteJSON(msg) }
				clientsMu.Unlock()
			}
		}
	})

	api := r.Group("/api")
	{
		api.GET("/config", func(c *gin.Context) {
			cfg, _ := loadConfig(configPath)
			c.JSON(200, cfg)
		})

		api.POST("/test", func(c *gin.Context) {
			var req struct {
				IP          string     `json:"ip"`
				Port        int        `json:"port"`
				ID          byte       `json:"id"`
				PtrEndian   Endianness `json:"pointer_endian"`
				PtrAddr     uint16     `json:"pointer_address"`
				PtrQty      uint16     `json:"pointer_qty"` // data_registers_count del YAML
				DBEndian    Endianness `json:"db_endian"`
				DBAddr      uint16     `json:"db_address"`
				Mode        string     `json:"mode"`
				ManualPtr   *float64   `json:"manual_ptr"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}

			client := NewModbusClient(req.IP, req.Port, req.ID, req.PtrEndian)
			if err := client.Connect(); err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			defer client.Close()

			var pVal float64
			var pData, dbData []byte

			// 1. Leer Puntero (usando req.PtrQty)
			if req.Mode == "ptr" || req.Mode == "full" {
				var err error
				pData, err = client.ReadHoldingRegisters(req.PtrAddr, req.PtrQty)
				if err != nil {
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}
				// Decodificación inteligente del puntero
				if len(pData) >= 4 {
					pVal = float64(client.DecodeFloat32(pData)[0])
				} else if len(pData) >= 2 {
					pVal = float64(client.DecodeInt16(pData)[0])
				}
				broadcastLog("INFO", fmt.Sprintf("Puntero ROC leido: %.0f", pVal), pData, 0, &pVal, "")
			} else if req.Mode == "hist" && req.ManualPtr != nil {
				pVal = *req.ManualPtr
			}

			// 2. Leer Histórico (ROC: Qty siempre 1)
			if req.Mode == "hist" || req.Mode == "full" {
				client.Endian = req.DBEndian
				dataAddr := req.DBAddr + uint16(pVal)
				var err error
				dbData, err = client.ReadHoldingRegisters(dataAddr, 1) // QTY SIEMPRE 1
				if err != nil {
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}
				broadcastLog("INFO", "Bloque Histórico ROC Recibido", dbData, 0, &pVal, hex.EncodeToString(dbData))
			}

			c.JSON(200, gin.H{
				"pointer_val": pVal,
				"pointer_hex": hex.EncodeToString(pData),
				"db_hex":      hex.EncodeToString(dbData),
			})
		})
	}

	fmt.Println("🚀 ROC Expert v2.1 (Reactiva) corriendo en puerto 8080")
	r.Run(":8080")
}

func loadConfig(f string) (*Config, error) {
	configMu.RLock()
	defer configMu.RUnlock()
	d, err := os.ReadFile(f)
	if err != nil { return &Config{}, nil }
	var c Config
	yaml.Unmarshal(d, &c)
	return &c, nil
}
