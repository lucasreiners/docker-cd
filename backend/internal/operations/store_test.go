package operations

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestHistorySurvivesRestartAndInterruptsUnfinishedWork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	parent := store.Start("", "refresh", "webhook", "", "main")
	store.Update(parent.ID, "completed", "Checked Git", true)
	child := store.Start(parent.ID, "deployment", "webhook", "nested/apps/web", "abc123")
	store.Update(child.ID, "pulling", "Pulling image", false)
	store.SaveSource("nested/apps/web", "abc123", "hash", "services: {}")
	restarted, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	saved, ok := restarted.Get(child.ID)
	if !ok || saved.Status != "interrupted" || saved.ParentID != parent.ID || saved.FinishedAt == nil {
		t.Fatalf("bad recovered operation: %+v", saved)
	}
	if p, _ := restarted.Get(parent.ID); p.Status != "completed" {
		t.Fatalf("completed result changed: %+v", p)
	}
	if source, ok := restarted.AppliedSource("nested/apps/web"); !ok || source.Content != "services: {}" || source.Revision != "abc123" {
		t.Fatalf("applied source lost: %+v", source)
	}
	saved.Steps[0].Message = "mutated"
	if original, _ := restarted.Get(child.ID); original.Steps[0].Message == "mutated" {
		t.Fatal("caller mutated store")
	}
}

func TestHistoryRetentionPreservesActiveOperations(t *testing.T) {
	store, _ := Open("")
	active := store.Start("", "deployment", "manual", "active", "")
	for i := 0; i < 1002; i++ {
		r := store.Start("", "refresh", "poll", "", "")
		store.Update(r.ID, "completed", "done", true)
	}
	if len(store.List()) != 1001 {
		t.Fatalf("retention count: %d", len(store.List()))
	}
	if _, ok := store.Get(active.ID); !ok {
		t.Fatal("active operation evicted")
	}
}

func TestRetentionPreservesCompletedAncestryAndTerminalImmutability(t *testing.T) {
	store, _ := Open("")
	root := store.Start("", "refresh", "manual", "", "")
	store.Update(root.ID, "completed", "Checked Git", true)
	parent := store.Start(root.ID, "action", "retry", "web", "")
	store.Update(parent.ID, "completed", "Accepted", true)
	child := store.Start(parent.ID, "deployment", "manual", "web", "")
	for i := 0; i < 1005; i++ {
		r := store.Start("", "refresh", "periodic", "", "")
		store.Update(r.ID, "completed", "done", true)
	}
	for _, id := range []string{root.ID, parent.ID, child.ID} {
		if _, ok := store.Get(id); !ok {
			t.Fatalf("active ancestor evicted: %s", id)
		}
	}
	before, _ := store.Get(root.ID)
	if _, changed := store.Update(root.ID, "failed", "late outcome", true); changed {
		t.Fatal("terminal operation changed outcome")
	}
	after, _ := store.Get(root.ID)
	if after.Version != before.Version || after.Status != "completed" {
		t.Fatal("terminal record changed")
	}
	store.Update(child.ID, "completed", "done", true)
	if _, ok := store.Get(root.ID); ok {
		t.Fatal("unneeded old ancestor leaked beyond retention")
	}
}

func TestConcurrentUpdatesAndStorageFailureAreObservable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data", "history.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := store.Start("", "deployment", "manual", "app", "")
			store.Update(r.ID, "completed", "done", true)
		}()
	}
	wg.Wait()
	if len(store.List()) != 8 {
		t.Fatal("lost concurrent operation")
	}
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	store.Start("", "refresh", "manual", "", "")
	if persistent, message := store.Health(); !persistent || message == "" {
		t.Fatal("storage failure not reported")
	}
}
