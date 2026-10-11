package handler

import (
	"context"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/operations"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
)

func publicURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func registerConsole(r *gin.Engine, cfg config.Config, store *desiredstate.Store, runner ReconcileRunner, scheduler UpdateTrigger, b *desiredstate.Broadcaster, api DockerAPI) {
	version := "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	lister, canInspect := runner.(ContainerLister)
	_, canLogs := api.(containerLogger)
	if canLogs && canInspect && store != nil {
		r.GET("/api/container-logs", containerLogsHandler(api, lister, store))
	}
	reviewed, canApply := runner.(interface {
		ReconcileReviewed(context.Context, string, string, bool) reconcile.ReconciliationRun
	})
	r.GET("/api/instance", func(c *gin.Context) {
		expression, zone, next := cfg.UpdaterCron, time.Local.String(), time.Time{}
		if effective, ok := scheduler.(interface {
			EffectiveSchedule() (string, string, time.Time)
		}); ok {
			expression, zone, next = effective.EffectiveSchedule()
		}
		persistent, storageError := false, ""
		if b != nil {
			persistent, storageError = b.Operations().Health()
		}
		c.JSON(http.StatusOK, gin.H{
			"name": cfg.ProjectName, "version": version,
			"repository": gin.H{
				"url": publicURL(cfg.GitRepoURL), "ref": cfg.GitRevision, "deployDir": cfg.GitDeployDir,
				"pollInterval": cfg.RefreshPollInterval.String(), "webhookConfigured": cfg.WebhookSecret != "", "credentialsConfigured": cfg.GitAccessToken != "",
			},
			"dockerEndpoint":      publicURL(cfg.DockerSocket),
			"policies":            gin.H{"reconcile": cfg.ReconcileEnabled, "remove": cfg.ReconcileRemoveEnabled, "drift": cfg.DriftPolicy},
			"schedule":            gin.H{"enabled": cfg.UpdaterEnabled, "cron": expression, "timezone": zone, "nextRun": next},
			"capabilities":        gin.H{"inspect": canInspect, "logs": canLogs && canInspect, "apply": canApply && cfg.ReconcileEnabled, "imageUpdates": cfg.UpdaterEnabled, "history": b != nil, "configuration": store != nil},
			"history":             gin.H{"persistent": persistent, "error": storageError, "retention": 1000},
			"configurationSource": "environment",
		})
	})
	if store != nil {
		r.GET("/api/stack/configuration", func(c *gin.Context) {
			for _, stack := range store.GetStacks() {
				if stack.Path == c.Query("path") {
					var applied *operations.Source
					if b != nil {
						if source, ok := b.Operations().AppliedSource(stack.Path); ok {
							applied = &source
						}
					}
					c.JSON(http.StatusOK, gin.H{"path": stack.Path, "file": stack.ComposeFile, "revision": stack.DesiredRevision, "content": string(stack.Content), "applied": applied})
					return
				}
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "Stack not found"})
		})
	}
	if canInspect && store != nil {
		r.GET("/api/runtime", func(c *gin.Context) {
			out := make(map[string]interface{})
			ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
			defer cancel()
			for _, stack := range store.GetStacks() {
				containers, err := lister.GetContainers(ctx, stack.Path)
				var message *string
				if err != nil {
					s := err.Error()
					message = &s
				}
				if containers == nil {
					containers = []desiredstate.ContainerInfo{}
				}
				out[stack.Path] = gin.H{"containers": containers, "error": message, "observedAt": time.Now().UTC()}
			}
			c.JSON(http.StatusOK, out)
		})
	}
	if b == nil {
		return
	}
	r.GET("/api/operations", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		if limit < 1 || limit > 100 {
			limit = 50
		}
		records := b.Operations().List()
		filtered := make([]operations.Record, 0)
		cursor := c.Query("cursor")
		past := cursor == ""
		for _, record := range records {
			if !past {
				if record.ID == cursor {
					past = true
				}
				continue
			}
			if stack := c.Query("stack"); stack != "" && record.Stack != stack {
				continue
			}
			if kind := c.Query("kind"); kind != "" && record.Kind != kind {
				continue
			}
			if status := c.Query("status"); status != "" && record.Status != status {
				continue
			}
			filtered = append(filtered, record)
		}
		next := ""
		if len(filtered) > limit {
			filtered = filtered[:limit]
			next = filtered[len(filtered)-1].ID
		}
		persistent, err := b.Operations().Health()
		c.JSON(http.StatusOK, gin.H{"records": filtered, "nextCursor": next, "persistent": persistent, "storageError": err})
	})
	r.GET("/api/operations/:id", func(c *gin.Context) {
		if record, ok := b.Operations().Get(c.Param("id")); ok {
			c.JSON(http.StatusOK, record)
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "Operation not found or expired"})
		}
	})
	var actionMu sync.Mutex
	active := make(map[string]bool)
	r.POST("/api/operations", func(c *gin.Context) {
		var body struct {
			Action   string `json:"action"`
			Stack    string `json:"stack"`
			Revision string `json:"revision"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Stack == "" || body.Revision == "" {
			c.JSON(400, gin.H{"error": "action, stack and reviewed revision are required"})
			return
		}
		if body.Action != "retry" && body.Action != "acknowledge" && body.Action != "update" {
			c.JSON(400, gin.H{"error": "Unknown stack action"})
			return
		}
		if !canApply || !cfg.ReconcileEnabled || store == nil {
			c.JSON(409, gin.H{"error": "Reconciliation is disabled"})
			return
		}
		if body.Action == "update" && !cfg.UpdaterEnabled {
			c.JSON(409, gin.H{"error": "Image updates are disabled"})
			return
		}
		snap := store.Get()
		if snap == nil || snap.UpdatesBlocked || snap.RefreshStatus != desiredstate.RefreshStatusCompleted {
			c.JSON(409, gin.H{"error": "Repository refresh must complete before deployment"})
			return
		}
		if snap.Revision != body.Revision {
			c.JSON(409, gin.H{"error": "Desired revision changed; review the stack again"})
			return
		}
		var target *desiredstate.StackRecord
		for i := range snap.Stacks {
			if snap.Stacks[i].Path == body.Stack {
				target = &snap.Stacks[i]
				break
			}
		}
		if target == nil {
			c.JSON(404, gin.H{"error": "Stack not found"})
			return
		}
		if target.RequiresAcknowledgement && body.Action != "acknowledge" {
			c.JSON(409, gin.H{"error": "This stack requires drift acknowledgement"})
			return
		}
		actionMu.Lock()
		if active[body.Stack] {
			actionMu.Unlock()
			c.JSON(409, gin.H{"error": "A stack action is already running"})
			return
		}
		active[body.Stack] = true
		actionMu.Unlock()
		ctx := b.BeginOperation(operations.WithTrigger(context.Background(), body.Action), "action", body.Action, body.Stack, body.Revision)
		c.JSON(http.StatusAccepted, gin.H{"operationId": operations.ID(ctx)})
		go func() {
			defer func() { actionMu.Lock(); delete(active, body.Stack); actionMu.Unlock() }()
			run := reviewed.ReconcileReviewed(ctx, body.Stack, body.Revision, body.Action == "acknowledge")
			status, message := "completed", "Stack action completed"
			if run.Result != "success" {
				status = "failed"
				message = run.Error
			}
			b.EndOperation(ctx, status, message)
		}()
	})
}
