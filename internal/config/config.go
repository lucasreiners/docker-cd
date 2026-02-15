package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config holds runtime configuration for the Docker-CD service.
type Config struct {
	Port         int
	ProjectName  string
	DockerSocket string

	// Git repository settings
	GitRepoURL     string
	GitAccessToken string
	GitRevision    string
	GitDeployDir   string
}

// Load reads configuration from environment variables, falling back to defaults.
// It returns the config and a slice of validation errors for required git fields.
func Load() (Config, []string) {
	port := 8080
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			port = p
		}
	}

	projectName := "Docker-CD"
	if v := os.Getenv("PROJECT_NAME"); v != "" {
		projectName = v
	}

	dockerSocket := "/var/run/docker.sock"
	if v := os.Getenv("DOCKER_SOCKET"); v != "" {
		dockerSocket = v
	}

	gitRepoURL := os.Getenv("GIT_REPO_URL")
	gitAccessToken := os.Getenv("GIT_ACCESS_TOKEN")
	gitRevision := os.Getenv("GIT_REVISION")
	gitDeployDir := os.Getenv("GIT_DEPLOY_DIR")

	cfg := Config{
		Port:           port,
		ProjectName:    projectName,
		DockerSocket:   dockerSocket,
		GitRepoURL:     gitRepoURL,
		GitAccessToken: gitAccessToken,
		GitRevision:    gitRevision,
		GitDeployDir:   gitDeployDir,
	}

	var errs []string
	if cfg.GitRepoURL == "" {
		errs = append(errs, "GIT_REPO_URL is required")
	} else {
		u, err := url.Parse(cfg.GitRepoURL)
		if err != nil || !strings.EqualFold(u.Scheme, "https") {
			errs = append(errs, fmt.Sprintf("GIT_REPO_URL must be an HTTPS URL, got %q", cfg.GitRepoURL))
		}
	}
	if cfg.GitAccessToken == "" {
		errs = append(errs, "GIT_ACCESS_TOKEN is required")
	}
	if cfg.GitRevision == "" {
		errs = append(errs, "GIT_REVISION is required")
	}

	return cfg, errs
}
