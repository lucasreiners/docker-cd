package handler_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	handler "github.com/lucasreiners/docker-cd/internal/http"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/internal/refresh"
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
	return handler.NewRouter(runner, cfg, nil, nil, nil, nil)
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

// --- Webhook handler tests (T014) ---

func signPayload(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func setupRouterWithRefresh(runner handler.CommandRunner, cfg config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	store := desiredstate.NewStore()
	queue := refresh.NewQueue()
	svc := refresh.NewService(cfg, store, queue, nil)
	return handler.NewRouter(runner, cfg, svc, store, nil, nil)
}

func TestWebhookHandler_NoSecretConfigured(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	router := setupRouterWithRefresh(runner, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/webhook", strings.NewReader(`{"ref":"refs/heads/main"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status"`) {
		t.Errorf("response should contain status field, got: %s", body)
	}
}

func TestWebhookHandler_ValidSignature(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	secret := "test-secret-123"
	cfg := config.Config{
		Port:          8080,
		ProjectName:   "Docker-CD",
		DockerSocket:  "/var/run/docker.sock",
		WebhookSecret: secret,
	}

	router := setupRouterWithRefresh(runner, cfg)

	payload := `{"ref":"refs/heads/main"}`
	sig := signPayload(secret, payload)

	req := httptest.NewRequest(http.MethodPost, "/api/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status"`) {
		t.Errorf("response should contain status field, got: %s", body)
	}
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{
		Port:          8080,
		ProjectName:   "Docker-CD",
		DockerSocket:  "/var/run/docker.sock",
		WebhookSecret: "real-secret",
	}

	router := setupRouterWithRefresh(runner, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/webhook", strings.NewReader(`{"ref":"refs/heads/main"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalidsignature")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"error"`) {
		t.Errorf("response should contain error field, got: %s", body)
	}
}

func TestWebhookHandler_MissingSignatureWhenRequired(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{
		Port:          8080,
		ProjectName:   "Docker-CD",
		DockerSocket:  "/var/run/docker.sock",
		WebhookSecret: "needs-sig",
	}

	router := setupRouterWithRefresh(runner, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/webhook", strings.NewReader(`{"ref":"refs/heads/main"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

// --- Manual refresh handler tests (T017) ---

func TestManualRefreshHandler_ReturnsStatus(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	router := setupRouterWithRefresh(runner, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/refresh", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status"`) {
		t.Errorf("response should contain status field, got: %s", body)
	}
}

// --- Refresh status handler tests (T017) ---

func TestRefreshStatusHandler_ReturnsJSON(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	store := desiredstate.NewStore()
	queue := refresh.NewQueue()
	svc := refresh.NewService(cfg, store, queue, nil)

	gin.SetMode(gin.TestMode)
	router := handler.NewRouter(runner, cfg, svc, store, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/refresh-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"refreshStatus"`) {
		t.Errorf("response should contain refreshStatus field, got: %s", body)
	}
}

func TestRefreshStatusHandler_WithPopulatedStore(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "abc123",
		Ref:           "main",
		RefType:       "branch",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Status: desiredstate.StackSyncSynced},
		},
	})
	queue := refresh.NewQueue()
	svc := refresh.NewService(cfg, store, queue, nil)

	gin.SetMode(gin.TestMode)
	router := handler.NewRouter(runner, cfg, svc, store, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/refresh-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"abc123"`) {
		t.Errorf("response should contain revision, got: %s", body)
	}
	// refresh-status should NOT contain stacks
	if strings.Contains(body, `"app1"`) {
		t.Errorf("refresh-status response should NOT contain stacks, got: %s", body)
	}
}

// --- Stacks handler tests (T017) ---

func TestStacksHandler_EmptyStore(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	store := desiredstate.NewStore()
	queue := refresh.NewQueue()
	svc := refresh.NewService(cfg, store, queue, nil)

	gin.SetMode(gin.TestMode)
	router := handler.NewRouter(runner, cfg, svc, store, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if body != "[]" && !strings.Contains(body, "[]") {
		t.Errorf("expected empty array, got: %s", body)
	}
}

func TestStacksHandler_WithStacks(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "abc123",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Status: desiredstate.StackSyncSynced},
			{Path: "app2", ComposeFile: "docker-compose.yaml", ComposeHash: "hash2", Status: desiredstate.StackSyncMissing},
		},
	})
	queue := refresh.NewQueue()
	svc := refresh.NewService(cfg, store, queue, nil)

	gin.SetMode(gin.TestMode)
	router := handler.NewRouter(runner, cfg, svc, store, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"app1"`) {
		t.Errorf("response should contain app1, got: %s", body)
	}
	if !strings.Contains(body, `"app2"`) {
		t.Errorf("response should contain app2, got: %s", body)
	}
	if !strings.Contains(body, `"synced"`) {
		t.Errorf("response should contain synced status, got: %s", body)
	}
}

