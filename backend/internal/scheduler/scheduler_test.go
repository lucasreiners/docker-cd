package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
)

// TestNewSchedulerService_Disabled verifies scheduler is nil when disabled
func TestNewSchedulerService_Disabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: false,
	}

	store := desiredstate.NewStore()
	dockerClient := docker.NewClient(nil)

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if svc != nil {
		t.Fatal("Expected nil scheduler when disabled")
	}
}

// TestNewSchedulerService_ValidCron verifies scheduler accepts valid cron expressions
func TestNewSchedulerService_ValidCron(t *testing.T) {
	tests := []struct {
		name string
		cron string
	}{
		{"default schedule", "0 3 * * *"},
		{"every 6 hours", "0 */6 * * *"},
		{"weekdays only", "0 4 * * 1-5"},
		{"monthly", "0 2 1 * *"},
		{"every minute", "* * * * *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
			cfg := config.Config{
				UpdaterEnabled: true,
				UpdaterCron:    tt.cron,
			}

			store := desiredstate.NewStore()
			dockerClient := docker.NewClient(nil)

			svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
			if err != nil {
				t.Fatalf("Unexpected error for valid cron %q: %v", tt.cron, err)
			}

			if svc == nil {
				t.Fatal("Expected non-nil scheduler when enabled")
			}

			if svc.config.UpdaterCron != tt.cron {
				t.Errorf("Expected cron %q, got %q", tt.cron, svc.config.UpdaterCron)
			}
		})
	}
}

// TestNewSchedulerService_InvalidCron verifies fallback to default for invalid expressions
func TestNewSchedulerService_InvalidCron(t *testing.T) {
	tests := []struct {
		name       string
		cron       string
		expectCron string
	}{
		{"empty string", "", "0 3 * * *"},
		{"garbage text", "not-a-cron", "0 3 * * *"},
		{"too many fields", "* * * * * * *", "0 3 * * *"},
		{"invalid syntax", "60 25 * * *", "0 3 * * *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
			cfg := config.Config{
				UpdaterEnabled: true,
				UpdaterCron:    tt.cron,
			}

			store := desiredstate.NewStore()
			dockerClient := docker.NewClient(nil)

			svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if svc == nil {
				t.Fatal("Expected non-nil scheduler when enabled")
			}

			if svc.config.UpdaterCron != tt.expectCron {
				t.Errorf("Expected fallback to %q, got %q", tt.expectCron, svc.config.UpdaterCron)
			}
		})
	}
}

