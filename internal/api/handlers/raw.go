package handlers

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"goModbus/internal/logger"
	"goModbus/internal/modbus"
)

type RawRequest struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	HexFrame string `json:"hex_frame"`
}

type RawResponse struct {
	SentHex   string `json:"sent_hex"`
	RecvHex   string `json:"recv_hex,omitempty"`
	ElapsedMs int64  `json:"elapsed_ms"`
	Error     string `json:"error,omitempty"`
}

func RawHandler(c *gin.Context) {
	var req RawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	clean := strings.ReplaceAll(strings.ReplaceAll(req.HexFrame, " ", ""), ":", "")
	frame, err := hex.DecodeString(clean)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hex inválido: " + err.Error()})
		return
	}
	if len(frame) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("trama muy corta: %d bytes (mínimo 8)", len(frame))})
		return
	}

	sid := c.GetHeader("X-Session-ID")
	client := modbus.NewModbusClient(req.IP, req.Port, frame[6], modbus.BigEndian)
	client.SID = sid
	defer client.Close()

	recv, elapsed, sendErr := client.SendRaw(frame)
	resp := RawResponse{
		SentHex:   strings.ToUpper(hex.EncodeToString(frame)),
		ElapsedMs: elapsed.Milliseconds(),
	}
	if sendErr != nil {
		resp.Error = sendErr.Error()
	} else {
		resp.RecvHex = strings.ToUpper(hex.EncodeToString(recv))
	}

	logger.SessionBroadcast(sid, logger.LogMessage{
		Level:        "INFO",
		Message:      fmt.Sprintf("RAW → %s:%d  %d bytes  RTT %dms", req.IP, req.Port, len(frame), resp.ElapsedMs),
		RawHex:       strings.ToUpper(hex.EncodeToString(frame)),
		DataBlockHex: resp.RecvHex,
	})
	c.JSON(http.StatusOK, resp)
}
