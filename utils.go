package main

import (
	"encoding/binary"
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
	// Campos para depuración
	PointerValue *float64 `json:"pointer_value,omitempty"`
	DataBlockHex string   `json:"data_block_hex,omitempty"`
}

var (
	logBuffer   []LogMessage
	logMutex    sync.Mutex
	logClients  = make(map[*websocket.Conn]bool)
	clientsMu   sync.Mutex
	logChan     = make(chan LogMessage, 100)
	fileLogger  *log.Logger
	logFile     *os.File
)

func init() {
	os.MkdirAll("logs", 0755)
	logFile, _ = os.OpenFile("logs/modbus_debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	fileLogger = log.New(logFile, "", log.LstdFlags|log.Lmicroseconds)
}

func closeLogger() { if logFile != nil { logFile.Close() } }

func broadcastLog(level, message string, raw []byte, duration time.Duration, pv *float64, dbh string) {
	msg := LogMessage{
		Timestamp: time.Now().Format("15:04:05.000"),
		Level:     level,
		Message:   message,
		PointerValue: pv,
		DataBlockHex: dbh,
	}
	if len(raw) > 0 { msg.RawHex = fmt.Sprintf("%X", raw) }
	if duration > 0 { msg.Duration = fmt.Sprintf("%dms", duration.Milliseconds()) }

	fileLogger.Printf("[%s] %s - %s (Raw: %s, Dur: %s, Ptr: %v)", msg.Timestamp, msg.Level, msg.Message, msg.RawHex, msg.Duration, pv)

	logMutex.Lock()
	logBuffer = append(logBuffer, msg)
	if len(logBuffer) > 100 { logBuffer = logBuffer[1:] }
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
		Host: host, Port: port, UnitID: unitID, Endian: endian,
		Timeout: 60 * time.Second, TransactionID: 1,
	}
}

func (c *ModbusClient) Connect() error {
	address := net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port))
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil { return err }
	c.Conn = conn
	return nil
}

func (c *ModbusClient) Close() { if c.Conn != nil { c.Conn.Close() } }

func (c *ModbusClient) ReadHoldingRegisters(addr uint16, count uint16) ([]byte, error) {
	if c.Conn == nil { return nil, fmt.Errorf("no conectado") }
	c.Conn.SetDeadline(time.Now().Add(c.Timeout))

	req := make([]byte, 12)
	binary.BigEndian.PutUint16(req[0:], c.TransactionID) // Transaction ID
	binary.BigEndian.PutUint16(req[2:], 0)               // Protocol ID
	binary.BigEndian.PutUint16(req[4:], 6)               // Length
	req[6] = c.UnitID
	req[7] = 0x03
	binary.BigEndian.PutUint16(req[8:], addr)
	binary.BigEndian.PutUint16(req[10:], count)

	start := time.Now()
	broadcastLog("DEBUG", fmt.Sprintf("Request Modbus @%d (Qty:%d)", addr, count), req, 0, nil, "")

	_, err := c.Conn.Write(req)
	if err != nil { return nil, err }

	buf := make([]byte, 2048)
	n, err := c.Conn.Read(buf)
	elapsed := time.Since(start)

	if err != nil { return nil, err }
	if n < 9 { return nil, fmt.Errorf("respuesta corta") }
	if buf[7] >= 0x80 {
		broadcastLog("ERROR", fmt.Sprintf("Excepción Modbus 0x%X", buf[8]), buf[:n], elapsed, nil, "")
		return nil, fmt.Errorf("exception %d", buf[8])
	}

	broadcastLog("INFO", "Respuesta OK", buf[:n], elapsed, nil, "")
	c.TransactionID++
	return buf[9:n], nil
}

func (c *ModbusClient) DecodeFloat32(data []byte) []float32 {
	res := make([]float32, 0)
	for i := 0; i+4 <= len(data); i += 4 {
		var combined uint32
		b := data[i : i+4]
		switch c.Endian {
		case LittleEndian: combined = uint32(b[3])<<24 | uint32(b[2])<<16 | uint32(b[1])<<8 | uint32(b[0])
		case WordSwapped:  combined = uint32(b[2])<<24 | uint32(b[3])<<16 | uint32(b[0])<<8 | uint32(b[1])
		case ByteSwapped:  combined = uint32(b[1])<<24 | uint32(b[0])<<16 | uint32(b[3])<<8 | uint32(b[2])
		default:           combined = uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
		}
		res = append(res, math.Float32frombits(combined))
	}
	return res
}

func (c *ModbusClient) DecodeInt16(data []byte) []int16 {
	res := make([]int16, 0)
	for i := 0; i+2 <= len(data); i += 2 {
		res = append(res, int16(binary.BigEndian.Uint16(data[i:i+2])))
	}
	return res
}
