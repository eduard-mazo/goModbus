package api

import (
	"github.com/gin-gonic/gin"
	"goModbus/internal/api/handlers"
)

// RegisterRoutes attaches all API and WebSocket routes to the Gin engine.
func RegisterRoutes(r *gin.Engine) {
	r.GET("/ws", handlers.WsHandler)

	api := r.Group("/api")
	{
		api.GET("/config", handlers.GetConfigHandler)
		api.POST("/config/save", handlers.SaveConfigHandler)
		api.POST("/stations/full-sync", handlers.FullSyncHandler)
		api.POST("/stations/partial-sync", handlers.PartialSyncHandler)
		api.POST("/stations/load-db", handlers.LoadFromDBHandler)
		api.POST("/query", handlers.QueryHandler)
		api.POST("/roc", handlers.RocHandler)
		api.POST("/roc/history24", handlers.RocHistory24Handler)
		api.POST("/raw", handlers.RawHandler)
	}
}
