package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"time"
)

// Endianness identifiers (Modbus byte/word order)
type Endianness string

const (
	BigEndian    Endianness = "abcd" // Standard Big-Endian
	LittleEndian Endianness = "dcba" // Full Little-Endian
	WordSwapped  Endianness = "cdab" // Word-Swapped (common in ROC devices)
	ByteSwapped  Endianness = "badc" // Byte-Swapped
)

// Modbus Function Codes
const (
	FCReadCoils            byte = 0x01
	FCReadDiscreteInputs   byte = 0x02
	FCReadHoldingRegisters byte = 0x03
	FCReadInputRegisters   byte = 0x04
	FCWriteSingleCoil      byte = 0x05
	FCWriteSingleRegister  byte = 0x06
	FCWriteMultipleCoils   byte = 0x0F
	FCWriteMultipleRegs    byte = 0x10
)

// ModbusExceptionDesc maps exception codes to human-readable descriptions
var ModbusExceptionDesc = map[byte]string{
	0x01: "Función no soportada",
	0x02: "Dirección fuera de rango",
	0x03: "Valor de dato incorrecto",
	0x04: "Fallo en dispositivo esclavo",
	0x05: "Confirmación (procesando)",
	0x06: "Esclavo ocupado",
	0x08: "Error de paridad en memoria",
	0x0A: "Gateway - ruta no disponible",
	0x0B: "Gateway - dispositivo no responde",
}

// ModbusClient manages a single Modbus TCP connection
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
	address := net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port))
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		broadcastLog("ERROR", fmt.Sprintf("TCP fallo al conectar %s: %v", address, err), nil, 0, nil, "")
		return fmt.Errorf("conexión a %s falló: %w", address, err)
	}
	c.Conn = conn
	broadcastLog("INFO", fmt.Sprintf("Conectado a %s (UnitID=%d)", address, c.UnitID), nil, 0, nil, "")
	return nil
}

