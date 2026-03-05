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
