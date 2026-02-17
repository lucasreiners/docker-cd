package handler_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	handler "github.com/lucasreiners/docker-cd/internal/http"
	"github.com/lucasreiners/docker-cd/internal/refresh"
)

// setupRouterWithBroadcaster creates a router with SSE broadcaster wired up.
func setupRouterWithBroadcaster(runner handler.CommandRunner, cfg config.Config, store *desiredstate.Store, broadcaster *desiredstate.Broadcaster) *gin.Engine {
	gin.SetMode(gin.TestMode)
	queue := refresh.NewQueue()
	svc := refresh.NewService(cfg, store, queue, nil)
	return handler.NewRouter(runner, cfg, svc, store, nil, nil, broadcaster)
}

// sseEvent holds a parsed SSE event.
type sseEvent struct {
	id        string
	eventType string
	data      string
}

// readSSEEvent reads one complete SSE event from a scanner.
func readSSEEvent(t *testing.T, scanner *bufio.Scanner) sseEvent {
	t.Helper()
	var evt sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if evt.data != "" || evt.eventType != "" {
				return evt
			}
			continue
		}
		if strings.HasPrefix(line, "id: ") {
			evt.id = strings.TrimPrefix(line, "id: ")
		} else if strings.HasPrefix(line, "event: ") {
			evt.eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			evt.data = strings.TrimPrefix(line, "data: ")
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	t.Fatal("stream ended before a complete event was read")
	return evt
}

// TestEventsSSE_InitialSnapshot verifies the /api/events endpoint sends
// SSE headers and an initial stack.snapshot event containing all current stacks.
func TestEventsSSE_InitialSnapshot(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "abc123",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "apps/api", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Status: desiredstate.StackSyncSynced},
			{Path: "apps/web", ComposeFile: "docker-compose.yml", ComposeHash: "hash2", Status: desiredstate.StackSyncSyncing},
		},
	})

	broadcaster := desiredstate.NewBroadcaster()
	router := setupRouterWithBroadcaster(runner, cfg, store, broadcaster)

	ts := httptest.NewServer(router)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to SSE: %v", err)
	}
	defer resp.Body.Close()

	// Verify SSE headers
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}
	cc := resp.Header.Get("Cache-Control")
	if cc != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %s", cc)
	}
	xab := resp.Header.Get("X-Accel-Buffering")
	if xab != "no" {
		t.Errorf("expected X-Accel-Buffering no, got %s", xab)
	}

	// Read the initial event
	scanner := bufio.NewScanner(resp.Body)
	event := readSSEEvent(t, scanner)

	if event.eventType != "stack.snapshot" {
		t.Errorf("expected event type stack.snapshot, got %s", event.eventType)
	}
	if event.id == "" {
		t.Error("event should have an id")
	}

	// Parse the data payload
	var payload struct {
		Records []desiredstate.StackRecord `json:"records"`
	}
	if err := json.Unmarshal([]byte(event.data), &payload); err != nil {
		t.Fatalf("failed to parse event data: %v", err)
	}
	if len(payload.Records) != 2 {
		t.Fatalf("expected 2 records in snapshot, got %d", len(payload.Records))
	}
	if payload.Records[0].Path != "apps/api" {
		t.Errorf("expected first record path apps/api, got %s", payload.Records[0].Path)
	}
	if payload.Records[1].Path != "apps/web" {
		t.Errorf("expected second record path apps/web, got %s", payload.Records[1].Path)
	}
}

// TestEventsSSE_UpsertEvent verifies that publishing a stack.upsert event
// reaches connected SSE clients.
func TestEventsSSE_UpsertEvent(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks:        []desiredstate.StackRecord{},
	})

	broadcaster := desiredstate.NewBroadcaster()
	router := setupRouterWithBroadcaster(runner, cfg, store, broadcaster)

	ts := httptest.NewServer(router)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to SSE: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// Read and discard initial snapshot
	_ = readSSEEvent(t, scanner)

	// Publish an upsert
	broadcaster.PublishStackUpsert(desiredstate.StackRecord{
		Path:        "apps/new",
		ComposeFile: "docker-compose.yml",
		ComposeHash: "newhash",
		Status:      desiredstate.StackSyncSynced,
	})

	// Read the upsert event
	event := readSSEEvent(t, scanner)
	if event.eventType != "stack.upsert" {
		t.Errorf("expected event type stack.upsert, got %s", event.eventType)
	}

	var payload struct {
		Record desiredstate.StackRecord `json:"record"`
	}
	if err := json.Unmarshal([]byte(event.data), &payload); err != nil {
		t.Fatalf("failed to parse event data: %v", err)
	}
	if payload.Record.Path != "apps/new" {
		t.Errorf("expected upsert path apps/new, got %s", payload.Record.Path)
	}
	if payload.Record.Status != desiredstate.StackSyncSynced {
		t.Errorf("expected status synced, got %s", payload.Record.Status)
	}
}

// TestEventsSSE_EmptyStoreSnapshot verifies empty store sends empty records array.
func TestEventsSSE_EmptyStoreSnapshot(t *testing.T) {
	runner := &stubRunner{output: []byte("a\n")}
	cfg := config.Config{Port: 8080, ProjectName: "Docker-CD", DockerSocket: "/var/run/docker.sock"}

	store := desiredstate.NewStore()
	broadcaster := desiredstate.NewBroadcaster()
	router := setupRouterWithBroadcaster(runner, cfg, store, broadcaster)

	ts := httptest.NewServer(router)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to SSE: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	event := readSSEEvent(t, scanner)

	if event.eventType != "stack.snapshot" {
		t.Errorf("expected event type stack.snapshot, got %s", event.eventType)
	}

	var payload struct {
		Records []desiredstate.StackRecord `json:"records"`
	}
	if err := json.Unmarshal([]byte(event.data), &payload); err != nil {
		t.Fatalf("failed to parse event data: %v", err)
	}
	if len(payload.Records) != 0 {
		t.Errorf("expected 0 records, got %d", len(payload.Records))
	}
}
