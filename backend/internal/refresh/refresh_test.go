package refresh_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/git"
	"github.com/lucasreiners/docker-cd/internal/operations"
	"github.com/lucasreiners/docker-cd/internal/refresh"
)

type mockComposeReader struct {
	entries       []git.ComposeEntry
	commit        string
	commitMessage string
	err           error
	calls         int
}

func (m *mockComposeReader) ReadComposeFiles(_ context.Context, _, _, _, _ string) ([]git.ComposeEntry, string, string, error) {
	m.calls++
	return m.entries, m.commit, m.commitMessage, m.err
}

func TestService_StartupRefresh(t *testing.T) {
	reader := &mockComposeReader{
		entries: []git.ComposeEntry{
			{StackPath: "app1", ComposeFile: "docker-compose.yml", Content: []byte("version: '3'")},
		},
		commit: "abc123def456",
	}

	store := desiredstate.NewStore()
	queue := refresh.NewQueue()
	cfg := config.Config{
		GitRepoURL:     "https://github.com/org/repo.git",
		GitAccessToken: "tok",
		GitRevision:    "main",
	}

	svc := refresh.NewService(cfg, store, queue, reader)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go svc.Start(ctx)

	// Wait for startup refresh to complete
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for startup refresh")
		default:
			snap := store.Get()
			if snap != nil && snap.RefreshStatus == desiredstate.RefreshStatusCompleted {
				if snap.Revision != "abc123def456" {
					t.Errorf("expected revision abc123def456, got %q", snap.Revision)
				}
				if len(snap.Stacks) != 1 {
					t.Errorf("expected 1 stack, got %d", len(snap.Stacks))
				}
				if snap.Stacks[0].Path != "app1" {
					t.Errorf("expected stack path app1, got %q", snap.Stacks[0].Path)
				}
				if reader.calls < 1 {
					t.Errorf("expected at least 1 reader call, got %d", reader.calls)
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestRefreshWorkFinishesBeforeChildAndRetainsCompletedRoot(t *testing.T) {
	store := desiredstate.NewStore()
	b := desiredstate.NewBroadcaster()
	reader := &mockComposeReader{commit: "revision", entries: []git.ComposeEntry{{StackPath: "web", ComposeFile: "compose.yaml"}}}
	svc := refresh.NewService(config.Config{GitRevision: "main"}, store, refresh.NewQueue(), reader)
	svc.SetBroadcaster(b)
	sub := b.Subscribe()
	defer b.Unsubscribe(sub)
	done := make(chan struct{})
	release := make(chan struct{})
	svc.SetReconcileFunc(func(ctx context.Context) {
		defer close(done)
		root, ok := b.Operations().Get(operations.ID(ctx))
		if !ok || root.Kind != "refresh" || root.Status != "completed" {
			t.Error("refresh boundary extended through child execution")
		}
		child := b.BeginOperation(ctx, "deployment", "startup", "web", "revision")
		defer b.EndOperation(child, "completed", "done")
		defer func() { <-release }()
		s, events := b.SubscribeWithSnapshot(store)
		defer b.Unsubscribe(s)
		var snapshot desiredstate.WorkSnapshot
		if err := json.Unmarshal([]byte(events[3].Data), &snapshot); err != nil {
			t.Error(err)
			return
		}
		if len(snapshot.Runs) != 0 || len(snapshot.Operations) != 2 {
			t.Error("completed refresh run retained or child ancestry missing")
		}
		for _, op := range snapshot.Operations {
			if op.RootOperationID != root.ID {
				t.Error("child became a duplicate workflow root")
			}
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() { defer close(stopped); svc.Start(ctx) }()
	// The child blocks so the completed-parent snapshot is authoritative while
	// this test observes the terminal stream (no sleeps or racy reader mutations).
	deadline := time.After(time.Second)
	found := false
	for !found {
		select {
		case event := <-sub.Events:
			if event.Type != desiredstate.EventWorkUpsert {
				continue
			}
			var up desiredstate.WorkUpsert
			if err := json.Unmarshal([]byte(event.Data), &up); err != nil {
				t.Fatal(err)
			}
			if up.Run != nil && up.Run.Status == "completed" {
				if up.Run.Source != "refresh" || up.Run.Phase != "finished" || up.Run.ImageCount != 0 {
					t.Error("refresh fabricated image work")
				}
				found = true
			}
		case <-deadline:
			t.Error("missing refresh terminal")
			found = true
		}
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("child callback did not finish")
	}
	cancel()
	<-stopped
}

func TestService_RequestRefresh(t *testing.T) {
	reader := &mockComposeReader{
		entries: []git.ComposeEntry{
			{StackPath: "app1", ComposeFile: "docker-compose.yml", Content: []byte("v1")},
		},
		commit: "commit1",
	}

	store := desiredstate.NewStore()
	queue := refresh.NewQueue()
	cfg := config.Config{
		GitRepoURL:     "https://github.com/org/repo.git",
		GitAccessToken: "tok",
		GitRevision:    "main",
	}

	svc := refresh.NewService(cfg, store, queue, reader)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go svc.Start(ctx)

	// Wait for startup refresh to complete
	time.Sleep(500 * time.Millisecond)

	// Request a manual refresh
	result := svc.RequestRefresh(refresh.TriggerManual)
	// It should either be refreshing or queued
	if result != refresh.QueueResultRefreshing && result != refresh.QueueResultQueued {
		t.Errorf("expected refreshing or queued, got %q", result)
	}
}

func TestService_PreserveSyncStatus(t *testing.T) {
	content := []byte("version: '3'")
	reader := &mockComposeReader{
		entries: []git.ComposeEntry{
			{StackPath: "app1", ComposeFile: "docker-compose.yml", Content: content},
		},
		commit: "commit1",
	}

	store := desiredstate.NewStore()
	queue := refresh.NewQueue()
	cfg := config.Config{
		GitRepoURL:     "https://github.com/org/repo.git",
		GitAccessToken: "tok",
		GitRevision:    "main",
	}

	svc := refresh.NewService(cfg, store, queue, reader)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go svc.Start(ctx)

	// Wait for startup refresh
	time.Sleep(500 * time.Millisecond)

	// Manually set sync status on a stack
	snap := store.Get()
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	snap.Stacks[0].Status = desiredstate.StackSyncSynced
	snap.Stacks[0].ContainersRunning = 2
	snap.Stacks[0].ContainersTotal = 2
	snap.Stacks[0].SyncedRevision = "deployed-commit"
	store.Set(snap)

	// Trigger another refresh (same content, same hash)
	svc.RequestRefresh(refresh.TriggerManual)
	time.Sleep(500 * time.Millisecond)

	// Verify sync status was preserved
	snap = store.Get()
	if snap == nil {
		t.Fatal("expected non-nil snapshot after second refresh")
	}
	if len(snap.Stacks) != 1 {
		t.Fatalf("expected 1 stack, got %d", len(snap.Stacks))
	}
	if snap.Stacks[0].Status != desiredstate.StackSyncSynced {
		t.Errorf("expected synced status preserved, got %q", snap.Stacks[0].Status)
	}
	if snap.Stacks[0].ContainersRunning != 2 || snap.Stacks[0].SyncedRevision != "deployed-commit" {
		t.Fatalf("refresh discarded runtime metadata: %+v", snap.Stacks[0])
	}
}

func TestService_RefreshFailure(t *testing.T) {
	reader := &mockComposeReader{
		err: context.DeadlineExceeded,
	}

	store := desiredstate.NewStore()
	queue := refresh.NewQueue()
	cfg := config.Config{
		GitRepoURL:     "https://github.com/org/repo.git",
		GitAccessToken: "tok",
		GitRevision:    "main",
	}

	svc := refresh.NewService(cfg, store, queue, reader)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go svc.Start(ctx)

	// Wait for failed refresh
	time.Sleep(500 * time.Millisecond)

	snap := store.Get()
	if snap == nil {
		t.Fatal("expected non-nil snapshot after failed refresh")
	}
	if snap.RefreshStatus != desiredstate.RefreshStatusFailed {
		t.Errorf("expected failed status, got %q", snap.RefreshStatus)
	}
	if snap.RefreshError == "" {
		t.Error("expected non-empty refresh error")
	}
}
