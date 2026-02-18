package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	gitval "github.com/lucasreiners/docker-cd/internal/git"
	handler "github.com/lucasreiners/docker-cd/internal/http"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/internal/refresh"
)

func main() {
	cfg, errs := config.Load()
	if len(errs) > 0 {
		for _, e := range errs {
			log.Printf("config error: %s", e)
		}
		log.Fatalf("startup aborted: %d config error(s)", len(errs))
	}

	// Validate read-only repository access
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	lister := &gitval.GoGitRemoteLister{}
	result := gitval.Validate(ctx, lister, cfg.GitRepoURL, cfg.GitAccessToken, cfg.GitRevision, cfg.GitDeployDir)
	if !result.Success {
		log.Fatalf("repository validation failed: %v", result.Error)
	}
	log.Printf("repository validated: %s @ %s", cfg.GitRepoURL, cfg.GitRevision)

	// Count folders in the deploy directory (or repo root)
	deployPath := cfg.GitDeployDir
	treeLister := &gitval.GoGitTreeLister{}
	dirCount, err := treeLister.CountDirs(ctx, cfg.GitRepoURL, cfg.GitAccessToken, cfg.GitRevision, deployPath)
	if err != nil {
		log.Printf("warning: could not count folders in deploy dir: %v", err)
	} else {
		dir := deployPath
		if dir == "" {
			dir = "/"
		}
		log.Printf("found %d folder(s) in %s", dirCount, dir)
	}

	runner := &docker.ExecRunner{}

	// Initialize desired-state refresh pipeline
	store := desiredstate.NewStore()
	broadcaster := desiredstate.NewBroadcaster()
	queue := refresh.NewQueue()
	composeReader := &gitval.GoGitComposeReader{}
	refreshSvc := refresh.NewService(cfg, store, queue, composeReader)
	refreshSvc.SetBroadcaster(broadcaster)

	// Initialize reconciler
	policy := reconcile.ReconciliationPolicy{
		Enabled:        cfg.ReconcileEnabled,
		RemoveEnabled:  cfg.ReconcileRemoveEnabled,
		DriftPolicy:    cfg.DriftPolicy,
		MaxConcurrency: 1,
	}
	dockerClient := docker.NewClient(runner, cfg.DockerSocket)
	composeRunner := reconcile.NewDockerComposeRunner(runner, cfg.DockerSocket)
	inspector := reconcile.NewDockerContainerInspector(dockerClient)
	ackStore := reconcile.NewAckStore()
	reconciler := reconcile.NewReconciler(store, policy, composeRunner, inspector, ackStore, cfg.GitDeployDir)
	reconciler.SetBroadcaster(broadcaster)

	// Wire reconciler into refresh pipeline
	refreshSvc.SetReconcileFunc(func(ctx context.Context) {
		runs := reconciler.Reconcile(ctx)
		for _, run := range runs {
			log.Printf("[info] reconcile: stack=%s result=%s", run.StackPath, run.Result)
		}
	})

	// Start background refresh loop (startup + periodic polling)
	go refreshSvc.Start(context.Background())

	router := handler.NewRouter(runner, cfg, refreshSvc, store, ackStore, reconciler, broadcaster)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Docker-CD starting on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
