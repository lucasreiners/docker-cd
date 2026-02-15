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
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	handler "github.com/lucasreiners/docker-cd/internal/http"
	"github.com/lucasreiners/docker-cd/internal/refresh"
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

	router := handler.NewRouter(runner, cfg, nil, nil)

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

func TestSmokeAPIEndpoints(t *testing.T) {
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

	store := desiredstate.NewStore()
	queue := refresh.NewQueue()
	svc := refresh.NewService(cfg, store, queue, nil)

	router := handler.NewRouter(runner, cfg, svc, store)

	t.Run("POST /api/refresh returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/refresh", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"status"`) {
			t.Errorf("expected status field in response, got: %s", w.Body.String())
		}
	})

	t.Run("GET /api/refresh-status returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/refresh-status", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"refreshStatus"`) {
			t.Errorf("expected refreshStatus field, got: %s", w.Body.String())
		}
	})

	t.Run("GET /api/stacks returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if body != "[]" && !strings.Contains(body, "[") {
			t.Errorf("expected JSON array, got: %s", body)
		}
	})

	t.Run("POST /api/webhook returns 200 without secret", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/webhook", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}
