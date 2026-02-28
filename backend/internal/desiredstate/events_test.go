package desiredstate_test

import (
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
)

func TestNewBroadcaster(t *testing.T) {
	b := desiredstate.NewBroadcaster()
	if b == nil {
		t.Fatal("NewBroadcaster returned nil")
	}
	if b.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers, got %d", b.SubscriberCount())
	}
}

func TestBroadcaster_Subscribe(t *testing.T) {
	b := desiredstate.NewBroadcaster()
	sub := b.Subscribe()
	if sub == nil {
		t.Fatal("Subscribe returned nil")
	}
	if sub.Events == nil {
		t.Error("subscriber Events channel is nil")
	}
	if sub.Done() == nil {
		t.Error("subscriber Done channel is nil")
	}
	if b.SubscriberCount() != 1 {
		t.Errorf("expected 1 subscriber, got %d", b.SubscriberCount())
	}
}

func TestBroadcaster_Unsubscribe(t *testing.T) {
	b := desiredstate.NewBroadcaster()
	sub1 := b.Subscribe()
	sub2 := b.Subscribe()
	if b.SubscriberCount() != 2 {
		t.Errorf("expected 2 subscribers, got %d", b.SubscriberCount())
	}
	b.Unsubscribe(sub1)
	if b.SubscriberCount() != 1 {
		t.Errorf("expected 1 subscriber after unsubscribe, got %d", b.SubscriberCount())
	}
	select {
	case <-sub1.Done():
	case <-time.After(100 * time.Millisecond):
		t.Error("subscriber Done channel should be closed after Unsubscribe")
	}
	b.Unsubscribe(sub2)
	if b.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers after all unsubscribed, got %d", b.SubscriberCount())
	}
}

func TestBroadcaster_Publish(t *testing.T) {
	b := desiredstate.NewBroadcaster()
	sub := b.Subscribe()
	payload := map[string]string{"key": "value"}
	b.Publish(desiredstate.EventStackSnapshot, payload)
	select {
	case event := <-sub.Events:
		if event.Type != desiredstate.EventStackSnapshot {
			t.Errorf("expected event type %s, got %s", desiredstate.EventStackSnapshot, event.Type)
		}
		if event.ID == "" {
			t.Error("event ID should not be empty")
		}
		if event.Data != `{"key":"value"}` {
			t.Errorf("expected data {\"key\":\"value\"}, got %s", event.Data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestBroadcaster_PublishToMultiple(t *testing.T) {
	b := desiredstate.NewBroadcaster()
	sub1 := b.Subscribe()
	sub2 := b.Subscribe()
	sub3 := b.Subscribe()
	payload := map[string]int{"count": 42}
	b.Publish(desiredstate.EventUpdateProgress, payload)
	for i, sub := range []*desiredstate.Subscriber{sub1, sub2, sub3} {
		select {
		case event := <-sub.Events:
			if event.Type != desiredstate.EventUpdateProgress {
				t.Errorf("subscriber %d: expected type %s, got %s", i+1, desiredstate.EventUpdateProgress, event.Type)
			}
			if event.Data != `{"count":42}` {
				t.Errorf("subscriber %d: unexpected data %s", i+1, event.Data)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("subscriber %d: timeout waiting for event", i+1)
		}
	}
}

func TestBroadcaster_EventIDIncremental(t *testing.T) {
	b := desiredstate.NewBroadcaster()
	sub := b.Subscribe()
	for i := 0; i < 5; i++ {
		b.Publish(desiredstate.EventUpdateProgress, map[string]int{"seq": i})
	}
	var lastID string
	for i := 0; i < 5; i++ {
		select {
		case event := <-sub.Events:
			if lastID != "" && event.ID <= lastID {
				t.Errorf("event %d: ID %s should be greater than previous %s", i, event.ID, lastID)
			}
			lastID = event.ID
		case <-time.After(100 * time.Millisecond):
			t.Errorf("timeout waiting for event %d", i)
		}
	}
}

func TestBroadcaster_DropsEventsOnFullBuffer(t *testing.T) {
	b := desiredstate.NewBroadcaster()
	sub := b.Subscribe()
	for i := 0; i < 64; i++ {
		b.Publish(desiredstate.EventUpdateProgress, map[string]int{"seq": i})
	}
	for i := 64; i < 80; i++ {
		b.Publish(desiredstate.EventUpdateProgress, map[string]int{"seq": i})
	}
	count := 0
	for {
		select {
		case <-sub.Events:
			count++
		case <-time.After(10 * time.Millisecond):
			if count != 64 {
				t.Errorf("expected 64 events in buffer, got %d", count)
			}
			return
		}
	}
}

func TestBroadcaster_PublishStackSnapshot(t *testing.T) {
	b := desiredstate.NewBroadcaster()
	sub := b.Subscribe()
	stacks := []desiredstate.StackRecord{
		{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "h1", Status: desiredstate.StackSyncSynced},
		{Path: "app2", ComposeFile: "docker-compose.yml", ComposeHash: "h2", Status: desiredstate.StackSyncSyncing},
	}
	b.PublishStackSnapshot(stacks)
	select {
	case event := <-sub.Events:
		if event.Type != desiredstate.EventStackSnapshot {
			t.Errorf("expected type %s, got %s", desiredstate.EventStackSnapshot, event.Type)
		}
		if event.Data == "" {
			t.Error("event data should not be empty")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for snapshot event")
	}
}

func TestBroadcaster_PublishStackUpsert(t *testing.T) {
	b := desiredstate.NewBroadcaster()
	sub := b.Subscribe()
	stack := desiredstate.StackRecord{
		Path:        "app1",
		ComposeFile: "docker-compose.yml",
		ComposeHash: "hash1",
		Status:      desiredstate.StackSyncSynced,
	}
	b.PublishStackUpsert(stack)
	select {
	case event := <-sub.Events:
		if event.Type != desiredstate.EventStackUpsert {
			t.Errorf("expected type %s, got %s", desiredstate.EventStackUpsert, event.Type)
		}
		if event.Data == "" {
			t.Error("event data should not be empty")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for upsert event")
	}
}

func TestBroadcaster_ConcurrentPublish(t *testing.T) {
	b := desiredstate.NewBroadcaster()
	sub := b.Subscribe()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			b.Publish(desiredstate.EventUpdateProgress, map[string]int{"seq": i})
			time.Sleep(time.Millisecond)
		}
		close(done)
	}()
	count := 0
	timeout := time.After(5 * time.Second)
	for {
		select {
		case <-sub.Events:
			count++
		case <-done:
			if count == 0 {
				t.Error("expected to receive at least some events")
			}
			return
		case <-timeout:
			t.Error("timeout waiting for concurrent events")
			return
		}
	}
}

func TestBroadcaster_NoSubscribers(t *testing.T) {
	b := desiredstate.NewBroadcaster()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("publishing with no subscribers caused panic: %v", r)
		}
	}()
	b.Publish(desiredstate.EventStackSnapshot, map[string]string{"test": "data"})
	b.PublishStackSnapshot([]desiredstate.StackRecord{})
	b.PublishStackUpsert(desiredstate.StackRecord{Path: "test"})
}
