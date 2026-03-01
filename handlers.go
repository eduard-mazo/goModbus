package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// wsClientConn holds a WebSocket connection with its own send channel
type wsClientConn struct {
	conn *websocket.Conn
	ch   chan LogMessage
}

var logClients = make(map[*wsClientConn]bool)

// wsHandler upgrades HTTP to WebSocket and streams log messages
func wsHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &wsClientConn{conn: conn, ch: make(chan LogMessage, 128)}

	// Replay history BEFORE registering the client.
	// If we register first, a message that arrives during the replay enters both
	// logBuffer (sent via history loop) and client.ch (sent via broadcaster) → duplicate.
	logMutex.Lock()
	history := make([]LogMessage, len(logBuffer))
	copy(history, logBuffer)
	logMutex.Unlock()
	for _, msg := range history {
		if err := conn.WriteJSON(msg); err != nil {
			return
		}
	}

	// Now register to receive live messages.
	clientsMu.Lock()
	logClients[client] = true
	clientsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(logClients, client)
		clientsMu.Unlock()
		close(client.ch)
		conn.Close()
	}()

	// Stream new messages until client disconnects
	for msg := range client.ch {
		if err := conn.WriteJSON(msg); err != nil {
			return
		}
	}
}

// getConfigHandler returns station presets from config.yaml
func getConfigHandler(c *gin.Context) {
	cfg, _ := loadConfig(configPath)
	c.JSON(http.StatusOK, cfg)
}

// --- Generic Modbus Query ---

type QueryRequest struct {
	IP           string     `json:"ip"`
	Port         int        `json:"port"`
	SlaveID      byte       `json:"slave_id"`
	FC           byte       `json:"fc"`
	StartAddress uint16     `json:"start_address"`
	Quantity     uint16     `json:"quantity"`
	WriteDataHex string     `json:"write_data_hex"`
	Endianness   Endianness `json:"endianness"`
}

type RegisterRow struct {
	Index   int    `json:"i"`
	Address uint16 `json:"addr"`
	Hex     string `json:"hex"`
	Dec     uint16 `json:"dec"`
	SDec    int16  `json:"sdec"`
	Bin     string `json:"bin"`
}

type QueryResponse struct {
	FC          byte           `json:"fc"`
	RequestHex  string         `json:"req_hex"`
	ResponseHex string         `json:"res_hex"`
	ElapsedMs   int64          `json:"elapsed_ms"`
	ByteCount   int            `json:"byte_count"`
	Registers   []RegisterRow  `json:"registers,omitempty"`
	Floats      []float32      `json:"floats,omitempty"`
	FloatModes  []Float32Modes `json:"float_modes,omitempty"` // all 4 endianness decodings
	Coils       []bool         `json:"coils,omitempty"`
	Error       string         `json:"error,omitempty"`
}

func queryHandler(c *gin.Context) {
	var req QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Endianness == "" {
		req.Endianness = BigEndian
	}
	if req.Port == 0 {
		req.Port = 502
	}

	var writeData []byte
	if req.WriteDataHex != "" {
		clean := strings.ReplaceAll(strings.ReplaceAll(req.WriteDataHex, " ", ""), "0x", "")
		var err error
		writeData, err = hex.DecodeString(clean)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "write_data_hex inválido: " + err.Error()})
			return
		}
	}

	client := NewModbusClient(req.IP, req.Port, req.SlaveID, req.Endianness)
	if err := client.Connect(); err != nil {
		c.JSON(http.StatusServiceUnavailable, QueryResponse{Error: err.Error()})
		return
	}
	defer client.Close()

	data, rawReq, elapsed, err := client.Execute(req.FC, req.StartAddress, req.Quantity, writeData)

	resp := QueryResponse{
		FC:         req.FC,
		RequestHex: fmt.Sprintf("%X", rawReq),
		ElapsedMs:  elapsed.Milliseconds(),
	}

	if err != nil {
		resp.Error = err.Error()
		c.JSON(http.StatusOK, resp)
		return
	}

	resp.ResponseHex = fmt.Sprintf("%X", data)
	resp.ByteCount = len(data)

	switch req.FC {
	case FCReadCoils, FCReadDiscreteInputs:
		resp.Coils = DecodeBits(data, req.Quantity)
	case FCReadHoldingRegisters, FCReadInputRegisters:
		resp.Registers = buildRegisterTable(data, req.StartAddress)
		resp.Floats = client.DecodeFloat32(data)
		resp.FloatModes = DecodeAllModes(data)
	}

	c.JSON(http.StatusOK, resp)
}

