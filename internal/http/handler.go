package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/docker"
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
