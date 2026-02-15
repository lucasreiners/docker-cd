package desiredstate_test

import (
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
)

func TestNewStore_ReturnsNilOnGet(t *testing.T) {
	store := desiredstate.NewStore()
	if snap := store.Get(); snap != nil {
		t.Errorf("expected nil snapshot for new store, got %+v", snap)
	}
}

func TestStore_SetAndGet(t *testing.T) {
	store := desiredstate.NewStore()
	now := time.Now()
	snap := &desiredstate.Snapshot{
		Revision:      "abc123",
		Ref:           "main",
		RefType:       "branch",
		RefreshedAt:   now,
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Status: desiredstate.StackSyncSynced},
		},
	}
	store.Set(snap)
	got := store.Get()
	if got == nil {
		t.Fatal("expected non-nil snapshot after Set")
	}
	if got.Revision != "abc123" {
		t.Errorf("expected revision abc123, got %q", got.Revision)
	}
	if got.RefreshStatus != desiredstate.RefreshStatusCompleted {
		t.Errorf("expected completed status, got %q", got.RefreshStatus)
	}
	if len(got.Stacks) != 1 || got.Stacks[0].Path != "app1" {
		t.Errorf("unexpected stacks: %+v", got.Stacks)
	}
}

func TestStore_GetReturnsCopy(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision: "abc",
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "h1", Status: desiredstate.StackSyncSynced},
		},
	})
	got := store.Get()
	got.Revision = "modified"
	got.Stacks[0].Path = "modified"
	original := store.Get()
	if original.Revision != "abc" {
		t.Errorf("modifying copy should not affect store, revision is %q", original.Revision)
	}
	if original.Stacks[0].Path != "app1" {
		t.Errorf("modifying copy stacks should not affect store, path is %q", original.Stacks[0].Path)
	}
}

func TestStore_UpdateStatus(t *testing.T) {
	store := desiredstate.NewStore()
	store.UpdateStatus(desiredstate.RefreshStatusRefreshing, "")
	snap := store.Get()
	if snap == nil {
		t.Fatal("expected snapshot after UpdateStatus on nil store")
	}
	if snap.RefreshStatus != desiredstate.RefreshStatusRefreshing {
		t.Errorf("expected refreshing, got %q", snap.RefreshStatus)
	}
	store.UpdateStatus(desiredstate.RefreshStatusFailed, "clone failed")
	snap = store.Get()
	if snap.RefreshStatus != desiredstate.RefreshStatusFailed {
		t.Errorf("expected failed, got %q", snap.RefreshStatus)
	}
	if snap.RefreshError != "clone failed" {
		t.Errorf("expected error message, got %q", snap.RefreshError)
	}
}

func TestStore_GetStacks_Nil(t *testing.T) {
	store := desiredstate.NewStore()
	if stacks := store.GetStacks(); stacks != nil {
		t.Errorf("expected nil stacks for empty store, got %v", stacks)
	}
}

func TestStore_GetStacks_ReturnsCopy(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "h1", Status: desiredstate.StackSyncSynced},
			{Path: "app2", ComposeFile: "docker-compose.yaml", ComposeHash: "h2", Status: desiredstate.StackSyncMissing},
		},
	})
	stacks := store.GetStacks()
	stacks[0].Path = "modified"
	if original := store.GetStacks(); original[0].Path != "app1" {
		t.Errorf("modifying copy should not affect store")
	}
}

func TestStore_GetRefreshStatus_Nil(t *testing.T) {
	store := desiredstate.NewStore()
	if status := store.GetRefreshStatus(); status != nil {
		t.Errorf("expected nil for empty store, got %+v", status)
	}
}

func TestStore_GetRefreshStatus_ExcludesStacks(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "abc123",
		Ref:           "main",
		RefType:       "branch",
		RefreshedAt:   time.Now(),
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "h1", Status: desiredstate.StackSyncSynced},
		},
	})
	status := store.GetRefreshStatus()
	if status == nil {
		t.Fatal("expected non-nil refresh status")
	}
	if status.Revision != "abc123" {
		t.Errorf("expected revision abc123, got %q", status.Revision)
	}
	if len(status.Stacks) != 0 {
		t.Errorf("expected no stacks in refresh status, got %d", len(status.Stacks))
	}
}

func TestComposeHash(t *testing.T) {
	content := []byte("version: '3'\nservices:\n  web:\n    image: nginx\n")
	hash1 := desiredstate.ComposeHash(content)
	hash2 := desiredstate.ComposeHash(content)
	if hash1 != hash2 {
		t.Errorf("expected deterministic hash, got %q and %q", hash1, hash2)
	}
	if len(hash1) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(hash1))
	}
	different := desiredstate.ComposeHash([]byte("different content"))
	if hash1 == different {
		t.Error("different content should produce different hashes")
	}
}
