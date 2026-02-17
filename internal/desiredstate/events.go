package desiredstate

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// EventType identifies the kind of SSE event.
type EventType string

const (
	EventStackSnapshot EventType = "stack.snapshot"
	EventStackUpsert   EventType = "stack.upsert"
	EventStackDelete   EventType = "stack.delete"
	EventRefreshStatus EventType = "refresh.status"
)

// SSEEvent represents a single event to be sent over the SSE stream.
type SSEEvent struct {
	ID   string    `json:"id"`
	Type EventType `json:"event"`
	Data string    `json:"data"` // JSON-encoded payload
}

// Subscriber receives events via a channel.
type Subscriber struct {
	Events chan SSEEvent
	done   chan struct{}
}

// Close signals the subscriber to stop.
func (s *Subscriber) Close() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

// Done returns a channel that is closed when the subscriber is closed.
func (s *Subscriber) Done() <-chan struct{} {
	return s.done
}

// Broadcaster fans out SSE events to all connected subscribers.
type Broadcaster struct {
	mu          sync.RWMutex
	subscribers map[*Subscriber]struct{}
	eventID     atomic.Int64
}

// NewBroadcaster creates a new SSE event broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[*Subscriber]struct{}),
	}
}

// Subscribe adds a new subscriber and returns it.
// The caller must call Unsubscribe when done.
func (b *Broadcaster) Subscribe() *Subscriber {
	sub := &Subscriber{
		Events: make(chan SSEEvent, 64),
		done:   make(chan struct{}),
	}
	b.mu.Lock()
	b.subscribers[sub] = struct{}{}
	b.mu.Unlock()
	return sub
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *Broadcaster) Unsubscribe(sub *Subscriber) {
	b.mu.Lock()
	delete(b.subscribers, sub)
	b.mu.Unlock()
	sub.Close()
}

// Publish sends an event to all subscribers. Non-blocking: if a subscriber
// buffer is full the event is dropped for that subscriber.
func (b *Broadcaster) Publish(eventType EventType, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	id := b.eventID.Add(1)
	event := SSEEvent{
		ID:   fmt.Sprintf("%d", id),
		Type: eventType,
		Data: string(data),
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for sub := range b.subscribers {
		select {
		case sub.Events <- event:
		default:
			// subscriber buffer full, drop event
		}
	}
}

// PublishStackSnapshot sends a full snapshot of all stacks.
func (b *Broadcaster) PublishStackSnapshot(stacks []StackRecord) {
	type snapshotPayload struct {
		Records []StackRecord `json:"records"`
	}
	b.Publish(EventStackSnapshot, snapshotPayload{Records: stacks})
}

// PublishStackUpsert sends a single full stack record update.
func (b *Broadcaster) PublishStackUpsert(stack StackRecord) {
	type upsertPayload struct {
		Record StackRecord `json:"record"`
	}
	b.Publish(EventStackUpsert, upsertPayload{Record: stack})
}

// PublishRefreshStatus sends a refresh status update.
func (b *Broadcaster) PublishRefreshStatus(snap *Snapshot) {
	b.Publish(EventRefreshStatus, snap)
}

// SubscriberCount returns the number of active subscribers.
func (b *Broadcaster) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}