// --- T018/T019: Sync metadata in API tests ---

func TestStacksHandler_ExposeSyncMetadata(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	store := desiredstate.NewStore()
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
				SyncedCommitMessage: "deploy v1",
				SyncedComposeHash:   "hash1",
				SyncedAt:            "2024-01-01T00:00:00Z",
				LastSyncAt:          "2024-01-01T00:00:00Z",
				LastSyncStatus:      "synced",
			},
		},
	})
	queue := refresh.NewQueue()
	svc := refresh.NewService(cfg, store, queue, nil)

	gin.SetMode(gin.TestMode)
	router := handler.NewRouter(runner, cfg, svc, store, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()

	// Verify sync metadata fields are present in response
	mustContain := []string{
		`"syncedRevision":"abc123"`,
		`"syncedCommitMessage":"deploy v1"`,
		`"syncedComposeHash":"hash1"`,
		`"syncedAt":"2024-01-01T00:00:00Z"`,
		`"lastSyncAt":"2024-01-01T00:00:00Z"`,
		`"lastSyncStatus":"synced"`,
	}
	for _, expected := range mustContain {
		if !strings.Contains(body, expected) {
			t.Errorf("response should contain %s, got: %s", expected, body)
		}
	}

	// Content field should NOT appear in JSON
	if strings.Contains(body, `"content"`) {
		t.Errorf("response should NOT expose content field, got: %s", body)
	}
}

func TestStacksHandler_SyncErrorExposed(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "abc123",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{
				Path:           "app1",
				ComposeFile:    "docker-compose.yml",
				ComposeHash:    "hash1",
				Status:         desiredstate.StackSyncFailed,
				LastSyncAt:     "2024-01-01T00:00:00Z",
				LastSyncStatus: "failed",
				LastSyncError:  "compose up failed: image not found",
			},
		},
	})
	queue := refresh.NewQueue()
	svc := refresh.NewService(cfg, store, queue, nil)

	gin.SetMode(gin.TestMode)
	router := handler.NewRouter(runner, cfg, svc, store, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()

	if !strings.Contains(body, `"lastSyncError":"compose up failed: image not found"`) {
		t.Errorf("response should contain error, got: %s", body)
	}
	if !strings.Contains(body, `"failed"`) {
		t.Errorf("response should contain failed status, got: %s", body)
	}
}

// --- T036: Ack handler tests ---

type stubReconciler struct {
	runs []reconcile.ReconciliationRun
}

func (s *stubReconciler) Reconcile(_ context.Context) []reconcile.ReconciliationRun {
	return s.runs
}

func TestAckHandler_Success(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	store := desiredstate.NewStore()
	queue := refresh.NewQueue()
	svc := refresh.NewService(cfg, store, queue, nil)

	ackStore := reconcile.NewAckStore()
	rec := &stubReconciler{runs: []reconcile.ReconciliationRun{
		{StackPath: "app1", Result: "success"},
	}}

	gin.SetMode(gin.TestMode)
	router := handler.NewRouter(runner, cfg, svc, store, ackStore, rec)

	body := `{"stack_path": "app1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/reconcile/ack", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	resp := w.Body.String()
	if !strings.Contains(resp, `"success"`) {
		t.Errorf("response should contain success, got: %s", resp)
	}
	if !strings.Contains(resp, `"app1"`) {
		t.Errorf("response should contain stack path, got: %s", resp)
	}
}

func TestAckHandler_MissingStackPath(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	store := desiredstate.NewStore()
	queue := refresh.NewQueue()
	svc := refresh.NewService(cfg, store, queue, nil)

	ackStore := reconcile.NewAckStore()
	rec := &stubReconciler{}

	gin.SetMode(gin.TestMode)
	router := handler.NewRouter(runner, cfg, svc, store, ackStore, rec)

	req := httptest.NewRequest(http.MethodPost, "/api/reconcile/ack", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}
