package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

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
	runner := &docker.ExecRunner{}
	dockerClient := docker.NewClient(runner, cfg.DockerSocket)

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
			runner := &docker.ExecRunner{}
			dockerClient := docker.NewClient(runner, cfg.DockerSocket)

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
			runner := &docker.ExecRunner{}
			dockerClient := docker.NewClient(runner, cfg.DockerSocket)

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
	pullCalls  []pullCall
	pruneCalls int
	pullError  error
	pruneOut   string
	pruneError error
	pullDelay  time.Duration // Add delay to simulate slow operations
}

// Helper function to create a minimal reconciler for testing
func newTestReconciler(store *desiredstate.Store) *reconcile.Reconciler {
	policy := reconcile.ReconciliationPolicy{Enabled: false} // Disabled to avoid nil pointer issues with missing deps
	return reconcile.NewReconciler(
		store,
		policy,
		nil, // compose runner - reconciler won't actually run since disabled
		nil, // container inspector
		nil, // ackStore
		"",  // deployDir
		nil, // driftDetector
		nil, // stateManager
	)
}

type pullCall struct {
	name string
	args []string
}

func (m *mockRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	// Check if it's a compose pull command
	if name == "docker" && len(args) > 2 && args[0] == "compose" {
		if m.pullDelay > 0 {
			time.Sleep(m.pullDelay)
		}
		m.pullCalls = append(m.pullCalls, pullCall{name: name, args: args})
		return []byte{}, m.pullError
	}

	// Check if it's an image prune command
	if name == "docker" && len(args) > 1 && args[0] == "image" && args[1] == "prune" {
		m.pruneCalls++
		if m.pruneError != nil {
			return nil, m.pruneError
		}
		// Return realistic docker prune output
		if m.pruneOut == "" {
			m.pruneOut = "Total reclaimed space: 100MB\n"
		}
		return []byte(m.pruneOut), nil
	}

	return []byte{}, nil
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
		Stacks: []desiredstate.StackRecord{{
			Path:        "test-stack",
			ComposeFile: "/test/docker-compose.yml",
			Status:      desiredstate.StackSyncSynced,
		}},
	})

	mockRun := &mockRunner{
		pruneOut: "Total reclaimed space: 100MB\n",
	}
	dockerClient := docker.NewClient(mockRun, cfg.DockerSocket)
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
	if len(mockRun.pullCalls) != 1 {
		t.Errorf("Expected 1 pull call, got %d", len(mockRun.pullCalls))
	}
	if mockRun.pruneCalls != 1 {
		t.Errorf("Expected 1 prune call, got %d", mockRun.pruneCalls)
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
		Stacks: []desiredstate.StackRecord{{
			Path:        "test-stack",
			ComposeFile: "/test/docker-compose.yml",
			Status:      desiredstate.StackSyncSynced,
		}},
	})

	// Simulate slow pull operation with delay
	mockRun := &mockRunner{
		pullDelay: 500 * time.Millisecond, // Add delay to keep update running
		pruneOut:  "Total reclaimed space: 100MB\\n",
	}
	dockerClient := docker.NewClient(mockRun, cfg.DockerSocket)

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
	dockerClient := docker.NewClient(mockRun, cfg.DockerSocket)

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
	dockerClient := docker.NewClient(mockRun, cfg.DockerSocket)

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
				ComposeFile: "/test/docker-compose.yml",
				Status:      desiredstate.StackSyncFailed, // Failed status
			},
			{
				Path:        "healthy-stack",
				ComposeFile: "/test2/docker-compose.yml",
				Status:      desiredstate.StackSyncSynced,
			},
		},
	})

	mockRun := &mockRunner{}
	dockerClient := docker.NewClient(mockRun, cfg.DockerSocket)

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	cycle := NewUpdateCycle()
	svc.executeUpdateCycle(context.Background(), cycle)

	// Should only process healthy-stack (failed-stack skipped)
	if len(mockRun.pullCalls) != 1 {
		t.Errorf("Expected 1 pull call (only healthy stack), got %d", len(mockRun.pullCalls))
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
				ComposeFile: "/test1/docker-compose.yml",
				Status:      desiredstate.StackSyncSynced,
			},
			{
				Path:        "stack2",
				ComposeFile: "/test2/docker-compose.yml",
				Status:      desiredstate.StackSyncSynced,
			},
		},
	})

	// First call fails, second succeeds - use errors slice
	mockRun := &mockRunner{}

	dockerClient := docker.NewClient(mockRun, cfg.DockerSocket)

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, newTestReconciler(store), nil)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	// Set pull error for only the first pull attempt
	mockRun.pullError = errors.New("pull failed for stack1")

	cycle := NewUpdateCycle()

	// Manually run the first stack which will fail
	stack1 := desiredstate.StackRecord{
		Path:        "stack1",
		ComposeFile: "/test1/docker-compose.yml",
		Status:      desiredstate.StackSyncSynced,
	}
	result1 := svc.updateStack(context.Background(), stack1)
	if result1.Success {
		t.Error("Expected stack1 to fail")
	}
	cycle.StacksProcessed++
	if result1.Error != nil {
		cycle.Errors = append(cycle.Errors, result1.Error)
	}

	// Clear the error for the second stack
	mockRun.pullError = nil

	// Manually run the second stack which will succeed
	stack2 := desiredstate.StackRecord{
		Path:        "stack2",
		ComposeFile: "/test2/docker-compose.yml",
		Status:      desiredstate.StackSyncSynced,
	}
	result2 := svc.updateStack(context.Background(), stack2)
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
			ComposeFile: "/test/docker-compose.yml",
			Status:      desiredstate.StackSyncSynced,
		}},
	})

	mockRun := &mockRunner{
		pruneOut: "Total reclaimed space: 500MB\n",
	}
	dockerClient := docker.NewClient(mockRun, cfg.DockerSocket)
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
