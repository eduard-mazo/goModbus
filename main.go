package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Stations []StationConfig `yaml:"stations" json:"stations"`
}

type StationConfig struct {
	IP           string     `yaml:"ip" json:"ip"`
	Port         int        `yaml:"port" json:"port"`
	ID           byte       `yaml:"id" json:"id"`
	Name         string     `yaml:"name" json:"name"`
	Endianness   Endianness `yaml:"endian" json:"endian"`
	DataType     string     `yaml:"data_type" json:"data_type"`
	StartAddr    uint16     `yaml:"start_addr" json:"start_addr"`
	Quantity     uint16     `yaml:"quantity" json:"quantity"`
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	configPath = "config.yaml"
	configMu   sync.RWMutex
)

func main() {
	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/", func(c *gin.Context) {
		c.File("index.html")
	})

	r.GET("/ws", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		clientsMu.Lock()
		logClients[conn] = true
		clientsMu.Unlock()
		defer func() {
			clientsMu.Lock()
			delete(logClients, conn)
			clientsMu.Unlock()
		}()
		logMutex.Lock()
		for _, msg := range logBuffer {
			conn.WriteJSON(msg)
		}
		logMutex.Unlock()
		for {
			select {
			case msg := <-logChan:
				clientsMu.Lock()
				for client := range logClients {
					client.WriteJSON(msg)
				}
				clientsMu.Unlock()
			}
		}
	})

	api := r.Group("/api")
	{
		api.GET("/config", func(c *gin.Context) {
			cfg, err := loadConfig(configPath)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			c.JSON(200, cfg)
		})

		api.POST("/config", func(c *gin.Context) {
			var cfg Config
			if err := c.ShouldBindJSON(&cfg); err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}
			configMu.Lock()
			data, _ := yaml.Marshal(cfg)
			os.WriteFile(configPath, data, 0644)
			configMu.Unlock()
			c.JSON(200, gin.H{"status": "ok"})
		})

		api.POST("/test", func(c *gin.Context) {
			var req struct {
				IP     string     `json:"ip"`
				Port   int        `json:"port"`
				ID     byte       `json:"id"`
				Endian Endianness `json:"endian"`
				Addr   uint16     `json:"addr"`
				Count  uint16     `json:"count"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}
			client := NewModbusClient(req.IP, req.Port, req.ID, req.Endian)
			if err := client.Connect(); err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			defer client.Close()
			data, err := client.ReadHoldingRegisters(req.Addr, req.Count)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			c.JSON(200, gin.H{"raw": fmt.Sprintf("%X", data)})
		})

		api.POST("/raw", func(c *gin.Context) {
			var req struct {
				IP   string `json:"ip"`
				Port int    `json:"port"`
				Hex  string `json:"hex"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}
			rawBytes, err := hex.DecodeString(req.Hex)
			if err != nil {
				c.JSON(400, gin.H{"error": "Invalid HEX"})
				return
			}
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", req.IP, req.Port), 5*time.Second)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			defer conn.Close()
			
			start := time.Now()
			broadcastLog("DEBUG", "Manual HEX Dispatched", rawBytes, 0)
			conn.Write(rawBytes)
			buf := make([]byte, 1024)
			n, err := conn.Read(buf)
			elapsed := time.Since(start)
			
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			broadcastLog("INFO", "Manual HEX Response", buf[:n], elapsed)
			c.JSON(200, gin.H{"raw": hex.EncodeToString(buf[:n])})
		})
	}

	fmt.Println("🚀 UI Server: http://localhost:8080")
	r.Run(":8080")
}

func loadConfig(filename string) (*Config, error) {
	configMu.RLock()
	defer configMu.RUnlock()
	data, err := os.ReadFile(filename)
	if err != nil {
		return &Config{Stations: []StationConfig{}}, nil
	}
	var config Config
	err = yaml.Unmarshal(data, &config)
	return &config, err
}
