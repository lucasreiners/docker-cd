package reconcile

import (
	"context"
	"log/slog"
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/events"
)

// StateManager handles all state updates for stack records and publishes changes.
type StateManager struct {
	store    *desiredstate.Store
	eventBus *events.EventBus
	logger   *slog.Logger
	compose  ComposeRunner
}

// NewStateManager creates a new state manager.
func NewStateManager(
	store *desiredstate.Store,
	compose ComposeRunner,
	eventBus *events.EventBus,
	logger *slog.Logger,
) *StateManager {
	return &StateManager{
		store:    store,
		eventBus: eventBus,
		compose:  compose,
		logger:   logger,
	}
}

// UpdateStatus updates the sync status of a stack.
func (sm *StateManager) UpdateStatus(path string, status desiredstate.StackSyncStatus, syncedAt, syncError string) {
	snap := sm.store.Get()
	if snap == nil {
		sm.logger.Warn("cannot update status, store snapshot is nil", "stack_path", path)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var updated *desiredstate.StackRecord

	for i := range snap.Stacks {
		if snap.Stacks[i].Path == path {
			snap.Stacks[i].Status = status
			snap.Stacks[i].LastSyncAt = now
			snap.Stacks[i].LastSyncStatus = string(status)
			if syncError != "" {
				snap.Stacks[i].LastSyncError = syncError
			}
			updated = &snap.Stacks[i]
			break
		}
	}

	sm.store.Set(snap)

	if updated != nil {
		sm.logger.Info("stack status updated",
			"stack_path", path,
			"status", status)

		// Publish domain event
		if sm.eventBus != nil {
			sm.eventBus.Publish(context.Background(),
				events.NewStackStatusChangedEvent(path, status, syncError))
		}
	}
}

// MarkSynced marks a stack as successfully synced with the given metadata.
func (sm *StateManager) MarkSynced(path, revision, commitMessage, composeHash, syncedAt string) {
	snap := sm.store.Get()
	if snap == nil {
		sm.logger.Warn("cannot mark synced, store snapshot is nil", "stack_path", path)
		return
	}

	found := false
	var updated *desiredstate.StackRecord

	for i := range snap.Stacks {
		if snap.Stacks[i].Path == path {
			snap.Stacks[i].Status = desiredstate.StackSyncSynced
			snap.Stacks[i].SyncedRevision = revision
			snap.Stacks[i].SyncedCommitMessage = commitMessage
			snap.Stacks[i].SyncedComposeHash = composeHash
			snap.Stacks[i].SyncedAt = syncedAt
			snap.Stacks[i].LastSyncAt = syncedAt
			snap.Stacks[i].LastSyncStatus = string(desiredstate.StackSyncSynced)
			snap.Stacks[i].LastSyncError = ""
			found = true
			updated = &snap.Stacks[i]
			break
		}
	}

	if !found {
		sm.logger.Warn("stack not found when marking synced",
			"stack_path", path,
			"total_stacks", len(snap.Stacks))
	} else {
		sm.logger.Debug("stack marked as synced",
			"stack_path", path,
			"revision", revision)
	}

	sm.store.Set(snap)

	if updated != nil {
		// Publish domain event
		if sm.eventBus != nil {
			sm.eventBus.Publish(context.Background(),
				events.NewStackSyncedEvent(path, revision, composeHash, commitMessage))
		}
	}
}

// UpdateContainerCounts queries container status for a stack and updates the store.
func (sm *StateManager) UpdateContainerCounts(ctx context.Context, stackPath, projectName string) {
	containers, err := sm.compose.ComposePs(ctx, projectName)
	if err != nil {
		sm.logger.Warn("failed to get container counts",
			"stack_path", stackPath,
			"error", err)
		return
	}

	running := 0
	for _, c := range containers {
		if c.State == "running" {
			running++
		}
	}

	snap := sm.store.Get()
	if snap == nil {
		return
	}

	var updated *desiredstate.StackRecord
	for i := range snap.Stacks {
		if snap.Stacks[i].Path == stackPath {
			snap.Stacks[i].ContainersRunning = running
			snap.Stacks[i].ContainersTotal = len(containers)
			updated = &snap.Stacks[i]
			break
		}
	}

	sm.store.Set(snap)

	if updated != nil {
		sm.logger.Debug("container counts updated",
			"stack_path", stackPath,
			"running", running,
			"total", len(containers))

		// Publish domain event
		if sm.eventBus != nil {
			sm.eventBus.Publish(context.Background(),
				events.NewContainersUpdatedEvent(stackPath, running, len(containers)))
		}
	}
}
