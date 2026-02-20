package reconcile_test

import (
	"testing"

	"github.com/lucasreiners/docker-cd/internal/reconcile"
)

func TestMapLabelsToMetadata_Complete(t *testing.T) {
	labels := map[string]string{
		"com.docker-cd.stack.path":             "app1",
		"com.docker-cd.desired.revision":       "abc123",
		"com.docker-cd.desired.commit_message": "deploy v1",
		"com.docker-cd.desired.compose_hash":   "hash1",
		"com.docker-cd.synced.at":              "2024-01-01T00:00:00Z",
		"com.docker-cd.sync.at":                "2024-01-01T00:00:00Z",
		"com.docker-cd.sync.status":            "synced",
		"com.docker-cd.sync.error":             "",
	}

	meta := reconcile.MapLabelsToMetadata(labels)

	if meta.StackPath != "app1" {
		t.Errorf("StackPath: got %q, want %q", meta.StackPath, "app1")
	}
	if meta.DesiredRevision != "abc123" {
		t.Errorf("DesiredRevision: got %q, want %q", meta.DesiredRevision, "abc123")
	}
	if meta.DesiredCommitMessage != "deploy v1" {
		t.Errorf("DesiredCommitMessage: got %q, want %q", meta.DesiredCommitMessage, "deploy v1")
	}
	if meta.DesiredComposeHash != "hash1" {
		t.Errorf("DesiredComposeHash: got %q, want %q", meta.DesiredComposeHash, "hash1")
	}
	if meta.SyncedAt != "2024-01-01T00:00:00Z" {
		t.Errorf("SyncedAt: got %q, want %q", meta.SyncedAt, "2024-01-01T00:00:00Z")
	}
	if meta.SyncStatus != "synced" {
		t.Errorf("SyncStatus: got %q, want %q", meta.SyncStatus, "synced")
	}
}

func TestMapLabelsToMetadata_Empty(t *testing.T) {
	meta := reconcile.MapLabelsToMetadata(map[string]string{})

	if meta.StackPath != "" {
		t.Errorf("StackPath: got %q, want empty", meta.StackPath)
	}
	if meta.DesiredRevision != "" {
		t.Errorf("DesiredRevision: got %q, want empty", meta.DesiredRevision)
	}
}

func TestMapLabelsToMetadata_GroupsByStack(t *testing.T) {
	containers := []map[string]string{
		{
			"com.docker-cd.stack.path":           "app1",
			"com.docker-cd.desired.revision":     "rev1",
			"com.docker-cd.desired.compose_hash": "hash1",
		},
		{
			"com.docker-cd.stack.path":           "app1",
			"com.docker-cd.desired.revision":     "rev1",
			"com.docker-cd.desired.compose_hash": "hash1",
		},
		{
			"com.docker-cd.stack.path":           "app2",
			"com.docker-cd.desired.revision":     "rev1",
			"com.docker-cd.desired.compose_hash": "hash2",
		},
	}

	result := make(map[string]reconcile.StackSyncMetadata)
	for _, labels := range containers {
		sp := labels["com.docker-cd.stack.path"]
		if sp == "" {
			continue
		}
		if _, exists := result[sp]; exists {
			continue
		}
		result[sp] = reconcile.MapLabelsToMetadata(labels)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 stacks, got %d", len(result))
	}
	if result["app1"].DesiredRevision != "rev1" {
		t.Errorf("app1 revision: got %q, want %q", result["app1"].DesiredRevision, "rev1")
	}
	if result["app2"].DesiredComposeHash != "hash2" {
		t.Errorf("app2 hash: got %q, want %q", result["app2"].DesiredComposeHash, "hash2")
	}
}
