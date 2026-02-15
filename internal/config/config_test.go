package config_test

import (
	"os"
	"testing"

	"github.com/lucasreiners/docker-cd/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	os.Unsetenv("PORT")
	os.Unsetenv("PROJECT_NAME")
	os.Unsetenv("DOCKER_SOCKET")

	cfg := config.Load()

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

	cfg := config.Load()

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

	cfg := config.Load()

	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080 for invalid PORT env, got %d", cfg.Port)
	}
}
