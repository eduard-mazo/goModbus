package main

import (
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Endianness string

const (
	BigEndian    Endianness = "big"
	LittleEndian Endianness = "little"
)

// LogMessage represents a log entry for the UI
type LogMessage struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	RawHex    string `json:"raw_hex,omitempty"`
}

var (
	logBuffer   []LogMessage
	logMutex    sync.Mutex
	logClients  = make(map[*websocket.Conn]bool)
	clientsMu   sync.Mutex
	logChan     = make(chan LogMessage, 100)
)

func broadcastLog(level, message string, raw []byte) {
	msg := LogMessage{
		Timestamp: time.Now().Format("15:04:05"),
		Level:     level,
		Message:   message,
	}
	if len(raw) > 0 {
		msg.RawHex = fmt.Sprintf("%X", raw)
	}

	logMutex.Lock()
	logBuffer = append(logBuffer, msg)
	if len(logBuffer) > 100 {
		logBuffer = logBuffer[1:]
	}
	logMutex.Unlock()

	select {
	case logChan <- msg:
	default:
	}
}

type ModbusClient struct {
	Host          string
	Port          int
	UnitID        byte
	Endian        Endianness
	Timeout       time.Duration
	Conn          net.Conn
	TransactionID uint16
}

func NewModbusClient(host string, port int, unitID byte, endian Endianness) *ModbusClient {
	return &ModbusClient{
		Host:          host,
		Port:          port,
		UnitID:        unitID,
		Endian:        endian,
		Timeout:       10 * time.Second,
		TransactionID: 1,
	}
}

func (c *ModbusClient) Connect() error {
	address := fmt.Sprintf("%s:%d", c.Host, c.Port)
	conn, err := net.DialTimeout("tcp", address, c.Timeout)
	if err != nil {
		broadcastLog("ERROR", fmt.Sprintf("Connection failed: %v", err), nil)
		return err
	}
	c.Conn = conn
	broadcastLog("INFO", fmt.Sprintf("Connected to %s", address), nil)
	return nil
}

func (c *ModbusClient) Close() {
	if c.Conn != nil {
		c.Conn.Close()
	}
}

func (c *ModbusClient) ReadHoldingRegisters(addr uint16, count uint16) ([]byte, error) {
	if c.Conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	req := make([]byte, 12)
	binary.BigEndian.PutUint16(req[0:], c.TransactionID)
	binary.BigEndian.PutUint16(req[2:], 0)      // Protocol ID
	binary.BigEndian.PutUint16(req[4:], 6)      // Length
	req[6] = c.UnitID                           // Unit ID
	req[7] = 0x03                               // Function code
	binary.BigEndian.PutUint16(req[8:], addr)   // Start Address
	binary.BigEndian.PutUint16(req[10:], count) // Quantity

	broadcastLog("DEBUG", fmt.Sprintf("Request: Read Holding %d (count %d)", addr, count), req)

	_, err := c.Conn.Write(req)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 1024)
	n, err := c.Conn.Read(buf)
	if err != nil {
		return nil, err
	}

	if n < 9 {
		return nil, fmt.Errorf("response too short")
	}

	if buf[7] == 0x83 {
		broadcastLog("ERROR", fmt.Sprintf("Modbus Exception: %d", buf[8]), buf[:n])
		return nil, fmt.Errorf("modbus exception: %d", buf[8])
	}

	byteCount := int(buf[8])
	broadcastLog("DEBUG", fmt.Sprintf("Response received (%d bytes)", byteCount), buf[:n])

	c.TransactionID++
	return buf[9 : 9+byteCount], nil
}

func (c *ModbusClient) DecodeFloat32(data []byte) []float32 {
	count := len(data) / 4
	result := make([]float32, 0, count)

	for i := 0; i < len(data); i += 4 {
		var combined uint32
		if c.Endian == LittleEndian {
			word1 := binary.BigEndian.Uint16(data[i:])
			word2 := binary.BigEndian.Uint16(data[i+2:])
			combined = uint32(word2)<<16 | uint32(word1)
		} else {
			combined = binary.BigEndian.Uint32(data[i:])
		}
		result = append(result, math.Float32frombits(combined))
	}
	return result
}

func timestampNow() string {
	return time.Now().Format("020106150405")
}

func formatDate(val float32) string {
	s := fmt.Sprintf("%06d", int(val))
	if len(s) < 6 {
		return "Invalid Date"
	}
	return fmt.Sprintf("%s/%s/20%s", s[:2], s[2:4], s[4:])
}

func formatTime(val float32) string {
	s := fmt.Sprintf("%04d", int(val))
	if len(s) < 4 {
		return "00:00"
	}
	return fmt.Sprintf("%s:%s", s[:2], s[2:])
}

func guardarCSV(nombre string, data [][]string) {
	file, err := os.Create(nombre)
	if err != nil {
		log.Println("Error creating file:", err)
		return
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	writer.WriteAll(data)
}
