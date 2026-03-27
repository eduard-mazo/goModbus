package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"goModbus/internal/config"
)

// GetConfigHandler returns station presets from config.yaml
func GetConfigHandler(c *gin.Context) {
	cfg, _ := config.LoadConfig(config.ConfigPath)
	c.JSON(http.StatusOK, cfg)
}

// SaveConfigHandler writes the posted Config back to config.yaml
func SaveConfigHandler(c *gin.Context) {
	var cfg config.Config
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := config.SaveConfig(config.ConfigPath, &cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
