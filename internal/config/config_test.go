package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	os.Unsetenv("PORT")
	os.Unsetenv("PROJECT_NAME")
	os.Unsetenv("DOCKER_SOCKET")

	// Set required git fields to avoid validation errors
	t.Setenv("GIT_REPO_URL", "https://github.com/example/repo.git")
	t.Setenv("GIT_ACCESS_TOKEN", "tok")
	t.Setenv("GIT_REVISION", "main")

	cfg, errs := config.Load()

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Port)
	}
	if cfg.ProjectName != "Docker-CD" {
		t.Errorf("expected default project name Docker-CD, got %q", cfg.ProjectName)
	}
	if cfg.DockerSocket != "/var/run/docker.sock" {
		t.Errorf("expected default socket /var/run/docker.sock, got %q", cfg.DockerSocket)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("PROJECT_NAME", "TestProject")
	t.Setenv("DOCKER_SOCKET", "/tmp/docker.sock")
	t.Setenv("GIT_REPO_URL", "https://github.com/example/repo.git")
	t.Setenv("GIT_ACCESS_TOKEN", "tok")
	t.Setenv("GIT_REVISION", "main")

	cfg, errs := config.Load()

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.ProjectName != "TestProject" {
		t.Errorf("expected project name TestProject, got %q", cfg.ProjectName)
	}
	if cfg.DockerSocket != "/tmp/docker.sock" {
		t.Errorf("expected socket /tmp/docker.sock, got %q", cfg.DockerSocket)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	t.Setenv("PORT", "notanumber")
	t.Setenv("GIT_REPO_URL", "https://github.com/example/repo.git")
	t.Setenv("GIT_ACCESS_TOKEN", "tok")
	t.Setenv("GIT_REVISION", "main")

	cfg, _ := config.Load()

	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080 for invalid PORT env, got %d", cfg.Port)
	}
}

func TestLoad_MissingGitRepoURL(t *testing.T) {
	t.Setenv("GIT_ACCESS_TOKEN", "tok")
	t.Setenv("GIT_REVISION", "main")
	os.Unsetenv("GIT_REPO_URL")

	_, errs := config.Load()

	found := false
	for _, e := range errs {
		if e == "GIT_REPO_URL is required" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected GIT_REPO_URL required error, got %v", errs)
	}
}

func TestLoad_NonHTTPSRepoURL(t *testing.T) {
	t.Setenv("GIT_REPO_URL", "git@github.com:org/repo.git")
	t.Setenv("GIT_ACCESS_TOKEN", "tok")
	t.Setenv("GIT_REVISION", "main")

	_, errs := config.Load()

	found := false
	for _, e := range errs {
		if strings.Contains(e, "must be an HTTPS URL") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected HTTPS URL error, got %v", errs)
	}
}

func TestLoad_MissingAllGitFields(t *testing.T) {
	os.Unsetenv("GIT_REPO_URL")
	os.Unsetenv("GIT_ACCESS_TOKEN")
	os.Unsetenv("GIT_REVISION")

	_, errs := config.Load()

	if len(errs) != 3 {
		t.Errorf("expected 3 errors for all missing git fields, got %d: %v", len(errs), errs)
	}
}

func TestLoad_GitDeployDirDefault(t *testing.T) {
	t.Setenv("GIT_REPO_URL", "https://github.com/example/repo.git")
	t.Setenv("GIT_ACCESS_TOKEN", "tok")
	t.Setenv("GIT_REVISION", "main")
	os.Unsetenv("GIT_DEPLOY_DIR")

	cfg, errs := config.Load()

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if cfg.GitDeployDir != "" {
		t.Errorf("expected empty deploy dir by default, got %q", cfg.GitDeployDir)
	}
}

func TestLoad_GitDeployDirOverride(t *testing.T) {
	t.Setenv("GIT_REPO_URL", "https://github.com/example/repo.git")
	t.Setenv("GIT_ACCESS_TOKEN", "tok")
	t.Setenv("GIT_REVISION", "main")
	t.Setenv("GIT_DEPLOY_DIR", "deployments/host-a")

	cfg, errs := config.Load()

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if cfg.GitDeployDir != "deployments/host-a" {
		t.Errorf("expected deploy dir deployments/host-a, got %q", cfg.GitDeployDir)
	}
}

func TestLoad_WebhookSecretDefault(t *testing.T) {
	t.Setenv("GIT_REPO_URL", "https://github.com/example/repo.git")
	t.Setenv("GIT_ACCESS_TOKEN", "tok")
	t.Setenv("GIT_REVISION", "main")
	os.Unsetenv("WEBHOOK_SECRET")

	cfg, errs := config.Load()

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if cfg.WebhookSecret != "" {
		t.Errorf("expected empty webhook secret by default, got %q", cfg.WebhookSecret)
	}
}

func TestLoad_WebhookSecretOverride(t *testing.T) {
	t.Setenv("GIT_REPO_URL", "https://github.com/example/repo.git")
	t.Setenv("GIT_ACCESS_TOKEN", "tok")
	t.Setenv("GIT_REVISION", "main")
	t.Setenv("WEBHOOK_SECRET", "my-secret-value")

	cfg, errs := config.Load()

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if cfg.WebhookSecret != "my-secret-value" {
		t.Errorf("expected webhook secret my-secret-value, got %q", cfg.WebhookSecret)
	}
}

func TestLoad_RefreshPollIntervalDefault(t *testing.T) {
	t.Setenv("GIT_REPO_URL", "https://github.com/example/repo.git")
	t.Setenv("GIT_ACCESS_TOKEN", "tok")
	t.Setenv("GIT_REVISION", "main")
	os.Unsetenv("REFRESH_POLL_INTERVAL")

	cfg, errs := config.Load()

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if cfg.RefreshPollInterval != 0 {
		t.Errorf("expected zero poll interval by default, got %v", cfg.RefreshPollInterval)
	}
}

func TestLoad_RefreshPollIntervalValid(t *testing.T) {
	t.Setenv("GIT_REPO_URL", "https://github.com/example/repo.git")
	t.Setenv("GIT_ACCESS_TOKEN", "tok")
	t.Setenv("GIT_REVISION", "main")
	t.Setenv("REFRESH_POLL_INTERVAL", "60s")

	cfg, errs := config.Load()

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if cfg.RefreshPollInterval != 60*time.Second {
		t.Errorf("expected 60s poll interval, got %v", cfg.RefreshPollInterval)
	}
}

func TestLoad_RefreshPollIntervalInvalid(t *testing.T) {
	t.Setenv("GIT_REPO_URL", "https://github.com/example/repo.git")
	t.Setenv("GIT_ACCESS_TOKEN", "tok")
	t.Setenv("GIT_REVISION", "main")
	t.Setenv("REFRESH_POLL_INTERVAL", "notaduration")

	cfg, errs := config.Load()

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if cfg.RefreshPollInterval != 0 {
		t.Errorf("expected zero poll interval for invalid value, got %v", cfg.RefreshPollInterval)
	}
}