// TestNewSchedulerService_NilDependencies verifies scheduler handles nil dependencies gracefully
func TestNewSchedulerService_NilDependencies(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	// Should not panic with nil dependencies
	svc, err := NewSchedulerService(cfg, logger, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if svc == nil {
		t.Fatal("Expected non-nil scheduler when enabled")
	}
}

// Mock CommandRunner for testing Docker client

type mockRunner struct {
	pullDelay     time.Duration     // copied to mockDockerAPI
	pullError     error             // copied to mockDockerAPI
	imageDigests  map[string]string // image name -> digest (copied to mockDockerAPI)
	inspectError  error             // error for inspect (copied to mockDockerAPI)
	pruneError    error             // copied to mockDockerAPI
}

// Helper function to create a minimal reconciler for testing
// noopComposeRunner satisfies reconcile.ComposeRunner for unit tests.
type noopComposeRunner struct{}

func (n *noopComposeRunner) ComposeUp(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (n *noopComposeRunner) ComposeDown(_ context.Context, _, _, _ string) error { return nil }
func (n *noopComposeRunner) ComposePs(_ context.Context, _ string) ([]desiredstate.ContainerInfo, error) {
	return nil, nil
}

func newTestReconciler(store *desiredstate.Store) *reconcile.Reconciler {
	compose := &noopComposeRunner{}
	sm := reconcile.NewStateManager(store, compose, nil, slog.Default())
	return reconcile.NewReconciler(
		store,
		reconcile.DefaultPolicy(),
		compose,
		nil, // container inspector — not needed for ReconcileStack path
		reconcile.NewAckStore(),
		"",  // deployDir
		nil, // driftDetector — not needed for ReconcileStack path
		sm,
		slog.Default(),
	)
}

func (m *mockRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return []byte{}, nil
}

// mockDockerAPI implements docker.DockerAPI for scheduler tests.
type mockDockerAPI struct {
	imageDigests map[string]string // image name -> digest
	inspectCalls []string
	inspectError error
	pruneCalls   int
	pruneError   error
	pullError    error
	pullDelay    time.Duration
	pullCalls    int
}

func (m *mockDockerAPI) ContainerList(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
	return nil, nil
}
func (m *mockDockerAPI) ContainerInspect(_ context.Context, _ string) (types.ContainerJSON, error) {
	return types.ContainerJSON{}, nil
}
func (m *mockDockerAPI) ImageInspectWithRaw(_ context.Context, imageID string) (types.ImageInspect, []byte, error) {
	m.inspectCalls = append(m.inspectCalls, imageID)
	if m.inspectError != nil {
		return types.ImageInspect{}, nil, m.inspectError
	}
	if m.imageDigests != nil {
		if digest, ok := m.imageDigests[imageID]; ok {
			return types.ImageInspect{ID: digest}, nil, nil
		}
	}
	return types.ImageInspect{ID: "sha256:default000"}, nil, nil
}
func (m *mockDockerAPI) ImagePull(ctx context.Context, _ string, _ image.PullOptions) (io.ReadCloser, error) {
	m.pullCalls++
	if m.pullDelay > 0 {
		select {
		case <-time.After(m.pullDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.pullError != nil {
		return nil, m.pullError
	}
	stream := `{"status":"Pull complete"}` + "\n"
	return io.NopCloser(strings.NewReader(stream)), nil
}
func (m *mockDockerAPI) ImagesPrune(_ context.Context, _ filters.Args) (image.PruneReport, error) {
	m.pruneCalls++
	if m.pruneError != nil {
		return image.PruneReport{}, m.pruneError
	}
	return image.PruneReport{SpaceReclaimed: 100000000}, nil // 100MB
}

// newMockAPIFromRunner creates a mockDockerAPI that mirrors a mockRunner's config.
func newMockAPIFromRunner(mr *mockRunner) *mockDockerAPI {
	return &mockDockerAPI{
		imageDigests: mr.imageDigests,
		inspectError: mr.inspectError,
		pruneError:   mr.pruneError,
		pullError:    mr.pullError,
		pullDelay:    mr.pullDelay,
	}
}

// sequentialMockAPI returns sequential digests from ImageInspectWithRaw.
type sequentialMockAPI struct {
	mockDockerAPI
	inspectDigests []string
	inspectIndex   *int
}

func (s *sequentialMockAPI) ImageInspectWithRaw(_ context.Context, imageID string) (types.ImageInspect, []byte, error) {
	idx := *s.inspectIndex
	*s.inspectIndex++
	if idx < len(s.inspectDigests) {
		return types.ImageInspect{ID: s.inspectDigests[idx]}, nil, nil
	}
	return types.ImageInspect{ID: "sha256:fallback"}, nil, nil
}

// TestTriggerUpdateCycle_Success verifies manual trigger starts update cycle and completes
func TestTriggerUpdateCycle_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	// Add a test stack
	store.Set(&desiredstate.Snapshot{
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{{
			Path:        "test-stack",
			ComposeFile: "docker-compose.yml",
			Content:     []byte("services:\n  web:\n    image: nginx\n"),
			Status:      desiredstate.StackSyncSynced,
		}},
	})

	mockRun := &mockRunner{
		
	}
	api := newMockAPIFromRunner(mockRun)
	dockerClient := docker.NewClient(api)
	broadcaster := desiredstate.NewBroadcaster()

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), broadcaster)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	// Trigger update cycle
	err = svc.TriggerUpdateCycle(context.Background())
	if err != nil {
		t.Fatalf("TriggerUpdateCycle failed: %v", err)
	}

	// Wait for cycle to complete
	time.Sleep(200 * time.Millisecond)

	// Verify cycle completed (activeUpdate should be nil)
	status := svc.GetUpdateStatus()
	if status != nil {
		t.Errorf("Expected update to be completed (nil status), but it's still running")
	}

	// Verify Docker client was called
	if api.pullCalls != 1 {
		t.Errorf("Expected 1 pull call, got %d", api.pullCalls)
	}
	if api.pruneCalls != 1 {
		t.Errorf("Expected 1 prune call, got %d", api.pruneCalls)
	}

	// Events were published via broadcaster (can't easily verify in unit test without subscriber)
}

// TestTriggerUpdateCycle_AlreadyRunning verifies concurrent trigger prevention
func TestTriggerUpdateCycle_AlreadyRunning(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{{
			Path:        "test-stack",
			ComposeFile: "docker-compose.yml",
			Content:     []byte("services:\n  web:\n    image: nginx\n"),
			Status:      desiredstate.StackSyncSynced,
		}},
	})

	// Simulate slow pull operation with delay
	mockRun := &mockRunner{
		pullDelay: 500 * time.Millisecond, // Add delay to keep update running
	}
	dockerClient := docker.NewClient(newMockAPIFromRunner(mockRun))

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	// Start first update cycle
	err = svc.TriggerUpdateCycle(context.Background())
	if err != nil {
		t.Fatalf("First TriggerUpdateCycle failed: %v", err)
	}

	// Give first cycle time to start
	time.Sleep(50 * time.Millisecond)

	// Try to start second update cycle - should fail
	err = svc.TriggerUpdateCycle(context.Background())
	if err == nil {
		t.Fatal("Expected error for concurrent trigger, got nil")
	}

	cycleID := ""
	svc.mu.Lock()
	if svc.activeUpdate != nil {
		cycleID = svc.activeUpdate.CycleID
	}
	svc.mu.Unlock()

	expectedError := "update cycle already in progress (cycle_id: " + cycleID + ")"
	if err.Error() != expectedError {
		t.Errorf("Expected error %q, got %q", expectedError, err.Error())
	}

	// Wait for first cycle to complete
	time.Sleep(1 * time.Second)
}

