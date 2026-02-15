package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/lucasreiners/docker-cd/internal/config"
)

// NewRouter creates a Gin engine with all routes registered.
func NewRouter(runner CommandRunner, cfg config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/", RootHandler(runner, cfg))

	return r
}
