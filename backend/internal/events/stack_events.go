package events

import (
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
)

const (
	EventTypeStackStatusChanged = "stack.status.changed"
	EventTypeStackSynced        = "stack.synced"
	EventTypeStackRemoved       = "stack.removed"
	EventTypeContainersUpdated  = "stack.containers.updated"
	EventTypeDriftDetected      = "stack.drift.detected"
)

// baseEvent provides common event fields.
type baseEvent struct {
	eventType  string
	occurredAt time.Time
	metadata   map[string]any
}

func (e *baseEvent) EventType() string {
	return e.eventType
}

func (e *baseEvent) OccurredAt() time.Time {
	return e.occurredAt
}

func (e *baseEvent) Metadata() map[string]any {
	return e.metadata
}

// StackStatusChangedEvent is published when a stack's status changes.
type StackStatusChangedEvent struct {
	baseEvent
	StackPath string
	Status    desiredstate.StackSyncStatus
	Error     string
}

// NewStackStatusChangedEvent creates a status change event.
func NewStackStatusChangedEvent(stackPath string, status desiredstate.StackSyncStatus, errorMsg string) *StackStatusChangedEvent {
	return &StackStatusChangedEvent{
		baseEvent: baseEvent{
			eventType:  EventTypeStackStatusChanged,
			occurredAt: time.Now().UTC(),
			metadata: map[string]any{
				"stack_path": stackPath,
				"status":     string(status),
			},
		},
		StackPath: stackPath,
		Status:    status,
		Error:     errorMsg,
	}
}

// StackSyncedEvent is published when a stack is successfully synced.
type StackSyncedEvent struct {
	baseEvent
	StackPath     string
	Revision      string
	ComposeHash   string
	CommitMessage string
}

// NewStackSyncedEvent creates a stack synced event.
func NewStackSyncedEvent(stackPath, revision, composeHash, commitMessage string) *StackSyncedEvent {
	return &StackSyncedEvent{
		baseEvent: baseEvent{
			eventType:  EventTypeStackSynced,
			occurredAt: time.Now().UTC(),
			metadata: map[string]any{
				"stack_path":   stackPath,
				"revision":     revision,
				"compose_hash": composeHash,
			},
		},
		StackPath:     stackPath,
		Revision:      revision,
		ComposeHash:   composeHash,
		CommitMessage: commitMessage,
	}
}

// StackRemovedEvent is published when a stack is removed.
type StackRemovedEvent struct {
	baseEvent
	StackPath string
	Reason    string
}

// NewStackRemovedEvent creates a stack removed event.
func NewStackRemovedEvent(stackPath, reason string) *StackRemovedEvent {
	return &StackRemovedEvent{
		baseEvent: baseEvent{
			eventType:  EventTypeStackRemoved,
			occurredAt: time.Now().UTC(),
			metadata: map[string]any{
				"stack_path": stackPath,
				"reason":     reason,
			},
		},
		StackPath: stackPath,
		Reason:    reason,
	}
}

// ContainersUpdatedEvent is published when container counts are updated.
type ContainersUpdatedEvent struct {
	baseEvent
	StackPath    string
	RunningCount int
	TotalCount   int
}

// NewContainersUpdatedEvent creates a containers updated event.
func NewContainersUpdatedEvent(stackPath string, running, total int) *ContainersUpdatedEvent {
	return &ContainersUpdatedEvent{
		baseEvent: baseEvent{
			eventType:  EventTypeContainersUpdated,
			occurredAt: time.Now().UTC(),
			metadata: map[string]any{
				"stack_path":    stackPath,
				"running_count": running,
				"total_count":   total,
			},
		},
		StackPath:    stackPath,
		RunningCount: running,
		TotalCount:   total,
	}
}

// DriftDetectedEvent is published when drift is detected for a stack.
type DriftDetectedEvent struct {
	baseEvent
	StackPath string
	Reason    string
	NeedSync  bool
}

// NewDriftDetectedEvent creates a drift detected event.
func NewDriftDetectedEvent(stackPath, reason string, needSync bool) *DriftDetectedEvent {
	return &DriftDetectedEvent{
		baseEvent: baseEvent{
			eventType:  EventTypeDriftDetected,
			occurredAt: time.Now().UTC(),
			metadata: map[string]any{
				"stack_path": stackPath,
				"reason":     reason,
				"need_sync":  needSync,
			},
		},
		StackPath: stackPath,
		Reason:    reason,
		NeedSync:  needSync,
	}
}
