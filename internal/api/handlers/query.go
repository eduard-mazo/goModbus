package handlers

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"goModbus/internal/modbus"
)

type QueryRequest struct {
	IP           string           `json:"ip"`
	Port         int              `json:"port"`
	SlaveID      byte             `json:"slave_id"`
	FC           byte             `json:"fc"`
	StartAddress uint16           `json:"start_address"`
	Quantity     uint16           `json:"quantity"`
	WriteDataHex string           `json:"write_data_hex"`
	Endianness   modbus.Endianness `json:"endianness"`
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
	FC          byte                   `json:"fc"`
	RequestHex  string                 `json:"req_hex"`
	ResponseHex string                 `json:"res_hex"`
	ElapsedMs   int64                  `json:"elapsed_ms"`
	ByteCount   int                    `json:"byte_count"`
	Registers   []RegisterRow          `json:"registers,omitempty"`
	Floats      []float32              `json:"floats,omitempty"`
	FloatModes  []modbus.Float32Modes  `json:"float_modes,omitempty"`
	Coils       []bool                 `json:"coils,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

func QueryHandler(c *gin.Context) {
	var req QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Endianness == "" {
		req.Endianness = modbus.BigEndian
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

	client := modbus.NewModbusClient(req.IP, req.Port, req.SlaveID, req.Endianness)
	client.SID = c.GetHeader("X-Session-ID")
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
	case modbus.FCReadCoils, modbus.FCReadDiscreteInputs:
		resp.Coils = modbus.DecodeBits(data, req.Quantity)
	case modbus.FCReadHoldingRegisters, modbus.FCReadInputRegisters:
		resp.Registers = buildRegisterTable(data, req.StartAddress)
		resp.Floats = client.DecodeFloat32(data)
		for i, v := range resp.Floats {
			resp.Floats[i] = modbus.SanitizeFloat(v)
		}
		resp.FloatModes = modbus.DecodeAllModes(data)
		for i := range resp.FloatModes {
			resp.FloatModes[i].Sanitize()
		}
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
