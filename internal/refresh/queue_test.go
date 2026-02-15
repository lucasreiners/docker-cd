package refresh_test

import (
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/refresh"
)

func TestQueue_EnqueueFirst_ReturnsRefreshing(t *testing.T) {
	q := refresh.NewQueue()
	result := q.Enqueue(refresh.Trigger{Source: refresh.TriggerManual, RequestedAt: time.Now()})
	if result != refresh.QueueResultRefreshing {
		t.Errorf("expected refreshing, got %q", result)
	}
}

func TestQueue_EnqueueWhileRunning_ReturnsQueued(t *testing.T) {
	q := refresh.NewQueue()
	q.Enqueue(refresh.Trigger{Source: refresh.TriggerManual, RequestedAt: time.Now()})
	<-q.TriggerChan()
	result := q.Enqueue(refresh.Trigger{Source: refresh.TriggerWebhook, RequestedAt: time.Now()})
	if result != refresh.QueueResultQueued {
		t.Errorf("expected queued, got %q", result)
	}
}

func TestQueue_SingleSlotReplacement(t *testing.T) {
	q := refresh.NewQueue()
	q.Enqueue(refresh.Trigger{Source: refresh.TriggerManual, RequestedAt: time.Now()})
	<-q.TriggerChan()
	q.Enqueue(refresh.Trigger{Source: refresh.TriggerWebhook, RequestedAt: time.Now()})
	q.Enqueue(refresh.Trigger{Source: refresh.TriggerPeriodic, RequestedAt: time.Now()})
	promoted := q.Done()
	if !promoted {
		t.Fatal("expected pending trigger to be promoted")
	}
	trigger := <-q.TriggerChan()
	if trigger.Source != refresh.TriggerPeriodic {
		t.Errorf("expected periodic trigger (latest), got %q", trigger.Source)
	}
}

func TestQueue_Done_NoPending_ReturnsFalse(t *testing.T) {
	q := refresh.NewQueue()
	q.Enqueue(refresh.Trigger{Source: refresh.TriggerManual, RequestedAt: time.Now()})
	<-q.TriggerChan()
	promoted := q.Done()
	if promoted {
		t.Error("expected no promotion when no pending trigger")
	}
}

func TestQueue_IsRunning(t *testing.T) {
	q := refresh.NewQueue()
	if q.IsRunning() {
		t.Error("expected not running initially")
	}
	q.Enqueue(refresh.Trigger{Source: refresh.TriggerManual, RequestedAt: time.Now()})
	<-q.TriggerChan()
	if !q.IsRunning() {
		t.Error("expected running after enqueue")
	}
	q.Done()
	if q.IsRunning() {
		t.Error("expected not running after Done")
	}
}
