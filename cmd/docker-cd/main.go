package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/docker"
	gitval "github.com/lucasreiners/docker-cd/internal/git"
	handler "github.com/lucasreiners/docker-cd/internal/http"
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

	router := handler.NewRouter(runner, cfg)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Docker-CD starting on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
