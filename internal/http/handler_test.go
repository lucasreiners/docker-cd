package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lucasreiners/docker-cd/internal/config"
	handler "github.com/lucasreiners/docker-cd/internal/http"
)

type stubRunner struct {
	output []byte
	err    error
}

func (s *stubRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return s.output, s.err
}

func setupRouter(runner handler.CommandRunner, cfg config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	return handler.NewRouter(runner, cfg)
}

func TestRootHandler_Success(t *testing.T) {
	runner := &stubRunner{output: []byte("a\nb\nc\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	router := setupRouter(runner, cfg)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Docker-CD") {
		t.Errorf("response should contain Docker-CD, got:\n%s", body)
	}
	if !strings.Contains(body, "Running containers: 3") {
		t.Errorf("response should contain 'Running containers: 3', got:\n%s", body)
	}
}

func TestRootHandler_DockerError(t *testing.T) {
	runner := &stubRunner{output: []byte("permission denied"), err: fmt.Errorf("exit status 1")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	router := setupRouter(runner, cfg)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "docker CLI error") {
		t.Errorf("response should contain error message, got:\n%s", body)
	}
}

func TestRootHandler_ZeroContainers(t *testing.T) {
	runner := &stubRunner{output: []byte("")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	router := setupRouter(runner, cfg)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Running containers: 0") {
		t.Errorf("response should contain 'Running containers: 0', got:\n%s", body)
	}
}

func TestRootHandler_ShowsRepoInfo(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{
		Port:           8080,
		ProjectName:    "Docker-CD",
		DockerSocket:   "/var/run/docker.sock",
		GitRepoURL:     "https://github.com/org/repo.git",
		GitAccessToken: "secret-token-value",
		GitRevision:    "main",
		GitDeployDir:   "deployments/host-a",
	}

	router := setupRouter(runner, cfg)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Repository: https://github.com/org/repo.git") {
		t.Errorf("response should show repo URL, got:\n%s", body)
	}
	if !strings.Contains(body, "Revision: main") {
		t.Errorf("response should show revision, got:\n%s", body)
	}
	if !strings.Contains(body, "Deploy dir: deployments/host-a") {
		t.Errorf("response should show deploy dir, got:\n%s", body)
	}
	if strings.Contains(body, "secret-token-value") {
		t.Errorf("response must NOT contain the access token, got:\n%s", body)
	}
}
