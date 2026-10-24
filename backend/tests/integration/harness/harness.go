//go:build integration

// Package harness runs the production Git/refresh/reconcile/HTTP pipeline against
// a disposable Git repository and a private DinD daemon. No host workloads are used.
package harness

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	"github.com/lucasreiners/docker-cd/internal/events"
	gitreader "github.com/lucasreiners/docker-cd/internal/git"
	handler "github.com/lucasreiners/docker-cd/internal/http"
	"github.com/lucasreiners/docker-cd/internal/operations"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/internal/refresh"
	"github.com/lucasreiners/docker-cd/internal/scheduler"
	"github.com/lucasreiners/docker-cd/tests/integration/dind"
)

const Secret = "integration-webhook-secret-not-a-real-credential"
const Image = "busybox:1.38.0"

func Compose(name string) string {
	return fmt.Sprintf("services:\n  worker:\n    image: %s\n    container_name: %s\n    command: [sleep, '3600']\n    stop_grace_period: 1s\n    volumes: [data:/data]\nvolumes:\n  data: {}\n", Image, name)
}

type Harness struct {
	Env         *dind.Env
	Store       *desiredstate.Store
	Broadcaster *desiredstate.Broadcaster
	Refresh     *refresh.Service
	Queue       *refresh.Queue
	Router      *gin.Engine
	Client      *docker.Client
	api         *dind.AuthenticatedClient
	Gate        *Gate
	SlowPull    *SlowPull
	Revision    string
	root        string
	repo        *gogit.Repository
	mu          sync.Mutex
	startOnce   sync.Once
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	ctx         context.Context
	updater     *scheduler.SchedulerService
}

func New(ctx context.Context) (_ *Harness, err error) {
	startup, stopStartup := context.WithTimeout(ctx, 3*time.Minute)
	defer stopStartup()
	h := &Harness{Store: desiredstate.NewStore(), Broadcaster: desiredstate.NewBroadcaster()}
	h.ctx, h.cancel = context.WithCancel(ctx)
	defer func() {
		if err != nil {
			h.Close()
		}
	}()
	h.root, err = os.MkdirTemp("", "docker-cd-integration-*")
	if err != nil {
		return nil, err
	}
	history, err := operations.Open(filepath.Join(h.root, "operations.json"))
	if err != nil {
		return nil, err
	}
	h.Broadcaster.SetOperations(history)
	h.repo, err = gogit.PlainInitWithOptions(filepath.Join(h.root, "remote"), &gogit.PlainInitOptions{InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}})
	if err != nil {
		return nil, err
	}
	web, cache := Compose("web-v1"), Compose("cache-v1")
	h.Revision, err = h.Commit(map[string]*string{"apps/web/compose.yaml": &web, "infra/cache/compose.yaml": &cache}, "initial deployment")
	if err != nil {
		return nil, err
	}
	h.Env, err = dind.Start(startup)
	if err != nil {
		return nil, err
	}
	api, err := dind.NewSDKClient(h.Env.DockerHost)
	if err != nil {
		return nil, err
	}
	h.api = api
	warmClient := docker.NewClient(api)
	// Warm the private daemon once; assertions do not depend on registry latency.
	if err = warmClient.PullImages(startup, []string{Image}, nil); err != nil {
		return nil, err
	}
	h.SlowPull = &SlowPull{DockerAPI: api}
	h.Client = docker.NewClient(h.SlowPull)
	cfg := config.Config{GitRepoURL: filepath.Join(h.root, "remote"), GitRevision: "main", WebhookSecret: Secret,
		ReconcileEnabled: true, ReconcileRemoveEnabled: true, DriftPolicy: "revert", UpdaterEnabled: true, UpdaterCron: "0 3 * * *"}
	reader := gitreader.NewLocalComposeReader(gitreader.NewLocalClone(filepath.Join(h.root, "clone"), cfg.GitRepoURL, "", "main"))
	h.Queue = refresh.NewQueue()
	h.Refresh = refresh.NewService(cfg, h.Store, h.Queue, reader)
	h.Refresh.SetBroadcaster(h.Broadcaster)
	bus := events.NewEventBus(slog.Default())
	events.ForwardToSSE(bus, h.Broadcaster, h.Store)
	h.Gate = &Gate{ComposeRunner: reconcile.NewDockerComposeRunner(&docker.ExecRunner{}, h.Env.DockerHost)}
	state := reconcile.NewStateManager(h.Store, h.Gate, bus, slog.Default())
	ackStore := reconcile.NewAckStore()
	r := reconcile.NewReconciler(h.Store, reconcile.ReconciliationPolicy{Enabled: true, RemoveEnabled: true, DriftPolicy: "revert"},
		h.Gate, reconcile.NewDockerContainerInspector(h.Client), ackStore, "",
		reconcile.NewDriftDetector("", slog.Default()), state, slog.Default(), puller{h.Client}, h.Broadcaster)
	h.Refresh.SetReconcileFunc(func(ctx context.Context) { r.Reconcile(ctx) })
	h.updater, err = scheduler.NewSchedulerService(cfg, slog.Default(), h.Store, h.Client, r, h.Broadcaster)
	if err != nil {
		return nil, err
	}
	h.Refresh.SetCancelFuncs(r.CancelActive, h.updater.CancelActiveUpdate)
	h.Router = handler.NewRouter(cfg, h.Refresh, h.Store, ackStore, r, h.updater, api, h.Broadcaster)
	return h, nil
}

