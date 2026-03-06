package handlers

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"goModbus/internal/logger"
	"goModbus/internal/modbus"
)

// --- ROC Expert Handler ---

type ROCRequest struct {
	IP        string            `json:"ip"`
	Port      int               `json:"port"`
	SlaveID   byte              `json:"slave_id"`
	PtrEndian modbus.Endianness `json:"ptr_endian"`
	PtrAddr   uint16            `json:"ptr_addr"`
	PtrQty    uint16            `json:"ptr_qty"`
	DBEndian  modbus.Endianness `json:"db_endian"`
	DBAddr    uint16            `json:"db_addr"`
	Mode      string            `json:"mode"` // "ptr" | "hist" | "full"
	ManualPtr *float64          `json:"manual_ptr,omitempty"`
}

type ROCResponse struct {
	PointerValue float64               `json:"ptr_value"`
	PointerHex   string                `json:"ptr_hex"`
	PtrModes     []modbus.Float32Modes `json:"ptr_modes,omitempty"`
	DBHex        string                `json:"db_hex"`
	DBRegisters  []RegisterRow         `json:"db_registers,omitempty"`
	DBFloats     []float32             `json:"db_floats,omitempty"`
	DBModes      []modbus.Float32Modes `json:"db_modes,omitempty"`
	ElapsedMs    int64                 `json:"elapsed_ms"`
	Error        string                `json:"error,omitempty"`
}

func RocHandler(c *gin.Context) {
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

	sid := c.GetHeader("X-Session-ID")
	client := modbus.NewModbusClient(req.IP, req.Port, req.SlaveID, req.PtrEndian)
	client.SID = sid
	if err := client.Connect(); err != nil {
		c.JSON(http.StatusServiceUnavailable, ROCResponse{Error: err.Error()})
		return
	}
	defer client.Close()

	var resp ROCResponse
	var totalMs int64

	// Step 1 – Read pointer register
	if req.Mode == "ptr" || req.Mode == "full" {
		data, _, elapsed, err := client.Execute(modbus.FCReadHoldingRegisters, req.PtrAddr, req.PtrQty, nil)
		totalMs += elapsed.Milliseconds()
		if err != nil {
			resp.Error = "Error puntero: " + err.Error()
			c.JSON(http.StatusOK, resp)
			return
		}
		resp.PointerHex = fmt.Sprintf("%X", data)
		resp.PtrModes = modbus.DecodeAllModes(data)
		for i := range resp.PtrModes {
			resp.PtrModes[i].Sanitize()
		}

		if len(data) >= 4 {
			floats := client.DecodeFloat32(data)
			if len(floats) > 0 {
				resp.PointerValue = float64(modbus.SanitizeFloat(floats[0]))
			}
		} else if len(data) >= 2 {
			resp.PointerValue = float64(int16(binary.BigEndian.Uint16(data)))
		}
		logger.SessionBroadcast(sid, logger.LogMessage{Level: "INFO", Message: fmt.Sprintf("Puntero ROC leído: %.0f", resp.PointerValue), RawHex: fmt.Sprintf("%X", data), PointerValue: &resp.PointerValue})
	} else if req.Mode == "hist" && req.ManualPtr != nil {
		resp.PointerValue = *req.ManualPtr
	}

	// Step 2 – Read history block (ROC: addr=db_addr, qty=ptr_value)
	if req.Mode == "hist" || req.Mode == "full" {
		client.Endian = req.DBEndian
		data, _, elapsed, err := client.Execute(modbus.FCReadHoldingRegisters, req.DBAddr, uint16(resp.PointerValue), nil)
		totalMs += elapsed.Milliseconds()
		if err != nil {
			resp.Error = "Error histórico: " + err.Error()
			c.JSON(http.StatusOK, resp)
			return
		}
		resp.DBHex = fmt.Sprintf("%X", data)
		resp.DBRegisters = buildRegisterTable(data, req.DBAddr)
		resp.DBFloats = client.DecodeFloat32(data)
		for i, v := range resp.DBFloats {
			resp.DBFloats[i] = modbus.SanitizeFloat(v)
		}
		resp.DBModes = modbus.DecodeAllModes(data)
		for i := range resp.DBModes {
			resp.DBModes[i].Sanitize()
		}
		logger.SessionBroadcast(sid, logger.LogMessage{Level: "INFO", Message: "Histórico ROC leído", PointerValue: &resp.PointerValue, DataBlockHex: resp.DBHex})
	}

	resp.ElapsedMs = totalMs
	c.JSON(http.StatusOK, resp)
}

// --- ROC 24-Hour History Handler ---

type History24Request struct {
	IP          string            `json:"ip"`
	Port        int               `json:"port"`
	SlaveID     byte              `json:"slave_id"`
	PtrEndian   modbus.Endianness `json:"ptr_endian"`
	PtrAddr     uint16            `json:"ptr_addr"`
	PtrQty      uint16            `json:"ptr_qty"`
	DBEndian    modbus.Endianness `json:"db_endian"`
	DBAddr      uint16            `json:"db_addr"`
	DBQty       uint16            `json:"db_qty"`
	BufSize     uint16            `json:"buf_size"`
	CurrentHour *int              `json:"current_hour,omitempty"`
}

