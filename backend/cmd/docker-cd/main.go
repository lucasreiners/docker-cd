package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	"github.com/lucasreiners/docker-cd/internal/events"
	gitval "github.com/lucasreiners/docker-cd/internal/git"
	handler "github.com/lucasreiners/docker-cd/internal/http"
	"github.com/lucasreiners/docker-cd/internal/operations"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/internal/refresh"
	"github.com/lucasreiners/docker-cd/internal/scheduler"
)

func main() {
	// Initialize structured logger
	logLevel := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}

	var logger *slog.Logger
	if os.Getenv("LOG_FORMAT") == "json" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	}
	slog.SetDefault(logger)

	logger.Info("docker-cd starting", "version", "dev")

	cfg, errs := config.Load()
	if len(errs) > 0 {
		for _, e := range errs {
			logger.Error("config error", "error", e)
		}
		logger.Error("startup aborted", "error_count", len(errs))
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Error("config validation failed", "error", err)
		os.Exit(1)
	}

	// Validate read-only repository access
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	lister := &gitval.GoGitRemoteLister{}
	result := gitval.Validate(ctx, lister, cfg.GitRepoURL, cfg.GitAccessToken, cfg.GitRevision, cfg.GitDeployDir)
	if !result.Success {
		logger.Error("repository validation failed", "error", result.Error)
		os.Exit(1)
	}
	logger.Info("repository validated",
		"url", cfg.GitRepoURL,
		"revision", cfg.GitRevision)

	// Count folders in the deploy directory (or repo root)
	deployPath := cfg.GitDeployDir
	treeLister := &gitval.GoGitTreeLister{}
	dirCount, err := treeLister.CountDirs(ctx, cfg.GitRepoURL, cfg.GitAccessToken, cfg.GitRevision, deployPath)
	if err != nil {
		logger.Warn("could not count folders in deploy dir", "error", err)
	} else {
		dir := deployPath
		if dir == "" {
			dir = "/"
		}
		logger.Info("found folders in deploy directory", "count", dirCount, "path", dir)
	}

	runner := &docker.ExecRunner{}

	// Initialize Docker SDK client
	sdkClient, err := docker.NewSDKClient(cfg.DockerSocket)
	if err != nil {
		logger.Error("failed to create Docker SDK client", "error", err)
		os.Exit(1)
	}

	// Initialize desired-state refresh pipeline
	store := desiredstate.NewStore()
	broadcaster := desiredstate.NewBroadcaster()
	history, err := operations.Open(filepath.Join(cfg.DataDir, "operations.json"))
	if err != nil {
		logger.Error("cannot open operation history", "error", err)
		os.Exit(1)
	}
	broadcaster.SetOperations(history)
	queue := refresh.NewQueue()
	localClone := gitval.NewLocalClone(gitval.DefaultLocalRepoPath, cfg.GitRepoURL, cfg.GitAccessToken, cfg.GitRevision)
	composeReader := gitval.NewLocalComposeReader(localClone)
	refreshSvc := refresh.NewService(cfg, store, queue, composeReader)
	refreshSvc.SetBroadcaster(broadcaster)

	// Initialize event bus and wire up event handlers
	eventBus := events.NewEventBus(logger)
	events.ForwardToSSE(eventBus, broadcaster, store)

	// Initialize reconciler
	policy := reconcile.ReconciliationPolicy{
		Enabled:        cfg.ReconcileEnabled,
		RemoveEnabled:  cfg.ReconcileRemoveEnabled,
		DriftPolicy:    cfg.DriftPolicy,
		MaxConcurrency: 1,
	}
	dockerClient := docker.NewClient(sdkClient)
	composeRunner := reconcile.NewDockerComposeRunner(runner, cfg.DockerSocket)
	inspector := reconcile.NewDockerContainerInspector(dockerClient)
	ackStore := reconcile.NewAckStore()

	// Initialize drift detector and state manager
	driftDetector := reconcile.NewDriftDetector(cfg.GitDeployDir, logger)
	stateManager := reconcile.NewStateManager(store, composeRunner, eventBus, logger)

	reconciler := reconcile.NewReconciler(store, policy, composeRunner, inspector, ackStore, cfg.GitDeployDir, driftDetector, stateManager, logger, &dockerImagePuller{client: dockerClient}, broadcaster)

	// Wire reconciler into refresh pipeline
	refreshSvc.SetReconcileFunc(func(ctx context.Context) {
		runs := reconciler.Reconcile(ctx)
		for _, run := range runs {
			logger.Info("reconcile completed",
				"stack", run.StackPath,
				"result", run.Result)
		}
	})

	// Initialize scheduler service (if enabled)
	schedulerSvc, err := scheduler.NewSchedulerService(cfg, logger, store, dockerClient, reconciler, broadcaster)
	if err != nil {
		logger.Error("scheduler initialization failed", "error", err)
		os.Exit(1)
	}

	refreshSvc.SetCancelFuncs(reconciler.CancelActive, func() {
		if schedulerSvc != nil {
			schedulerSvc.CancelActiveUpdate()
		}
	})

	// Create context with signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start scheduler service in background (if enabled)
	if schedulerSvc != nil {
		go schedulerSvc.Start(ctx)
	}

	// Start background refresh loop
	go refreshSvc.Start(ctx)

	router := handler.NewRouter(cfg, refreshSvc, store, ackStore, reconciler, schedulerSvc, sdkClient, broadcaster)

	addr := fmt.Sprintf(":%d", cfg.Port)
	logger.Info("http server starting", "addr", addr)

	// Run server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := router.Run(addr); err != nil {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, stopping gracefully")

		// Create shutdown context with timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// Gracefully stop scheduler service
		if schedulerSvc != nil {
			if err := schedulerSvc.Stop(shutdownCtx); err != nil {
				logger.Error("scheduler shutdown error", "error", err)
			}
		}

		logger.Info("shutdown complete")
	case err := <-errChan:
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

// dockerImagePuller adapts docker.Client to reconcile.ImagePuller.
type dockerImagePuller struct {
	client *docker.Client
}

func (d *dockerImagePuller) PullImages(ctx context.Context, images []string, onProgress reconcile.PullProgressFn) error {
	var dockerProgress docker.PullProgressFn
	if onProgress != nil {
		dockerProgress = func(p docker.PullProgress) {
			onProgress(reconcile.PullProgress{
				Image:        p.Image,
				Status:       p.Status,
				Progress:     p.Progress,
				Current:      p.Current,
				Total:        p.Total,
				LayerID:      p.LayerID,
				BytesCurrent: p.BytesCurrent,
				BytesTotal:   p.BytesTotal,
				Done:         p.Done,
			})
		}
	}
	return d.client.PullImages(ctx, images, dockerProgress)
}
