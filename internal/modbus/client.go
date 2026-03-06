package modbus

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"time"
)

// LogFunc is called for Modbus-level events (TX/RX frames, connection events).
// sid is the session ID of the requesting client; empty string means broadcast to all.
// Set this at startup to wire the modbus package into the application logger.
var LogFunc func(sid, level, message string, raw []byte, duration time.Duration, pv *float64, dbh string)

// ModbusClient manages a single Modbus TCP connection
type ModbusClient struct {
	Host          string
	Port          int
	UnitID        byte
	Endian        Endianness
	Timeout       time.Duration
	Conn          net.Conn
	TransactionID uint16
	Silent        bool   // when true, suppresses TX/RX INFO/DEBUG logs (used during bulk sync)
	SID           string // session ID — routes logs only to the owning WebSocket client
}

func (c *ModbusClient) log(level, message string, raw []byte, duration time.Duration, pv *float64, dbh string) {
	if LogFunc != nil {
		LogFunc(c.SID, level, message, raw, duration, pv, dbh)
	}
}

func NewModbusClient(host string, port int, unitID byte, endian Endianness) *ModbusClient {
	return &ModbusClient{
		Host:          host,
		Port:          port,
		UnitID:        unitID,
		Endian:        endian,
		Timeout:       15 * time.Second,
		TransactionID: 1,
	}
}

func (c *ModbusClient) Connect() error {
	address := net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port))
	conn, err := net.DialTimeout("tcp", address, 15*time.Second)
	if err != nil {
		c.log("ERROR", fmt.Sprintf("TCP fallo al conectar %s: %v", address, err), nil, 0, nil, "")
		return fmt.Errorf("conexión a %s falló: %w", address, err)
	}
	c.Conn = conn
	if !c.Silent {
		c.log("INFO", fmt.Sprintf("Conectado a %s (UnitID=%d)", address, c.UnitID), nil, 0, nil, "")
	}
	return nil
}

func (c *ModbusClient) Close() {
	if c.Conn != nil {
		c.Conn.Close()
		c.log("INFO", "Conexión cerrada", nil, 0, nil, "")
	}
}

// SendRaw sends raw bytes as-is and returns (responseBytes, elapsed, error).
func (c *ModbusClient) SendRaw(frame []byte) ([]byte, time.Duration, error) {
	if err := c.Connect(); err != nil {
		return nil, 0, err
	}
	start := time.Now()
	c.Conn.SetDeadline(time.Now().Add(c.Timeout))
	if _, err := c.Conn.Write(frame); err != nil {
		elapsed := time.Since(start)
		c.log("ERROR", "RAW TX: "+err.Error(), nil, elapsed, nil, "")
		return nil, elapsed, err
	}
	buf := make([]byte, 512)
	n, err := c.Conn.Read(buf)
	elapsed := time.Since(start)
	if err != nil {
		c.log("ERROR", "RAW RX: "+err.Error(), nil, elapsed, nil, "")
		return nil, elapsed, err
	}
	return buf[:n], elapsed, nil
}

