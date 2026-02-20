package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/internal/refresh"
	"github.com/lucasreiners/docker-cd/internal/render"
)

// CommandRunner abstracts command execution so handler tests can stub it.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// RootHandler returns a Gin handler that renders the status page.
func RootHandler(runner CommandRunner, cfg config.Config) gin.HandlerFunc {
	// Build repo info from config (never includes token)
	var repo *render.RepoInfo
	if cfg.GitRepoURL != "" {
		repo = &render.RepoInfo{
			URL:       cfg.GitRepoURL,
			Revision:  cfg.GitRevision,
			DeployDir: cfg.GitDeployDir,
		}
	}

	return func(c *gin.Context) {
		client := docker.NewClient(runner, cfg.DockerSocket)
		status, err := client.ContainerCount(c.Request.Context())
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		page := render.StatusPage(cfg.ProjectName, status.RunningContainers, repo)
		c.String(http.StatusOK, page)
	}
}

// WebhookHandler handles GitHub webhook POST requests with optional HMAC validation.
func WebhookHandler(cfg config.Config, refreshSvc *refresh.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}

		// Validate HMAC signature if a webhook secret is configured
		if cfg.WebhookSecret != "" {
			sigHeader := c.GetHeader("X-Hub-Signature-256")
			if sigHeader == "" {
				log.Printf("[warn] webhook rejected: missing X-Hub-Signature-256 header")
				c.JSON(http.StatusUnauthorized, gin.H{"error": "missing signature header"})
				return
			}
			if !verifyHMAC(cfg.WebhookSecret, body, sigHeader) {
				log.Printf("[warn] webhook rejected: invalid HMAC signature")
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
				return
			}
		}

		result := refreshSvc.RequestRefresh(refresh.TriggerWebhook)
		log.Printf("[info] webhook refresh %s", string(result))
		c.JSON(http.StatusOK, gin.H{
			"status":  string(result),
			"message": "webhook refresh " + string(result),
		})
	}
}

// verifyHMAC checks the GitHub-style HMAC-SHA256 signature.
func verifyHMAC(secret string, payload []byte, sigHeader string) bool {
	sigHex := strings.TrimPrefix(sigHeader, "sha256=")
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := mac.Sum(nil)
	return hmac.Equal(sig, expected)
}

// ManualRefreshHandler handles POST /api/refresh to trigger a manual desired-state refresh.
func ManualRefreshHandler(refreshSvc *refresh.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result := refreshSvc.RequestRefresh(refresh.TriggerManual)
		log.Printf("[info] manual refresh %s", string(result))
		c.JSON(http.StatusOK, gin.H{
			"status":  string(result),
			"message": "manual refresh " + string(result),
		})
	}
}

// RefreshStatusHandler handles GET /api/refresh-status.
func RefreshStatusHandler(store *desiredstate.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := store.GetRefreshStatus()
		if snap == nil {
			snap = &desiredstate.Snapshot{
				RefreshStatus: desiredstate.RefreshStatusQueued,
			}
		}
		c.JSON(http.StatusOK, snap)
	}
}

// StacksHandler handles GET /api/stacks.
func StacksHandler(store *desiredstate.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		stacks := store.GetStacks()
		if stacks == nil {
			stacks = []desiredstate.StackRecord{}
		}
		c.JSON(http.StatusOK, stacks)
	}
}

// ackRequest is the JSON body for POST /api/reconcile/ack.
type ackRequest struct {
	StackPath string `json:"stack_path" binding:"required"`
}

// AckHandler handles POST /api/reconcile/ack to acknowledge drift for a stack
// and trigger immediate reconciliation.
func AckHandler(ackStore *reconcile.AckStore, reconciler ReconcileRunner) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ackRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "stack_path is required"})
			return
		}

		ackStore.Acknowledge(req.StackPath)
		log.Printf("[info] acknowledged drift for stack %s", req.StackPath)

		// Trigger immediate reconciliation
		runs := reconciler.Reconcile(c.Request.Context())

		result := "acknowledged"
		for _, run := range runs {
			if run.StackPath == req.StackPath {
				result = run.Result
				break
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"status":     result,
			"stack_path": req.StackPath,
			"message":    "drift acknowledged for " + req.StackPath,
		})
	}
}

// ContainerLister abstracts container listing for a stack.
type ContainerLister interface {
	GetContainers(ctx context.Context, stackPath string) ([]desiredstate.ContainerInfo, error)
}

// ContainersHandler handles GET /api/stacks/:path to list containers for a stack.
func ContainersHandler(lister ContainerLister) gin.HandlerFunc {
	return func(c *gin.Context) {
		stackPath := c.Param("path")
		if stackPath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "stack path is required"})
			return
		}
		// Strip leading slash from wildcard param
		stackPath = strings.TrimPrefix(stackPath, "/")

		containers, err := lister.GetContainers(c.Request.Context(), stackPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if containers == nil {
			containers = []desiredstate.ContainerInfo{}
		}
		c.JSON(http.StatusOK, containers)
	}
}

// EventsHandler handles GET /api/events as an SSE stream.
// It subscribes to the broadcaster and pushes events to the client.
func EventsHandler(broadcaster *desiredstate.Broadcaster, store *desiredstate.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		sub := broadcaster.Subscribe()
		defer broadcaster.Unsubscribe(sub)

		// Send initial snapshot of current stacks
		stacks := store.GetStacks()
		if stacks == nil {
			stacks = []desiredstate.StackRecord{}
		}
		broadcaster.PublishStackSnapshot(stacks)

		ctx := c.Request.Context()
		flusher := c.Writer

		for {
			select {
			case <-ctx.Done():
				return
			case <-sub.Done():
				return
			case event := <-sub.Events:
				fmt.Fprintf(flusher, "id: %s\nevent: %s\ndata: %s\n\n", event.ID, event.Type, event.Data)
				flusher.Flush()
			}
		}
	}
}
