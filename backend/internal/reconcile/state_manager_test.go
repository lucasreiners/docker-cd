package reconcile_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
)

// psStubComposeRunner returns canned ComposePs results.
type psStubComposeRunner struct {
	containers []desiredstate.ContainerInfo
	psErr      error
}

func (s *psStubComposeRunner) ComposeUp(_ context.Context, _, _, _, _ string) error { return nil }
func (s *psStubComposeRunner) ComposeDown(_ context.Context, _, _, _ string) error  { return nil }
func (s *psStubComposeRunner) ComposePs(_ context.Context, _ string) ([]desiredstate.ContainerInfo, error) {
	return s.containers, s.psErr
}

func TestUpdateContainerCounts_HappyPath(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "stacks/app", Status: desiredstate.StackSyncSynced},
		},
	})

	compose := &psStubComposeRunner{
		containers: []desiredstate.ContainerInfo{
			{ID: "aaa", State: "running"},
			{ID: "bbb", State: "running"},
			{ID: "ccc", State: "exited"},
		},
	}
	sm := reconcile.NewStateManager(store, compose, nil, slog.Default())

	sm.UpdateContainerCounts(context.Background(), "stacks/app", "proj")

	snap := store.Get()
	if snap.Stacks[0].ContainersRunning != 2 {
		t.Errorf("ContainersRunning = %d, want 2", snap.Stacks[0].ContainersRunning)
	}
	if snap.Stacks[0].ContainersTotal != 3 {
		t.Errorf("ContainersTotal = %d, want 3", snap.Stacks[0].ContainersTotal)
	}
}

func TestUpdateContainerCounts_PsError(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "stacks/app", Status: desiredstate.StackSyncSynced, ContainersRunning: 5},
		},
	})

	compose := &psStubComposeRunner{psErr: errors.New("ps failed")}
	sm := reconcile.NewStateManager(store, compose, nil, slog.Default())

	sm.UpdateContainerCounts(context.Background(), "stacks/app", "proj")

	// Counts should remain unchanged on error
	snap := store.Get()
	if snap.Stacks[0].ContainersRunning != 5 {
		t.Errorf("ContainersRunning = %d, want 5 (unchanged)", snap.Stacks[0].ContainersRunning)
	}
}

func TestUpdateContainerCounts_NilSnapshot(t *testing.T) {
	store := desiredstate.NewStore()
	// No snapshot set — store.Get() returns nil

	compose := &psStubComposeRunner{
		containers: []desiredstate.ContainerInfo{{ID: "a", State: "running"}},
	}
	sm := reconcile.NewStateManager(store, compose, nil, slog.Default())

	// Should not panic
	sm.UpdateContainerCounts(context.Background(), "stacks/app", "proj")
}