// Execute sends a Modbus request and returns (data, sentBytes, elapsed, error).
// data is the payload after MBAP+FC+ByteCount headers.
func (c *ModbusClient) Execute(fc byte, addr uint16, qty uint16, writeData []byte) ([]byte, []byte, time.Duration, error) {
	if c.Conn == nil {
		return nil, nil, 0, fmt.Errorf("no conectado")
	}

	pdu := buildPDU(fc, addr, qty, writeData)
	if pdu == nil {
		return nil, nil, 0, fmt.Errorf("función 0x%02X no soportada", fc)
	}
	req := buildMBAP(c.TransactionID, c.UnitID, pdu)

	c.Conn.SetDeadline(time.Now().Add(c.Timeout))
	start := time.Now()

	if !c.Silent {
		c.log("DEBUG", fmt.Sprintf("TX FC=0x%02X addr=%d qty=%d", fc, addr, qty), req, 0, nil, "")
	}

	if _, err := c.Conn.Write(req); err != nil {
		c.log("ERROR", fmt.Sprintf("TX error FC=0x%02X addr=%d: %v", fc, addr, err), nil, 0, nil, "")
		return nil, req, 0, fmt.Errorf("error TX: %w", err)
	}

	buf := make([]byte, 4096)
	n, err := c.Conn.Read(buf)
	elapsed := time.Since(start)

	if err != nil {
		c.log("ERROR", fmt.Sprintf("RX error FC=0x%02X addr=%d [%dms]: %v", fc, addr, elapsed.Milliseconds(), err), nil, elapsed, nil, "")
		return nil, req, elapsed, fmt.Errorf("error RX: %w", err)
	}

	resp := buf[:n]

	if n < 8 {
		c.log("ERROR", fmt.Sprintf("Respuesta incompleta FC=0x%02X: %d bytes (mín 8)", fc, n), resp, elapsed, nil, "")
		return nil, req, elapsed, fmt.Errorf("respuesta incompleta (%d bytes)", n)
	}

	// Check exception: high bit of FC byte set
	if resp[7] >= 0x80 {
		code := byte(0)
		if n > 8 {
			code = resp[8]
		}
		desc := ModbusExceptionDesc[code]
		if desc == "" {
			desc = "Error desconocido"
		}
		c.log("ERROR", fmt.Sprintf("Excepción Modbus 0x%02X: %s", code, desc), resp, elapsed, nil, "")
		return nil, req, elapsed, fmt.Errorf("excepción 0x%02X: %s", code, desc)
	}

	if !c.Silent {
		c.log("INFO", fmt.Sprintf("RX OK %dms | %d bytes datos", elapsed.Milliseconds(), n), resp, elapsed, nil, "")
	}
	c.TransactionID++

	// Data payload starts after MBAP(7) + FC(1) + ByteCount(1) = byte 9
	if n >= 9 {
		return resp[9:n], req, elapsed, nil
	}
	return resp[8:n], req, elapsed, nil
}

// ReadHoldingRegisters is a convenience wrapper (FC03)
func (c *ModbusClient) ReadHoldingRegisters(addr, count uint16) ([]byte, error) {
	data, _, _, err := c.Execute(FCReadHoldingRegisters, addr, count, nil)
	return data, err
}

// DecodeFloat32 decodes raw bytes as IEEE 754 float32 using the client's endianness.
func (c *ModbusClient) DecodeFloat32(data []byte) []float32 {
	res := make([]float32, 0, len(data)/4)
	for i := 0; i+4 <= len(data); i += 4 {
		b := data[i : i+4]
		var combined uint32
		switch c.Endian {
		case LittleEndian:
			combined = uint32(b[3])<<24 | uint32(b[2])<<16 | uint32(b[1])<<8 | uint32(b[0])
		case WordSwapped:
			combined = uint32(b[2])<<24 | uint32(b[3])<<16 | uint32(b[0])<<8 | uint32(b[1])
		case ByteSwapped:
			combined = uint32(b[1])<<24 | uint32(b[0])<<16 | uint32(b[3])<<8 | uint32(b[2])
		default: // BigEndian abcd
			combined = uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
		}
		res = append(res, math.Float32frombits(combined))
	}
	return res
}

// DecodeUint16 decodes raw bytes as big-endian uint16 values.
func (c *ModbusClient) DecodeUint16(data []byte) []uint16 {
	res := make([]uint16, 0, len(data)/2)
	for i := 0; i+2 <= len(data); i += 2 {
		res = append(res, uint16(data[i])<<8|uint16(data[i+1]))
	}
	return res
}

// DecodeInt16 decodes raw bytes as big-endian int16 values.
func (c *ModbusClient) DecodeInt16(data []byte) []int16 {
	res := make([]int16, 0, len(data)/2)
	for i := 0; i+2 <= len(data); i += 2 {
		res = append(res, int16(binary.BigEndian.Uint16(data[i:i+2])))
	}
	return res
}