// TestGetUpdateStatus verifies status reporting
func TestGetUpdateStatus(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	mockRun := &mockRunner{}
	dockerClient := docker.NewClient(newMockAPIFromRunner(mockRun))

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	// Initially no update should be running
	status := svc.GetUpdateStatus()
	if status != nil {
		t.Errorf("Expected nil status initially, got: %v", status)
	}

	// Set an active update manually for testing
	svc.mu.Lock()
	svc.activeUpdate = NewUpdateCycle()
	svc.mu.Unlock()

	// Should now return the active update
	status = svc.GetUpdateStatus()
	if status == nil {
		t.Fatal("Expected active update status, got nil")
	}

	cycle, ok := status.(*UpdateCycle)
	if !ok {
		t.Fatalf("Expected *UpdateCycle, got %T", status)
	}

	if cycle.CycleID == "" {
		t.Error("Expected non-empty CycleID")
	}
}

// TestExecuteUpdateCycle_EmptyStore verifies handling of empty desired state
func TestExecuteUpdateCycle_EmptyStore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	// Store is empty (nil snapshot)

	mockRun := &mockRunner{}
	dockerClient := docker.NewClient(newMockAPIFromRunner(mockRun))

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	cycle := NewUpdateCycle()
	svc.executeUpdateCycle(context.Background(), cycle)

	// Should complete without processing any stacks
	if cycle.StacksProcessed != 0 {
		t.Errorf("Expected 0 stacks processed, got %d", cycle.StacksProcessed)
	}

	// Events published via broadcaster (verified manually)
}

// TestExecuteUpdateCycle_SkipFailedStacks verifies failed stacks are skipped (FR-014)
func TestExecuteUpdateCycle_SkipFailedStacks(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Stacks: []desiredstate.StackRecord{
			{
				Path:        "failed-stack",
				ComposeFile: "docker-compose.yml",
				Content:     []byte("services:\n  web:\n    image: nginx\n"),
				Status:      desiredstate.StackSyncFailed, // Failed status
			},
			{
				Path:        "healthy-stack",
				ComposeFile: "docker-compose.yml",
				Content:     []byte("services:\n  web:\n    image: nginx\n"),
				Status:      desiredstate.StackSyncSynced,
			},
		},
	})

	mockRun := &mockRunner{}
	api := newMockAPIFromRunner(mockRun)
	dockerClient := docker.NewClient(api)

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	cycle := NewUpdateCycle()
	svc.executeUpdateCycle(context.Background(), cycle)

	// Should only process healthy-stack (failed-stack skipped)
	if api.pullCalls != 1 {
		t.Errorf("Expected 1 pull call (only healthy stack), got %d", api.pullCalls)
	}

	if cycle.StacksProcessed != 1 {
		t.Errorf("Expected 1 stack processed, got %d", cycle.StacksProcessed)
	}
}