func (c *ModbusClient) Close() {
	if c.Conn != nil {
		c.Conn.Close()
		broadcastLog("INFO", "Conexión cerrada", nil, 0, nil, "")
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
		broadcastLog("ERROR", "RAW TX: "+err.Error(), nil, elapsed, nil, "")
		return nil, elapsed, err
	}
	buf := make([]byte, 512)
	n, err := c.Conn.Read(buf)
	elapsed := time.Since(start)
	if err != nil {
		broadcastLog("ERROR", "RAW RX: "+err.Error(), nil, elapsed, nil, "")
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

	broadcastLog("DEBUG", fmt.Sprintf("TX FC=0x%02X addr=%d qty=%d", fc, addr, qty), req, 0, nil, "")

	if _, err := c.Conn.Write(req); err != nil {
		broadcastLog("ERROR", fmt.Sprintf("TX error FC=0x%02X addr=%d: %v", fc, addr, err), nil, 0, nil, "")
		return nil, req, 0, fmt.Errorf("error TX: %w", err)
	}

	buf := make([]byte, 4096)
	n, err := c.Conn.Read(buf)
	elapsed := time.Since(start)

	if err != nil {
		broadcastLog("ERROR", fmt.Sprintf("RX error FC=0x%02X addr=%d [%dms]: %v", fc, addr, elapsed.Milliseconds(), err), nil, elapsed, nil, "")
		return nil, req, elapsed, fmt.Errorf("error RX: %w", err)
	}

	resp := buf[:n]

	if n < 8 {
		broadcastLog("ERROR", fmt.Sprintf("Respuesta incompleta FC=0x%02X: %d bytes (mín 8)", fc, n), resp, elapsed, nil, "")
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
		broadcastLog("ERROR", fmt.Sprintf("Excepción Modbus 0x%02X: %s", code, desc), resp, elapsed, nil, "")
		return nil, req, elapsed, fmt.Errorf("excepción 0x%02X: %s", code, desc)
	}

	broadcastLog("INFO", fmt.Sprintf("RX OK %dms | %d bytes datos", elapsed.Milliseconds(), n), resp, elapsed, nil, "")
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

// buildPDU constructs the PDU (FC + data) for the given function code
func buildPDU(fc byte, addr, qty uint16, data []byte) []byte {
	switch fc {
	case FCReadCoils, FCReadDiscreteInputs, FCReadHoldingRegisters, FCReadInputRegisters:
		pdu := make([]byte, 5)
		pdu[0] = fc
		binary.BigEndian.PutUint16(pdu[1:], addr)
		binary.BigEndian.PutUint16(pdu[3:], qty)
		return pdu

	case FCWriteSingleCoil:
		pdu := make([]byte, 5)
		pdu[0] = fc
		binary.BigEndian.PutUint16(pdu[1:], addr)
		if len(data) > 0 && data[0] != 0 {
			pdu[3], pdu[4] = 0xFF, 0x00 // ON
		} // else 0x00 0x00 = OFF (zero value)
		return pdu

	case FCWriteSingleRegister:
		pdu := make([]byte, 5)
		pdu[0] = fc
		binary.BigEndian.PutUint16(pdu[1:], addr)
		if len(data) >= 2 {
			pdu[3], pdu[4] = data[0], data[1]
		}
		return pdu

	case FCWriteMultipleCoils:
		byteCount := (qty + 7) / 8
		pdu := make([]byte, 6+byteCount)
		pdu[0] = fc
		binary.BigEndian.PutUint16(pdu[1:], addr)
		binary.BigEndian.PutUint16(pdu[3:], qty)
		pdu[5] = byte(byteCount)
		copy(pdu[6:], data)
		return pdu

	case FCWriteMultipleRegs:
		byteCount := qty * 2
		pdu := make([]byte, 6+byteCount)
		pdu[0] = fc
		binary.BigEndian.PutUint16(pdu[1:], addr)
		binary.BigEndian.PutUint16(pdu[3:], qty)
		pdu[5] = byte(byteCount)
		copy(pdu[6:], data)
		return pdu
	}
	return nil
}

// buildMBAP builds the full Modbus TCP Application Data Unit
func buildMBAP(txID uint16, unitID byte, pdu []byte) []byte {
	// ADU = MBAP(6) + UnitID(1) + PDU → total 7+len(pdu) bytes
	// MBAP(6) alone does NOT include the Unit ID byte; using 6+len(pdu) left
	// only 4 destination bytes for a 5-byte PDU, silently dropping the last byte.
	req := make([]byte, 7+len(pdu))
	binary.BigEndian.PutUint16(req[0:], txID)               // Transaction ID
	binary.BigEndian.PutUint16(req[2:], 0)                  // Protocol ID = 0
	binary.BigEndian.PutUint16(req[4:], uint16(1+len(pdu))) // Length = UnitID(1) + PDU
	req[6] = unitID
	copy(req[7:], pdu)
	return req
}

// --- Decoders ---

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

func (c *ModbusClient) DecodeUint16(data []byte) []uint16 {
	res := make([]uint16, 0, len(data)/2)
	for i := 0; i+2 <= len(data); i += 2 {
		res = append(res, binary.BigEndian.Uint16(data[i:i+2]))
	}
	return res
}

func (c *ModbusClient) DecodeInt16(data []byte) []int16 {
	res := make([]int16, 0, len(data)/2)
	for i := 0; i+2 <= len(data); i += 2 {
		res = append(res, int16(binary.BigEndian.Uint16(data[i:i+2])))
	}
	return res
}

// Float32Modes holds one float32 value decoded under all four endianness conventions.
type Float32Modes struct {
	ABCD float32 `json:"abcd"` // Big-Endian
	DCBA float32 `json:"dcba"` // Little-Endian
	CDAB float32 `json:"cdab"` // Word-Swap (ROC default)
	BADC float32 `json:"badc"` // Byte-Swap
}

// Sanitize replaces NaN or Inf values with 0 to prevent JSON marshal errors.
func (f *Float32Modes) Sanitize() {
	f.ABCD = sanitizeFloat(f.ABCD)
	f.DCBA = sanitizeFloat(f.DCBA)
	f.CDAB = sanitizeFloat(f.CDAB)
	f.BADC = sanitizeFloat(f.BADC)
}

func sanitizeFloat(v float32) float32 {
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return 0
	}
	return v
}

// DecodeAllModes decodes each 4-byte group in data under all four endianness modes.
func DecodeAllModes(data []byte) []Float32Modes {
	out := make([]Float32Modes, 0, len(data)/4)
	for i := 0; i+4 <= len(data); i += 4 {
		b := data[i : i+4]
		out = append(out, Float32Modes{
			ABCD: math.Float32frombits(uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])),
			DCBA: math.Float32frombits(uint32(b[3])<<24 | uint32(b[2])<<16 | uint32(b[1])<<8 | uint32(b[0])),
			CDAB: math.Float32frombits(uint32(b[2])<<24 | uint32(b[3])<<16 | uint32(b[0])<<8 | uint32(b[1])),
			BADC: math.Float32frombits(uint32(b[1])<<24 | uint32(b[0])<<16 | uint32(b[3])<<8 | uint32(b[2])),
		})
	}
	return out
}

// DecodeBits unpacks bit values from coil response bytes
func DecodeBits(data []byte, qty uint16) []bool {
	bits := make([]bool, qty)
	for i := uint16(0); i < qty && int(i/8) < len(data); i++ {
		bits[i] = (data[i/8]>>(i%8))&1 == 1
	}
	return bits
}
