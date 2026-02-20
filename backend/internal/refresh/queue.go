package refresh

import (
	"sync"
	"time"
)

// TriggerSource identifies what triggered a refresh.
type TriggerSource string

const (
	TriggerStartup  TriggerSource = "startup"
	TriggerWebhook  TriggerSource = "webhook"
	TriggerManual   TriggerSource = "manual"
	TriggerPeriodic TriggerSource = "periodic"
)

// Trigger represents a refresh request.
type Trigger struct {
	Source      TriggerSource
	RequestedAt time.Time
}

// QueueResult indicates what happened when a trigger was enqueued.
type QueueResult string

const (
	QueueResultRefreshing QueueResult = "refreshing"
	QueueResultQueued     QueueResult = "queued"
)

// Queue implements single-slot replacement semantics:
// one in-flight refresh and at most one queued refresh that can be replaced.
type Queue struct {
	mu        sync.Mutex
	running   bool
	pending   *Trigger
	triggerCh chan Trigger
}

// NewQueue creates a Queue with a trigger channel.
func NewQueue() *Queue {
	return &Queue{
		triggerCh: make(chan Trigger, 1),
	}
}

// Enqueue adds a refresh trigger. If a refresh is already running, the pending
// slot is replaced (single-slot replacement). Returns whether the trigger
// started immediately or was queued.
func (q *Queue) Enqueue(t Trigger) QueueResult {
	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.running {
		q.running = true
		select {
		case q.triggerCh <- t:
		default:
		}
		return QueueResultRefreshing
	}

	q.pending = &t
	return QueueResultQueued
}

// TriggerChan returns the channel that emits triggers when a refresh should run.
func (q *Queue) TriggerChan() <-chan Trigger {
	return q.triggerCh
}

// Done marks the current refresh as complete and promotes any pending trigger.
// Returns true if a pending trigger was promoted.
func (q *Queue) Done() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.running = false

	if q.pending != nil {
		t := *q.pending
		q.pending = nil
		q.running = true
		select {
		case q.triggerCh <- t:
		default:
		}
		return true
	}

	return false
}

// IsRunning returns whether a refresh is currently in progress.
func (q *Queue) IsRunning() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.running
}