// Start is explicit so a browser can connect before the initial refresh.
func (h *Harness) Start() {
	h.startOnce.Do(func() {
		h.wg.Add(1)
		go func() { defer h.wg.Done(); h.Refresh.Start(h.ctx) }()
	})
}

func (h *Harness) Close() {
	if h.cancel != nil {
		h.cancel()
	}
	if h.updater != nil {
		h.updater.CancelActiveUpdate()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = h.updater.Stop(ctx)
	}
	h.wg.Wait()
	if h.api != nil {
		_ = h.api.Close()
	}
	if h.Env != nil {
		h.Env.Cleanup()
	}
	if h.root != "" {
		_ = os.RemoveAll(h.root)
	}
}

// Commit changes the actual origin repository. nil values delete tracked files.
func (h *Harness) Commit(files map[string]*string, message string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	wt, err := h.repo.Worktree()
	if err != nil {
		return "", err
	}
	for path, content := range files {
		if !filepath.IsLocal(path) {
			return "", fmt.Errorf("invalid fixture path %q", path)
		}
		if content == nil {
			if _, err := wt.Remove(path); err != nil {
				return "", err
			}
			continue
		}
		full := filepath.Join(h.root, "remote", path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(full, []byte(*content), 0644); err != nil {
			return "", err
		}
		if _, err := wt.Add(path); err != nil {
			return "", err
		}
	}
	hash, err := wt.Commit(message, &gogit.CommitOptions{Author: &object.Signature{Name: "Integration Test", Email: "test@example.invalid", When: time.Now()}})
	return hash.String(), err
}

func Signature(body []byte) string {
	mac := hmac.New(sha256.New, []byte(Secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Gate pauses immediately before real ComposeUp, making transient syncing states
// deterministic without mocking Docker results, SSE messages, or elapsed sleeps.
type Gate struct {
	reconcile.ComposeRunner
	mu      sync.Mutex
	release chan struct{}
	waiting bool
}

func (g *Gate) Arm() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.release == nil {
		g.release = make(chan struct{})
	}
}
func (g *Gate) Release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.release != nil {
		close(g.release)
		g.release = nil
	}
}
func (g *Gate) Waiting() bool { g.mu.Lock(); defer g.mu.Unlock(); return g.waiting }
func (g *Gate) ComposeUp(ctx context.Context, project, file, override, dir string) error {
	g.mu.Lock()
	release := g.release
	g.waiting = release != nil
	g.mu.Unlock()
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
		}
		g.mu.Lock()
		g.waiting = false
		g.mu.Unlock()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return g.ComposeRunner.ComposeUp(ctx, project, file, override, dir)
}

type puller struct{ client *docker.Client }

func (p puller) PullImages(ctx context.Context, images []string, progress reconcile.PullProgressFn) error {
	return p.client.PullImages(ctx, images, func(v docker.PullProgress) {
		if progress != nil {
			progress(reconcile.PullProgress{Image: v.Image, Status: v.Status, Progress: v.Progress, Current: v.Current, Total: v.Total,
				LayerID: v.LayerID, BytesCurrent: v.BytesCurrent, BytesTotal: v.BytesTotal, Done: v.Done})
		}
	})
}

// Test controls are registered only by the integration-tagged test server.
// Callers must bind it to loopback; this is never part of the production binary.
func (h *Harness) EnableControls() {
	h.Router.POST("/test/pull/arm", func(c *gin.Context) { h.SlowPull.Arm(c.Query("fail") == "true"); c.Status(http.StatusOK) })
	h.Router.POST("/test/pull/advance", func(c *gin.Context) { h.SlowPull.Advance(); c.Status(http.StatusOK) })
	h.Router.POST("/test/periodic", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": h.Refresh.RequestRefresh(refresh.TriggerPeriodic)})
	})
	h.Router.GET("/test/idle", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"idle": !h.Queue.IsRunning() && h.updater.GetUpdateStatus() == nil})
	})
	h.Router.GET("/test/ready", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"revision": h.Revision}) })
	h.Router.POST("/test/start", func(c *gin.Context) { h.Start(); c.Status(http.StatusAccepted) })
	h.Router.POST("/test/gate/arm", func(c *gin.Context) { h.Gate.Arm(); c.Status(http.StatusOK) })
	h.Router.POST("/test/gate/release", func(c *gin.Context) { h.Gate.Release(); c.Status(http.StatusOK) })
	h.Router.GET("/test/gate", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"waiting": h.Gate.Waiting()}) })
	h.Router.POST("/test/commit", func(c *gin.Context) {
		var req struct {
			Files   map[string]*string `json:"files"`
			Message string             `json:"message"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		rev, err := h.Commit(req.Files, req.Message)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"revision": rev})
	})
}
