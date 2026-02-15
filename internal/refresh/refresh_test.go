package refresh_test

import (
	"context"
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/git"
	"github.com/lucasreiners/docker-cd/internal/refresh"
)

type mockComposeReader struct {
	entries []git.ComposeEntry
	commit  string
	err     error
	calls   int
}

func (m *mockComposeReader) ReadComposeFiles(_ context.Context, _, _, _, _ string) ([]git.ComposeEntry, string, error) {
	m.calls++
	return m.entries, m.commit, m.err
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
