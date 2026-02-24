package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Endianness string

const (
	BigEndian    Endianness = "abcd"
	LittleEndian Endianness = "dcba"
	WordSwapped  Endianness = "cdab"
	ByteSwapped  Endianness = "badc"
)

type LogMessage struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	RawHex    string `json:"raw_hex,omitempty"`
	Duration  string `json:"duration,omitempty"`
}

var (
	logBuffer   []LogMessage
	logMutex    sync.Mutex
	logClients  = make(map[*websocket.Conn]bool)
	clientsMu   sync.Mutex
	logChan     = make(chan LogMessage, 100)
)

func broadcastLog(level, message string, raw []byte, duration time.Duration) {
	msg := LogMessage{
		Timestamp: time.Now().Format("15:04:05.000"),
		Level:     level,
		Message:   message,
	}
	if len(raw) > 0 {
		msg.RawHex = fmt.Sprintf("%X", raw)
	}
	if duration > 0 {
		msg.Duration = fmt.Sprintf("%dms", duration.Milliseconds())
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
		Timeout:       60 * time.Second, // Timeout de 1 minuto para procesos largos
		TransactionID: 1,
	}
}

func (c *ModbusClient) Connect() error {
	address := fmt.Sprintf("%s:%d", c.Host, c.Port)
	conn, err := net.DialTimeout("tcp", address, 10*time.Second) // Timeout de conexión inicial
	if err != nil {
		broadcastLog("ERROR", fmt.Sprintf("Fallo de conexión: %v", err), nil, 0)
		return err
	}
	c.Conn = conn
	return nil
}

func (c *ModbusClient) Close() {
	if c.Conn != nil {
		c.Conn.Close()
	}
}

func (c *ModbusClient) ReadHoldingRegisters(addr uint16, count uint16) ([]byte, error) {
	if c.Conn == nil {
		return nil, fmt.Errorf("no conectado")
	}

	c.Conn.SetDeadline(time.Now().Add(c.Timeout)) // Aplicar timeout de 1 min a la operación

	req := make([]byte, 12)
	binary.BigEndian.PutUint16(req[0:], c.TransactionID)
	binary.BigEndian.PutUint16(req[2:], 0)
	binary.BigEndian.PutUint16(req[4:], 6)
	req[6] = c.UnitID
	req[7] = 0x03
	binary.BigEndian.PutUint16(req[8:], addr)
	binary.BigEndian.PutUint16(req[10:], count)

	start := time.Now()
	broadcastLog("DEBUG", "Despachando Trama Modbus...", req, 0)

	_, err := c.Conn.Write(req)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 2048)
	n, err := c.Conn.Read(buf)
	elapsed := time.Since(start)

	if err != nil {
		broadcastLog("ERROR", fmt.Sprintf("Error en lectura (Timeout?): %v", err), nil, elapsed)
		return nil, err
	}

	if n < 9 {
		return nil, fmt.Errorf("respuesta incompleta")
	}

	if buf[7] >= 0x80 {
		broadcastLog("ERROR", fmt.Sprintf("Excepción Modbus 0x%X", buf[8]), buf[:n], elapsed)
		return nil, fmt.Errorf("modbus exception %d", buf[8])
	}

	broadcastLog("INFO", "Petición Completada", buf[:n], elapsed)
	c.TransactionID++
	return buf[9:n], nil
}

func (c *ModbusClient) DecodeFloat32(data []byte) []float32 {
	count := len(data) / 4
	result := make([]float32, 0, count)
	for i := 0; i < len(data); i += 4 {
		if i+4 > len(data) { break }
		var combined uint32
		b := data[i : i+4]
		
		switch c.Endian {
		case LittleEndian: // DCBA
			combined = uint32(b[3])<<24 | uint32(b[2])<<16 | uint32(b[1])<<8 | uint32(b[0])
		case WordSwapped: // CDAB
			combined = uint32(b[2])<<24 | uint32(b[3])<<16 | uint32(b[0])<<8 | uint32(b[1])
		case ByteSwapped: // BADC
			combined = uint32(b[1])<<24 | uint32(b[0])<<16 | uint32(b[3])<<8 | uint32(b[2])
		default: // ABCD (Big Endian)
			combined = uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
		}
		result = append(result, math.Float32frombits(combined))
	}
	return result
}
