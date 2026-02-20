//go:build integration

package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"encoding/json"

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

	router := handler.NewRouter(runner, cfg, nil, nil, nil, nil)

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

	router := handler.NewRouter(runner, cfg, svc, store, nil, nil)

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

	t.Run("GET /api/stacks includes sync metadata fields", func(t *testing.T) {
		// Seed a stack with sync metadata
		store.Set(&desiredstate.Snapshot{
			Revision:      "abc123",
			RefreshStatus: desiredstate.RefreshStatusCompleted,
			Stacks: []desiredstate.StackRecord{
				{
					Path:                "app1",
					ComposeFile:         "docker-compose.yml",
					ComposeHash:         "hash1",
					Status:              desiredstate.StackSyncSynced,
					SyncedRevision:      "abc123",
					SyncedComposeHash:   "hash1",
					SyncedCommitMessage: "deploy v1",
					LastSyncAt:          "2024-01-01T00:00:00Z",
				},
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var stacks []map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &stacks); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		if len(stacks) == 0 {
			t.Fatal("expected at least 1 stack")
		}

		stack := stacks[0]
		requiredFields := []string{"syncedRevision", "syncedComposeHash", "syncedCommitMessage", "lastSyncAt", "status"}
		for _, field := range requiredFields {
			if _, ok := stack[field]; !ok {
				t.Errorf("expected field %q in stacks response, got keys: %v", field, keysOf(stack))
			}
		}

		// Content should NOT be exposed
		if _, ok := stack["content"]; ok {
			t.Error("content field should not be in API response")
		}
	})
}

func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