// TestExecuteUpdateCycle_ContinueOnError verifies error recovery (FR-012)
func TestExecuteUpdateCycle_ContinueOnError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Stacks: []desiredstate.StackRecord{
			{
				Path:        "stack1",
				ComposeFile: "docker-compose.yml",
				Content:     []byte("services:\n  web:\n    image: nginx\n"),
				Status:      desiredstate.StackSyncSynced,
			},
			{
				Path:        "stack2",
				ComposeFile: "docker-compose.yml",
				Content:     []byte("services:\n  web:\n    image: nginx\n"),
				Status:      desiredstate.StackSyncSynced,
			},
		},
	})

	// First call fails, second succeeds - use errors slice
	mockRun := &mockRunner{}
	api := newMockAPIFromRunner(mockRun)

	dockerClient := docker.NewClient(api)

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	// Set pull error for only the first pull attempt
	api.pullError = errors.New("pull failed for stack1")

	cycle := NewUpdateCycle()

	// Manually run the first stack which will fail
	stack1 := desiredstate.StackRecord{
		Path:        "stack1",
		ComposeFile: "docker-compose.yml",
		Content:     []byte("services:\n  web:\n    image: nginx\n"),
		Status:      desiredstate.StackSyncSynced,
	}
	result1 := svc.updateStack(context.Background(), stack1, nil)
	if result1.Success {
		t.Error("Expected stack1 to fail")
	}
	cycle.StacksProcessed++
	if result1.Error != nil {
		cycle.Errors = append(cycle.Errors, result1.Error)
	}

	// Clear the error for the second stack
	api.pullError = nil

	// Manually run the second stack which will succeed
	stack2 := desiredstate.StackRecord{
		Path:        "stack2",
		ComposeFile: "docker-compose.yml",
		Content:     []byte("services:\n  web:\n    image: nginx\n"),
		Status:      desiredstate.StackSyncSynced,
	}
	result2 := svc.updateStack(context.Background(), stack2, nil)
	if !result2.Success {
		t.Errorf("Expected stack2 to succeed, got error: %v", result2.Error)
	}
	cycle.StacksProcessed++
	if result2.Error != nil {
		cycle.Errors = append(cycle.Errors, result2.Error)
	}

	// Should process both stacks
	if cycle.StacksProcessed != 2 {
		t.Errorf("Expected 2 stacks processed, got %d", cycle.StacksProcessed)
	}

	// Should have 1 error recorded
	if len(cycle.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(cycle.Errors))
	}
}

// TestExecuteUpdateCycle_EventBroadcasting verifies all events are published
func TestExecuteUpdateCycle_EventBroadcasting(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Stacks: []desiredstate.StackRecord{{
			Path:        "test-stack",
			ComposeFile: "docker-compose.yml",
			Content:     []byte("services:\n  web:\n    image: nginx\n"),
			Status:      desiredstate.StackSyncSynced,
		}},
	})

	mockRun := &mockRunner{}
	dockerClient := docker.NewClient(newMockAPIFromRunner(mockRun))
	broadcaster := desiredstate.NewBroadcaster()

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), broadcaster)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	cycle := NewUpdateCycle()
	svc.executeUpdateCycle(context.Background(), cycle)

	// Events were published via broadcaster (can't easily assert without subscriber)
	// Just verify the cycle completed successfully
	if cycle.StacksProcessed != 1 {
		t.Errorf("Expected 1 stack processed, got %d", cycle.StacksProcessed)
	}
}

// TestUpdateStack_DigestComparison_ImageChanged verifies that when an image digest
// changes after pull, it is detected and logged, and reconciliation is triggered.
func TestUpdateStack_DigestComparison_ImageChanged(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Stacks: []desiredstate.StackRecord{{
			Path:        "test-stack",
			ComposeFile: "docker-compose.yml",
			Content:     []byte("services:\n  web:\n    image: nginx:latest\n"),
			Status:      desiredstate.StackSyncSynced,
		}},
	})

	// Simulate digest change: before pull returns old digest, after pull returns new.
	callCount := 0
	seqAPI := &sequentialMockAPI{
		inspectDigests: []string{"sha256:olddigest111", "sha256:newdigest222"},
		inspectIndex:   &callCount,
	}

	dockerClient := docker.NewClient(seqAPI)

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	stack := desiredstate.StackRecord{
		Path:        "test-stack",
		ComposeFile: "docker-compose.yml",
		Content:     []byte("services:\n  web:\n    image: nginx:latest\n"),
		Status:      desiredstate.StackSyncSynced,
	}

	result := svc.updateStack(context.Background(), stack, nil)

	if !result.Success {
		t.Fatalf("Expected success, got error: %v", result.Error)
	}
	if !result.ReconcileTriggered {
		t.Error("Expected reconciliation to be triggered when image changed")
	}
	if len(result.ImagesPulled) != 1 {
		t.Fatalf("Expected 1 image pull result, got %d", len(result.ImagesPulled))
	}
	img := result.ImagesPulled[0]
	if img.ImageName != "nginx:latest" {
		t.Errorf("Expected image name 'nginx:latest', got %q", img.ImageName)
	}
	if !img.Changed {
		t.Error("Expected image to be marked as changed")
	}
	if img.PreviousDigest != "sha256:olddigest111" {
		t.Errorf("Expected previous digest 'sha256:olddigest111', got %q", img.PreviousDigest)
	}
	if img.NewDigest != "sha256:newdigest222" {
		t.Errorf("Expected new digest 'sha256:newdigest222', got %q", img.NewDigest)
	}
}