func buildRegisterTable(data []byte, baseAddr uint16) []RegisterRow {
	rows := make([]RegisterRow, 0, len(data)/2)
	for i := 0; i+2 <= len(data); i += 2 {
		v := binary.BigEndian.Uint16(data[i : i+2])
		idx := i / 2
		rows = append(rows, RegisterRow{
			Index:   idx,
			Address: baseAddr + uint16(idx),
			Hex:     fmt.Sprintf("%04X", v),
			Dec:     v,
			SDec:    int16(v),
			Bin:     fmt.Sprintf("%016b", v),
		})
	}
	return rows
}

// --- ROC Expert Handler ---

type ROCRequest struct {
	IP        string     `json:"ip"`
	Port      int        `json:"port"`
	SlaveID   byte       `json:"slave_id"`
	PtrEndian Endianness `json:"ptr_endian"`
	PtrAddr   uint16     `json:"ptr_addr"`
	PtrQty    uint16     `json:"ptr_qty"`
	DBEndian  Endianness `json:"db_endian"`
	DBAddr    uint16     `json:"db_addr"`
	Mode      string     `json:"mode"` // "ptr" | "hist" | "full"
	ManualPtr *float64   `json:"manual_ptr,omitempty"`
}

type ROCResponse struct {
	PointerValue float64        `json:"ptr_value"`
	PointerHex   string         `json:"ptr_hex"`
	PtrModes     []Float32Modes `json:"ptr_modes,omitempty"` // all 4 endianness decodings of pointer
	DBHex        string         `json:"db_hex"`
	DBRegisters  []RegisterRow  `json:"db_registers,omitempty"`
	DBFloats     []float32      `json:"db_floats,omitempty"`
	DBModes      []Float32Modes `json:"db_modes,omitempty"` // all 4 endianness decodings of history block
	ElapsedMs    int64          `json:"elapsed_ms"`
	Error        string         `json:"error,omitempty"`
}

