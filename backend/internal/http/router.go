package handler

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/internal/refresh"
)

// ReconcileRunner abstracts the reconciliation engine.
type ReconcileRunner interface {
	Reconcile(ctx context.Context) []reconcile.ReconciliationRun
}

// NewRouter creates a Gin engine with all routes registered.
func NewRouter(cfg config.Config, refreshSvc *refresh.Service, store *desiredstate.Store, ackStore *reconcile.AckStore, reconciler ReconcileRunner, scheduler UpdateTrigger, api DockerAPI, broadcaster ...*desiredstate.Broadcaster) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/", RootHandler(cfg, api))
	var consoleBroadcaster *desiredstate.Broadcaster
	if len(broadcaster) > 0 {
		consoleBroadcaster = broadcaster[0]
	}
	registerConsole(r, cfg, store, reconciler, scheduler, consoleBroadcaster, api)

	// API routes
	if refreshSvc != nil {
		r.POST("/api/webhook", WebhookHandler(cfg, refreshSvc, consoleBroadcaster))
		r.POST("/api/refresh", ManualRefreshHandler(refreshSvc))
	}
	if scheduler != nil {
		r.POST("/api/update", TriggerUpdateHandler(scheduler))
		r.GET("/api/update/status", UpdateStatusHandler(scheduler))
	}
	if store != nil {
		r.GET("/api/refresh-status", RefreshStatusHandler(store))
		r.GET("/api/stacks", StacksHandler(store))
	}
	if ackStore != nil && reconciler != nil {
		r.POST("/api/reconcile/ack", AckHandler(ackStore, reconciler))
	}
	// Container listing for individual stacks
	var runtime *RuntimeReader
	if lister, ok := reconciler.(ContainerLister); ok && store != nil {
		inspector, _ := api.(docker.IdentityInspector)
		runtime = NewRuntimeReader(store, lister, inspector, consoleBroadcaster)
		r.GET("/api/stacks/containers/*path", ContainersHandler(runtime))
		r.GET("/api/runtime", func(c *gin.Context) {
			observations, err := runtime.Read(c.Request.Context())
			if err != nil {
				c.JSON(503, gin.H{"error": "Runtime observation unavailable"})
				return
			}
			c.JSON(200, observations)
		})
	}
	if len(broadcaster) > 0 && broadcaster[0] != nil && store != nil {
		r.GET("/api/events", func(c *gin.Context) {
			if runtime != nil {
				runtime.Reconnect()
			}
			EventsHandler(broadcaster[0], store)(c)
		})
	}

	return r
}
