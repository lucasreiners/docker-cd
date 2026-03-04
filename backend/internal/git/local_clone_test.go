package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestLocalClone_ReadComposeFiles_CloneAndRefresh(t *testing.T) {
	remoteDir := t.TempDir()
	revision, firstHash := initTestRepo(t, remoteDir, "stack-a", "docker-compose.yml", "services:\n  web:\n    image: nginx:alpine\n")

	localDir := t.TempDir()
	clone := NewLocalClone(filepath.Join(localDir, "repo"), remoteDir, "", revision)
	reader := NewLocalComposeReader(clone)

	entries, commitHash, _, err := reader.ReadComposeFiles(context.Background(), "", "", "", "")
	if err != nil {
		t.Fatalf("ReadComposeFiles failed: %v", err)
	}
	if commitHash != firstHash {
		t.Fatalf("expected commit hash %s, got %s", firstHash, commitHash)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].StackPath != "stack-a" {
		t.Fatalf("expected stack path stack-a, got %s", entries[0].StackPath)
	}

	secondHash := addCommit(t, remoteDir, "stack-a", "docker-compose.yml", "services:\n  web:\n    image: nginx:1.25-alpine\n")

	entries, commitHash, _, err = reader.ReadComposeFiles(context.Background(), "", "", "", "")
	if err != nil {
		t.Fatalf("ReadComposeFiles refresh failed: %v", err)
	}
	if commitHash != secondHash {
		t.Fatalf("expected updated hash %s, got %s", secondHash, commitHash)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after refresh, got %d", len(entries))
	}
}

func TestLocalClone_RecloneOnMissingRepo(t *testing.T) {
	remoteDir := t.TempDir()
	revision, _ := initTestRepo(t, remoteDir, "stack-a", "docker-compose.yml", "services:\n  web:\n    image: nginx:alpine\n")

	localDir := filepath.Join(t.TempDir(), "repo")
	clone := NewLocalClone(localDir, remoteDir, "", revision)
	reader := NewLocalComposeReader(clone)

	if _, _, _, err := reader.ReadComposeFiles(context.Background(), "", "", "", ""); err != nil {
		t.Fatalf("initial ReadComposeFiles failed: %v", err)
	}

	if err := os.RemoveAll(localDir); err != nil {
		t.Fatalf("failed to remove local clone: %v", err)
	}

	if _, _, _, err := reader.ReadComposeFiles(context.Background(), "", "", "", ""); err != nil {
		t.Fatalf("reclone ReadComposeFiles failed: %v", err)
	}
}

func TestLocalClone_RecloneOnCorruptRepo(t *testing.T) {
	remoteDir := t.TempDir()
	revision, _ := initTestRepo(t, remoteDir, "stack-a", "docker-compose.yml", "services:\n  web:\n    image: nginx:alpine\n")

	localDir := filepath.Join(t.TempDir(), "repo")
	clone := NewLocalClone(localDir, remoteDir, "", revision)
	reader := NewLocalComposeReader(clone)

	if _, _, _, err := reader.ReadComposeFiles(context.Background(), "", "", "", ""); err != nil {
		t.Fatalf("initial ReadComposeFiles failed: %v", err)
	}

	if err := os.RemoveAll(localDir); err != nil {
		t.Fatalf("failed to remove local clone: %v", err)
	}
	if err := os.WriteFile(localDir, []byte("corrupt"), 0644); err != nil {
		t.Fatalf("failed to corrupt local clone: %v", err)
	}

	if _, _, _, err := reader.ReadComposeFiles(context.Background(), "", "", "", ""); err != nil {
		t.Fatalf("reclone after corruption failed: %v", err)
	}
}

func initTestRepo(t *testing.T, dir, stack, composeFile, content string) (string, string) {
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit failed: %v", err)
	}

	revision := "master"
	if err := writeComposeFile(dir, stack, composeFile, content); err != nil {
		t.Fatalf("writeComposeFile failed: %v", err)
	}

	hash := commitAll(t, repo, "initial")
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName(revision), hash)); err != nil {
		t.Fatalf("set reference failed: %v", err)
	}

	return revision, hash.String()
}

func addCommit(t *testing.T, repoPath, stack, composeFile, content string) string {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		t.Fatalf("PlainOpen failed: %v", err)
	}
	if err := writeComposeFile(repoPath, stack, composeFile, content); err != nil {
		t.Fatalf("writeComposeFile failed: %v", err)
	}

	hash := commitAll(t, repo, "update")
	return hash.String()
}

func writeComposeFile(repoPath, stack, composeFile, content string) error {
	stackDir := filepath.Join(repoPath, stack)
	if err := os.MkdirAll(stackDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stackDir, composeFile), []byte(content), 0644)
}

func commitAll(t *testing.T, repo *gogit.Repository, message string) plumbing.Hash {
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree failed: %v", err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	hash, err := wt.Commit(message, &gogit.CommitOptions{Author: testAuthor()})
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	return hash
}

func testAuthor() *object.Signature {
	return &object.Signature{
		Name:  "Test",
		Email: "test@example.com",
		When:  time.Now(),
	}
}
