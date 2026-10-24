package desiredstate

import (
	"testing"
)

func TestRuntimeMetadataRevisionAndPayloadIsolation(t *testing.T) {
	s := NewStore()
	s.Set(&Snapshot{Stacks: []StackRecord{{Path: "web", ComposeHash: "one", Content: make([]byte, 1024*1024)}}})
	revision, stacks, ok := s.RuntimeStacks(0, 10, 100, 100)
	if !ok || len(stacks) != 1 {
		t.Fatal("missing metadata")
	}
	old := stacks[0].Signature
	stacks[0].Path = "mutated"
	if _, got, _ := s.RuntimeStacks(0, 10, 100, 100); got[0].Path != "web" {
		t.Fatal("mutable scope escaped")
	}
	allocations := testing.AllocsPerRun(100, func() {
		if rev, got, ok := s.RuntimeStacks(revision, 10, 100, 100); rev != revision || got != nil || !ok {
			t.Fatal("unchanged metadata recopied")
		}
	})
	if allocations != 0 {
		t.Fatal("revision check allocated", allocations)
	}
	s.Set(&Snapshot{Stacks: []StackRecord{{Path: "web", ComposeHash: "one", Content: []byte("different payload; same declared hash")}}})
	next, got, _ := s.RuntimeStacks(revision, 10, 100, 100)
	if next == revision || got[0].Signature != old {
		t.Fatal("signature depends on Compose payload instead of declared metadata")
	}
	s.Set(&Snapshot{Stacks: []StackRecord{{Path: "web", ComposeHash: "two"}}})
	_, got, _ = s.RuntimeStacks(next, 10, 100, 100)
	if got[0].Signature == old {
		t.Fatal("metadata change missed")
	}
	if _, got, ok := s.RuntimeStacks(0, 0, 100, 100); ok || got != nil {
		t.Fatal("oversized scope copied before rejection")
	}
}
