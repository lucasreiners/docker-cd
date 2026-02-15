package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/refresh"
)

// NewRouter creates a Gin engine with all routes registered.
func NewRouter(runner CommandRunner, cfg config.Config, refreshSvc *refresh.Service, store *desiredstate.Store) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/", RootHandler(runner, cfg))

	// API routes
	if refreshSvc != nil {
		r.POST("/api/webhook", WebhookHandler(cfg, refreshSvc))
		r.POST("/api/refresh", ManualRefreshHandler(refreshSvc))
	}
	if store != nil {
		r.GET("/api/refresh-status", RefreshStatusHandler(store))
		r.GET("/api/stacks", StacksHandler(store))
	}

	return r
}
