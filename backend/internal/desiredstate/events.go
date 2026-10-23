package desiredstate

import (
	"encoding/json"
	"fmt"
	"github.com/lucasreiners/docker-cd/internal/operations"
	"sort"
	"sync"
	"sync/atomic"
)

// EventType identifies the kind of SSE event.
type EventType string

const (
	EventStackSnapshot   EventType = "stack.snapshot"
	EventStackUpsert     EventType = "stack.upsert"
	EventStackDelete     EventType = "stack.delete"
	EventRefreshStatus   EventType = "refresh.status"
	EventUpdateStarted   EventType = "update.started"
	EventUpdateProgress  EventType = "update.progress"
	EventUpdateSnapshot  EventType = "update.snapshot"
	EventUpdateCompleted EventType = "update.completed"
	EventWorkSnapshot    EventType = "work.snapshot"
	EventWorkUpsert      EventType = "work.upsert"
)

// SSEEvent represents a single event to be sent over the SSE stream.
type SSEEvent struct {
	ID   string    `json:"id"`
	Type EventType `json:"event"`
	Data string    `json:"data"` // JSON-encoded payload
}

// Subscriber receives events via a channel.
type Subscriber struct {
	Events    chan SSEEvent
	done      chan struct{}
	closeOnce sync.Once
}

// Close signals the subscriber to stop.
func (s *Subscriber) Close() {
	s.closeOnce.Do(func() { close(s.done) })
}

// Done returns a channel that is closed when the subscriber is closed.
func (s *Subscriber) Done() <-chan struct{} {
	return s.done
}

// Broadcaster fans out SSE events to all connected subscribers.
type Broadcaster struct {
	operations  *operations.Store
	mu          sync.RWMutex
	subscribers map[*Subscriber]struct{}
	eventID     atomic.Int64
	progress    map[string]json.RawMessage
	pulls       map[string]map[string]json.RawMessage
	work        workState
}

// NewBroadcaster creates a new SSE event broadcaster.
func NewBroadcaster() *Broadcaster {
	operationStore, _ := operations.Open("")
	return &Broadcaster{
		operations:  operationStore,
		subscribers: make(map[*Subscriber]struct{}),
		progress:    make(map[string]json.RawMessage),
		pulls:       make(map[string]map[string]json.RawMessage),
		work:        newWorkState(),
	}
}

// SubscribeWithSnapshot captures bootstrap state while publication is paused.
// Bootstrap events belong only to this connection, including on reconnect.
func (b *Broadcaster) SubscribeWithSnapshot(store *Store) (*Subscriber, []SSEEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sub := &Subscriber{Events: make(chan SSEEvent, 64), done: make(chan struct{})}
	b.subscribers[sub] = struct{}{}
	snap := store.Get()
	if snap == nil {
		snap = &Snapshot{RefreshStatus: RefreshStatusQueued, UpdatesBlocked: true, BlockedReason: "refresh pending"}
	}
	stacks := snap.Stacks
	if stacks == nil {
		stacks = []StackRecord{}
	}
	snap.Stacks = nil
	progress := make([]json.RawMessage, 0, len(b.progress))
	sources := make([]string, 0, len(b.progress))
	for source := range b.progress {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	for _, source := range sources {
		progress = append(progress, b.progress[source])
		keys := make([]string, 0, len(b.pulls[source]))
		for key := range b.pulls[source] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			progress = append(progress, b.pulls[source][key])
		}
	}
	return sub, []SSEEvent{
		b.newEvent(EventStackSnapshot, map[string]interface{}{"records": stacks}),
		b.newEvent(EventRefreshStatus, snap),
		b.newEvent(EventUpdateSnapshot, map[string]interface{}{"progress": progress}),
		b.newEvent(EventWorkSnapshot, b.workSnapshotLocked()),
	}
}

func (b *Broadcaster) newEvent(eventType EventType, payload interface{}) SSEEvent {
	data, _ := json.Marshal(payload)
	return SSEEvent{ID: fmt.Sprintf("%d", b.eventID.Add(1)), Type: eventType, Data: string(data)}
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
// buffer is full the connection is closed so it reconnects with fresh state.
func (b *Broadcaster) Publish(eventType EventType, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if eventType == EventUpdateProgress {
		var progress struct {
			Source  string `json:"source"`
			Type    string `json:"type"`
			Image   string `json:"image"`
			LayerID string `json:"layerId"`
		}
		if json.Unmarshal(data, &progress) == nil {
			if progress.Type == "image_pull_progress" {
				if b.pulls[progress.Source] == nil {
					b.pulls[progress.Source] = make(map[string]json.RawMessage)
				}
				b.pulls[progress.Source][progress.Image+"\x00"+progress.LayerID] = json.RawMessage(data)
				if b.progress[progress.Source] == nil {
					b.progress[progress.Source] = json.RawMessage(data)
				}
			} else {
				b.progress[progress.Source] = json.RawMessage(data)
				delete(b.pulls, progress.Source)
			}
		}
	}
	b.publishLocked(eventType, json.RawMessage(data))
}

// All v2 state and operation mutations take mu before accessing the operation
// store. The store never calls back into the broadcaster (no inverse lock).
func (b *Broadcaster) publishLocked(eventType EventType, payload interface{}) {
	event := b.newEvent(eventType, payload)
	for sub := range b.subscribers {
		select {
		case sub.Events <- event:
		default:
			sub.Close()
			delete(b.subscribers, sub)
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

// PublishUpdateProgress sends an update cycle progress event.
func (b *Broadcaster) PublishUpdateProgress(payload interface{}) {
	b.Publish(EventUpdateProgress, payload)
}

// SubscriberCount returns the number of active subscribers.
func (b *Broadcaster) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}