func rocHandler(c *gin.Context) {
	var req ROCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Port == 0 {
		req.Port = 502
	}
	if req.PtrQty == 0 {
		req.PtrQty = 2
	}

	client := NewModbusClient(req.IP, req.Port, req.SlaveID, req.PtrEndian)
	if err := client.Connect(); err != nil {
		c.JSON(http.StatusServiceUnavailable, ROCResponse{Error: err.Error()})
		return
	}
	defer client.Close()

	var resp ROCResponse
	var totalMs int64

	// Step 1 – Read pointer register
	if req.Mode == "ptr" || req.Mode == "full" {
		data, _, elapsed, err := client.Execute(FCReadHoldingRegisters, req.PtrAddr, req.PtrQty, nil)
		totalMs += elapsed.Milliseconds()
		if err != nil {
			resp.Error = "Error puntero: " + err.Error()
			c.JSON(http.StatusOK, resp)
			return
		}
		resp.PointerHex = fmt.Sprintf("%X", data)
		resp.PtrModes = DecodeAllModes(data)

		if len(data) >= 4 {
			floats := client.DecodeFloat32(data)
			if len(floats) > 0 {
				resp.PointerValue = float64(floats[0])
			}
		} else if len(data) >= 2 {
			resp.PointerValue = float64(int16(binary.BigEndian.Uint16(data)))
		}
		broadcastLog("INFO", fmt.Sprintf("Puntero ROC leído: %.0f", resp.PointerValue), data, 0, &resp.PointerValue, "")
	} else if req.Mode == "hist" && req.ManualPtr != nil {
		resp.PointerValue = *req.ManualPtr
	}

	// Step 2 – Read history block (ROC: always Qty=1 at db_addr + pointer)
	if req.Mode == "hist" || req.Mode == "full" {
		client.Endian = req.DBEndian
		histAddr := req.DBAddr + uint16(resp.PointerValue)
		data, _, elapsed, err := client.Execute(FCReadHoldingRegisters, histAddr, 1, nil)
		totalMs += elapsed.Milliseconds()
		if err != nil {
			resp.Error = "Error histórico: " + err.Error()
			c.JSON(http.StatusOK, resp)
			return
		}
		resp.DBHex = fmt.Sprintf("%X", data)
		resp.DBRegisters = buildRegisterTable(data, histAddr)
		resp.DBFloats = client.DecodeFloat32(data)
		resp.DBModes = DecodeAllModes(data)
		broadcastLog("INFO", "Histórico ROC leído", data, 0, &resp.PointerValue, resp.DBHex)
	}

	resp.ElapsedMs = totalMs
	c.JSON(http.StatusOK, resp)
}

// --- ROC 24-Hour History Handler ---

// History24Request fetches 24 consecutive hourly records from a ROC circular buffer.
//
// Circular buffer math (size = buf_size, default 840):
//
//	startPtr = (currentPtr - currentHour + bufSize) % bufSize   → pointer for 00:00
//	ptr[h]   = (startPtr + h) % bufSize                         → pointer for hour h
//
// Example: ptr=1, hour=02:00 → startPtr=(1-2+840)%840=839 → ptr[0]=839, ptr[1]=840%840=0, ptr[2]=1
type History24Request struct {
	IP          string     `json:"ip"`
	Port        int        `json:"port"`
	SlaveID     byte       `json:"slave_id"`
	PtrEndian   Endianness `json:"ptr_endian"`
	PtrAddr     uint16     `json:"ptr_addr"`
	PtrQty      uint16     `json:"ptr_qty"`
	DBEndian    Endianness `json:"db_endian"`
	DBAddr      uint16     `json:"db_addr"`
	DBQty       uint16     `json:"db_qty"`   // registers per record (2 = float32, ≥3 reads hour from device)
	BufSize     uint16     `json:"buf_size"` // circular buffer total slots (default 840)
	CurrentHour *int       `json:"current_hour,omitempty"` // nil → read from device if DBQty≥3, else system clock
}

type HourRecord struct {
	Hour  int     `json:"hour"`
	Ptr   uint16  `json:"ptr"`
	Hex   string  `json:"hex"`
	Value float32 `json:"value"`
	Valid bool    `json:"valid"`
}

type History24Response struct {
	CurrentPtr  uint16       `json:"current_ptr"`
	CurrentHour int          `json:"current_hour"`
	StartPtr    uint16       `json:"start_ptr"` // pointer for 00:00
	Records     []HourRecord `json:"records"`
	ElapsedMs   int64        `json:"elapsed_ms"`
	Error       string       `json:"error,omitempty"`
}

