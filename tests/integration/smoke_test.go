//go:build integration

package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/docker"
	handler "github.com/lucasreiners/docker-cd/internal/http"
)

func TestSmokeRootEndpoint(t *testing.T) {
	socketPath := "/var/run/docker.sock"
	if v := os.Getenv("DOCKER_SOCKET"); v != "" {
		socketPath = v
	}

	if _, err := os.Stat(socketPath); err != nil {
		t.Skipf("Docker socket not available at %s: %v", socketPath, err)
	}

	runner := &docker.ExecRunner{}
	out, err := runner.Run(context.Background(), "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil {
		t.Skipf("Docker CLI not available: %v (output: %s)", err, string(out))
	}

	cfg := config.Config{
		Port:         8080,
		ProjectName:  "Docker-CD",
		DockerSocket: socketPath,
	}

	router := handler.NewRouter(runner, cfg)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "Docker-CD") {
		t.Errorf("response should contain Docker-CD, got:\n%s", body)
	}
	if !strings.Contains(body, "Running containers:") {
		t.Errorf("response should contain Running containers:, got:\n%s", body)
	}

	t.Logf("Integration smoke test response:\n%s", body)
}