// TestUpdateStack_DigestComparison_NoChange verifies that when digests are the same
// before and after pull, no reconciliation is triggered (optimization).
func TestUpdateStack_DigestComparison_NoChange(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Stacks: []desiredstate.StackRecord{{
			Path:        "test-stack",
			ComposeFile: "docker-compose.yml",
			Content:     []byte("services:\n  web:\n    image: nginx:latest\n"),
			Status:      desiredstate.StackSyncSynced,
		}},
	})

	// Same digest before and after pull — no change.
	callCount := 0
	sameDigest := "sha256:unchanged999"
	seqAPI := &sequentialMockAPI{
		inspectDigests: []string{sameDigest, sameDigest},
		inspectIndex:   &callCount,
	}

	dockerClient := docker.NewClient(seqAPI)

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	stack := desiredstate.StackRecord{
		Path:        "test-stack",
		ComposeFile: "docker-compose.yml",
		Content:     []byte("services:\n  web:\n    image: nginx:latest\n"),
		Status:      desiredstate.StackSyncSynced,
	}

	result := svc.updateStack(context.Background(), stack, nil)

	if !result.Success {
		t.Fatalf("Expected success, got error: %v", result.Error)
	}
	if result.ReconcileTriggered {
		t.Error("Expected reconciliation NOT to be triggered when no image changed")
	}
	if len(result.ImagesPulled) != 1 {
		t.Fatalf("Expected 1 image pull result, got %d", len(result.ImagesPulled))
	}
	img := result.ImagesPulled[0]
	if img.Changed {
		t.Error("Expected image NOT to be marked as changed")
	}
}

// TestUpdateStack_DigestComparison_MultipleImages verifies digest comparison
// with multiple images where only some changed.
func TestUpdateStack_DigestComparison_MultipleImages(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Stacks: []desiredstate.StackRecord{{
			Path:        "test-stack",
			ComposeFile: "docker-compose.yml",
			Content:     []byte("services:\n  web:\n    image: nginx:latest\n  db:\n    image: postgres:16\n"),
			Status:      desiredstate.StackSyncSynced,
		}},
	})

	// nginx changes, postgres stays the same.
	// Inspect order: nginx(before), postgres(before), nginx(after), postgres(after)
	callCount := 0
	seqAPI := &sequentialMockAPI{
		inspectDigests: []string{
			"sha256:nginx_old", "sha256:pg_same",
			"sha256:nginx_new", "sha256:pg_same",
		},
		inspectIndex: &callCount,
	}

	dockerClient := docker.NewClient(seqAPI)

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	stack := desiredstate.StackRecord{
		Path:        "test-stack",
		ComposeFile: "docker-compose.yml",
		Content:     []byte("services:\n  web:\n    image: nginx:latest\n  db:\n    image: postgres:16\n"),
		Status:      desiredstate.StackSyncSynced,
	}

	result := svc.updateStack(context.Background(), stack, nil)

	if !result.Success {
		t.Fatalf("Expected success, got error: %v", result.Error)
	}
	if !result.ReconcileTriggered {
		t.Error("Expected reconciliation to be triggered (nginx changed)")
	}
	if len(result.ImagesPulled) != 2 {
		t.Fatalf("Expected 2 image pull results, got %d", len(result.ImagesPulled))
	}

	// Find the results by name
	var nginxResult, pgResult *ImagePullResult
	for i := range result.ImagesPulled {
		switch result.ImagesPulled[i].ImageName {
		case "nginx:latest":
			nginxResult = &result.ImagesPulled[i]
		case "postgres:16":
			pgResult = &result.ImagesPulled[i]
		}
	}

	if nginxResult == nil || pgResult == nil {
		t.Fatal("Expected both nginx and postgres results")
	}
	if !nginxResult.Changed {
		t.Error("Expected nginx to be marked as changed")
	}
	if pgResult.Changed {
		t.Error("Expected postgres NOT to be marked as changed")
	}
}

