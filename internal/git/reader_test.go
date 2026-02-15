package git_test

import (
	"context"
	"testing"

	"github.com/lucasreiners/docker-cd/internal/git"
)

type stubComposeReader struct {
	entries []git.ComposeEntry
	commit  string
	err     error
}

func (s *stubComposeReader) ReadComposeFiles(_ context.Context, _, _, _, _ string) ([]git.ComposeEntry, string, error) {
	return s.entries, s.commit, s.err
}

func TestStubComposeReader_ReturnsEntries(t *testing.T) {
	reader := &stubComposeReader{
		entries: []git.ComposeEntry{
			{StackPath: "app1", ComposeFile: "docker-compose.yml", Content: []byte("version: '3'")},
			{StackPath: "app2", ComposeFile: "docker-compose.yaml", Content: []byte("version: '3.8'")},
		},
		commit: "abc123def456",
	}
	entries, commit, err := reader.ReadComposeFiles(context.Background(), "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit != "abc123def456" {
		t.Errorf("expected commit abc123def456, got %q", commit)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].StackPath != "app1" {
		t.Errorf("expected app1, got %q", entries[0].StackPath)
	}
}

func TestStubComposeReader_ReturnsError(t *testing.T) {
	reader := &stubComposeReader{err: context.DeadlineExceeded}
	_, _, err := reader.ReadComposeFiles(context.Background(), "", "", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStubComposeReader_EmptyRepo(t *testing.T) {
	reader := &stubComposeReader{entries: nil, commit: "abc123"}
	entries, commit, err := reader.ReadComposeFiles(context.Background(), "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit != "abc123" {
		t.Errorf("expected abc123, got %q", commit)
	}
	if entries != nil {
		t.Errorf("expected nil entries for empty repo, got %v", entries)
	}
}
