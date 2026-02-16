package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
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

	// Refresh settings
	WebhookSecret       string
	RefreshPollInterval time.Duration

	// Reconcile settings
	ReconcileEnabled       bool
	ReconcileRemoveEnabled bool
	DriftPolicy            string // "revert" or "flag"
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

	webhookSecret := os.Getenv("WEBHOOK_SECRET")

	var refreshPollInterval time.Duration
	if v := os.Getenv("REFRESH_POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil && d > 0 {
			refreshPollInterval = d
		}
	}

	reconcileEnabled := true
	if v := os.Getenv("RECONCILE_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			reconcileEnabled = b
		}
	}

	reconcileRemoveEnabled := false
	if v := os.Getenv("RECONCILE_REMOVE_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			reconcileRemoveEnabled = b
		}
	}

	driftPolicy := "revert"
	if v := os.Getenv("DRIFT_POLICY"); v != "" {
		v = strings.ToLower(v)
		if v == "revert" || v == "flag" {
			driftPolicy = v
		}
	}

	cfg := Config{
		Port:                   port,
		ProjectName:            projectName,
		DockerSocket:           dockerSocket,
		GitRepoURL:             gitRepoURL,
		GitAccessToken:         gitAccessToken,
		GitRevision:            gitRevision,
		GitDeployDir:           gitDeployDir,
		WebhookSecret:          webhookSecret,
		RefreshPollInterval:    refreshPollInterval,
		ReconcileEnabled:       reconcileEnabled,
		ReconcileRemoveEnabled: reconcileRemoveEnabled,
		DriftPolicy:            driftPolicy,
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