func rocHistory24Handler(c *gin.Context) {
	var req History24Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Port == 0 {
		req.Port = 502
	}
	if req.PtrQty == 0 {
		req.PtrQty = 2
	}
	if req.DBQty == 0 {
		req.DBQty = 2
	}
	if req.BufSize == 0 {
		req.BufSize = 840
	}

	client := NewModbusClient(req.IP, req.Port, req.SlaveID, req.PtrEndian)
	if err := client.Connect(); err != nil {
		c.JSON(http.StatusServiceUnavailable, History24Response{Error: err.Error()})
		return
	}
	defer client.Close()

	var resp History24Response
	var totalMs int64

	// Step 1 – Read current pointer
	ptrData, _, elapsed, err := client.Execute(FCReadHoldingRegisters, req.PtrAddr, req.PtrQty, nil)
	totalMs += elapsed.Milliseconds()
	if err != nil {
		resp.Error = "Error leyendo puntero: " + err.Error()
		c.JSON(http.StatusOK, resp)
		return
	}

	var currentPtr uint16
	if len(ptrData) >= 4 {
		floats := client.DecodeFloat32(ptrData)
		if len(floats) > 0 {
			currentPtr = uint16(floats[0])
		}
	} else if len(ptrData) >= 2 {
		currentPtr = uint16(int16(binary.BigEndian.Uint16(ptrData)))
	}
	resp.CurrentPtr = currentPtr

	broadcastLog("INFO", fmt.Sprintf("History24: puntero actual = %d", currentPtr), ptrData, elapsed, nil, "")

	// Step 2 – Determine current hour
	currentHour := time.Now().Hour()
	if req.CurrentHour != nil {
		// Explicit override from UI
		currentHour = *req.CurrentHour
		broadcastLog("INFO", fmt.Sprintf("History24: hora manual = %d", currentHour), nil, 0, nil, "")
	} else if req.DBQty >= 3 {
		// Read current record and extract hour from register index 2 (month=0, day=1, hour=2)
		client.Endian = req.DBEndian
		recData, _, recElapsed, recErr := client.Execute(FCReadHoldingRegisters, req.DBAddr+currentPtr, req.DBQty, nil)
		totalMs += recElapsed.Milliseconds()
		if recErr == nil {
			regs := client.DecodeUint16(recData)
			if len(regs) >= 3 {
				h := int(regs[2])
				if h >= 0 && h <= 23 {
					currentHour = h
					broadcastLog("INFO", fmt.Sprintf("History24: hora leída del dispositivo = %d", currentHour), recData, recElapsed, nil, "")
				}
			}
		}
	} else {
		broadcastLog("INFO", fmt.Sprintf("History24: hora del sistema = %d", currentHour), nil, 0, nil, "")
	}
	resp.CurrentHour = currentHour

	// Step 3 – Calculate pointer for 00:00 of current day
	//   startPtr = (currentPtr - currentHour + BufSize) % BufSize
	bufSize := int(req.BufSize)
	startPtr := (int(currentPtr) - currentHour + bufSize) % bufSize
	resp.StartPtr = uint16(startPtr)

	broadcastLog("INFO",
		fmt.Sprintf("History24: ptr=%d hora=%d → ptr_00:00=%d (buf=%d)", currentPtr, currentHour, startPtr, bufSize),
		nil, 0, nil, "")

	// Step 4 – Fetch 24 hourly records
	client.Endian = req.DBEndian
	resp.Records = make([]HourRecord, 24)

	for h := 0; h < 24; h++ {
		ptr := uint16((startPtr + h) % bufSize)
		addr := req.DBAddr + ptr

		data, _, recElapsed, recErr := client.Execute(FCReadHoldingRegisters, addr, req.DBQty, nil)
		totalMs += recElapsed.Milliseconds()

		rec := HourRecord{Hour: h, Ptr: ptr}
		if recErr != nil {
			broadcastLog("WARN", fmt.Sprintf("History24 h=%02d ptr=%d: %s", h, ptr, recErr.Error()), nil, 0, nil, "")
		} else {
			rec.Valid = true
			rec.Hex = fmt.Sprintf("%X", data)
			floats := client.DecodeFloat32(data)
			if len(floats) > 0 {
				rec.Value = floats[0]
			}
		}
		resp.Records[h] = rec
	}

	resp.ElapsedMs = totalMs
	broadcastLog("INFO", fmt.Sprintf("History24 completo: %dms", totalMs), nil, 0, nil, "")
	c.JSON(http.StatusOK, resp)
}