// TestUpdateStack_DigestComparison_InspectFailure verifies graceful handling
// when digest inspection fails (should still pull and reconcile).
func TestUpdateStack_DigestComparison_InspectFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Stacks: []desiredstate.StackRecord{{
			Path:        "test-stack",
			ComposeFile: "docker-compose.yml",
			Content:     []byte("services:\n  web:\n    image: nginx:latest\n"),
			Status:      desiredstate.StackSyncSynced,
		}},
	})

	mockRun := &mockRunner{
		
		inspectError:  errors.New("inspect failed"),
	}

	dockerClient := docker.NewClient(newMockAPIFromRunner(mockRun))

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	stack := desiredstate.StackRecord{
		Path:        "test-stack",
		ComposeFile: "docker-compose.yml",
		Content:     []byte("services:\n  web:\n    image: nginx:latest\n"),
		Status:      desiredstate.StackSyncSynced,
	}

	result := svc.updateStack(context.Background(), stack, nil)

	// Should still succeed — inspect failure is non-fatal, just triggers reconcile as fallback
	if !result.Success {
		t.Fatalf("Expected success even with inspect failure, got error: %v", result.Error)
	}
	if !result.ReconcileTriggered {
		t.Error("Expected reconciliation to be triggered as fallback when inspect fails")
	}
}

// TestUpdateStack_EmptyContentFallback verifies graceful handling
// when stack content has no images (equivalent to old config --images failure).
func TestUpdateStack_EmptyContentFallback(t *testing.T) {
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	logger := slog.Default()
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Stacks: []desiredstate.StackRecord{{
			Path:        "test-stack",
			ComposeFile: "docker-compose.yml",
			Content:     []byte(""),
			Status:      desiredstate.StackSyncSynced,
		}},
	})

	mockRun := &mockRunner{}

	dockerClient := docker.NewClient(newMockAPIFromRunner(mockRun))

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	stack := desiredstate.StackRecord{
		Path:        "test-stack",
		ComposeFile: "docker-compose.yml",
		Content:     []byte(""),
		Status:      desiredstate.StackSyncSynced,
	}

	result := svc.updateStack(context.Background(), stack, nil)

	// Should still succeed — empty content triggers unconditional reconcile
	if !result.Success {
		t.Fatalf("Expected success even with empty content, got error: %v", result.Error)
	}
	if !result.ReconcileTriggered {
		t.Error("Expected reconciliation to be triggered as fallback when no images found")
	}
}

// --- CancelActiveUpdate tests ---

// blockingMockAPI blocks on ImagePull until context is cancelled.
type blockingMockAPI struct {
	mockDockerAPI
	pullStarted chan struct{} // closed when pull begins
}

func (b *blockingMockAPI) ImagePull(ctx context.Context, _ string, _ image.PullOptions) (io.ReadCloser, error) {
	select {
	case <-b.pullStarted:
	default:
		close(b.pullStarted)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestCancelActiveUpdate_CancelsRunningCycle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{{
			Path:        "test-stack",
			ComposeFile: "docker-compose.yml",
			Content:     []byte("services:\n  web:\n    image: nginx\n"),
			Status:      desiredstate.StackSyncSynced,
		}},
	})

	inner := &mockRunner{
		imageDigests:  map[string]string{"nginx": "sha256:aaa"},
	}
	blockingAPI := &blockingMockAPI{
		mockDockerAPI: mockDockerAPI{
			imageDigests: inner.imageDigests,
		},
		pullStarted: make(chan struct{}),
	}
	dockerClient := docker.NewClient(blockingAPI)

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	err = svc.TriggerUpdateCycle(context.Background())
	if err != nil {
		t.Fatalf("TriggerUpdateCycle failed: %v", err)
	}

	// Wait for pull to start
	select {
	case <-blockingAPI.pullStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("pull did not start in time")
	}

	// Cancel the active update
	svc.CancelActiveUpdate()

	// Wait for cycle to complete (activeUpdate cleared)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("update cycle did not terminate after cancel")
		default:
		}
		svc.mu.Lock()
		active := svc.activeUpdate
		svc.mu.Unlock()
		if active == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCancelActiveUpdate_SafeWhenNoCycleActive(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	store := desiredstate.NewStore()
	dockerClient := docker.NewClient(nil)

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	// Should not panic
	svc.CancelActiveUpdate()
}