type History24Response struct {
	CurrentPtr  uint16              `json:"current_ptr"`
	CurrentHour int                 `json:"current_hour"`
	StartPtr    uint16              `json:"start_ptr"`
	Records     []modbus.HourRecord `json:"records"`
	ElapsedMs   int64               `json:"elapsed_ms"`
	Error       string              `json:"error,omitempty"`
}

func RocHistory24Handler(c *gin.Context) {
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

	sid := c.GetHeader("X-Session-ID")
	client := modbus.NewModbusClient(req.IP, req.Port, req.SlaveID, req.PtrEndian)
	client.SID = sid
	if err := client.Connect(); err != nil {
		c.JSON(http.StatusServiceUnavailable, History24Response{Error: err.Error()})
		return
	}
	defer client.Close()

	var resp History24Response
	var totalMs int64

	// Step 1 – Read current pointer
	ptrData, _, elapsed, err := client.Execute(modbus.FCReadHoldingRegisters, req.PtrAddr, req.PtrQty, nil)
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
	logger.SessionBroadcast(sid, logger.LogMessage{Level: "INFO", Message: fmt.Sprintf("History24: puntero actual = %d", currentPtr), RawHex: fmt.Sprintf("%X", ptrData)})

	// Step 2 – Determine current hour
	currentHour := time.Now().Hour()
	if req.CurrentHour != nil {
		currentHour = *req.CurrentHour
		logger.SessionBroadcast(sid, logger.LogMessage{Level: "INFO", Message: fmt.Sprintf("History24: hora manual = %d", currentHour)})
	} else if req.DBQty >= 3 {
		client.Endian = req.DBEndian
		recData, _, recElapsed, recErr := client.Execute(modbus.FCReadHoldingRegisters, req.DBAddr, currentPtr, nil)
		totalMs += recElapsed.Milliseconds()
		if recErr == nil {
			regs := client.DecodeUint16(recData)
			if len(regs) >= 3 {
				h := int(regs[2])
				if h >= 0 && h <= 23 {
					currentHour = h
					logger.SessionBroadcast(sid, logger.LogMessage{Level: "INFO", Message: fmt.Sprintf("History24: hora leída del dispositivo = %d", currentHour)})
				}
			}
		}
	} else {
		logger.SessionBroadcast(sid, logger.LogMessage{Level: "INFO", Message: fmt.Sprintf("History24: hora del sistema = %d", currentHour)})
	}
	resp.CurrentHour = currentHour

	// Step 3 – Calculate pointer for 00:00 of PREVIOUS day
	bufSize := int(req.BufSize)
	startPtr := (int(currentPtr) - currentHour - 24 + 2*bufSize) % bufSize
	resp.StartPtr = uint16(startPtr)
	logger.SessionBroadcast(sid, logger.LogMessage{Level: "INFO",
		Message: fmt.Sprintf("History24: ptr=%d hora=%d → ptr_ayer_00:00=%d  fórmula=(%d−%d−24+%d)%%%d",
			currentPtr, currentHour, startPtr, currentPtr, currentHour, 2*bufSize, bufSize)})

	// Step 4 – Fetch 24 hourly records in parallel (2 workers)
	resp.Records = make([]modbus.HourRecord, 24)
	start := time.Now()

	type job struct{ h int }
	jobs := make(chan job, 24)
	for h := 0; h < 24; h++ {
		jobs <- job{h}
	}
	close(jobs)

	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wClient := modbus.NewModbusClient(req.IP, req.Port, req.SlaveID, req.DBEndian)
			wClient.SID = sid
			if err := wClient.Connect(); err != nil {
				return
			}
			defer wClient.Close()

			for j := range jobs {
				ptr := uint16((startPtr + j.h) % bufSize)
				data, _, _, err := wClient.Execute(modbus.FCReadHoldingRegisters, req.DBAddr, ptr, nil)

				rec := modbus.HourRecord{Hour: j.h, Ptr: ptr}
				if err != nil {
					logger.SessionBroadcast(sid, logger.LogMessage{Level: "WARN", Message: fmt.Sprintf("History24 h=%02d ptr=%d: %s", j.h, ptr, err.Error())})
				} else {
					rec.Valid = true
					rec.Hex = fmt.Sprintf("%X", data)
					payload := data
					if len(data) >= 4 {
						payload = data[4:]
					}
					rec.Modes = modbus.DecodeAllModes(payload)
					for i := range rec.Modes {
						rec.Modes[i].Sanitize()
					}
					floats := wClient.DecodeFloat32(payload)
					if len(floats) > 0 {
						rec.Value = modbus.SanitizeFloat(floats[0])
					}
				}
				resp.Records[j.h] = rec
			}
		}()
	}
	wg.Wait()

	resp.ElapsedMs = time.Since(start).Milliseconds()
	logger.SessionBroadcast(sid, logger.LogMessage{Level: "INFO", Message: fmt.Sprintf("History24 completo: %dms (paralelo)", resp.ElapsedMs)})
	c.JSON(http.StatusOK, resp)
}
