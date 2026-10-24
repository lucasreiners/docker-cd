package operations

import (
	"strings"
	"testing"
)

func TestRuntimeTerminalMetadataGenerations(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	scopes := map[string]struct{}{"web": {}, "": {}}
	revision, terminals := s.RuntimeTerminals(0, scopes)
	if len(terminals) != 0 {
		t.Fatal("empty history has terminals")
	}
	r := s.Start("", "deployment", "test", "web", "")
	s.Update(r.ID, "pulling", strings.Repeat("x", 1024*1024), false)
	if next, got := s.RuntimeTerminals(revision, scopes); next != revision || got != nil {
		t.Fatal("running step changed terminal generation")
	}
	s.Update(r.ID, "completed", "done", true)
	next, got := s.RuntimeTerminals(revision, scopes)
	if next == revision || got["web"] == 0 {
		t.Fatal("terminal missed")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if _, got := s.RuntimeTerminals(next, scopes); got != nil {
			t.Fatal("unchanged terminal metadata recopied")
		}
	}); allocs != 0 {
		t.Fatal("terminal fast path allocated", allocs)
	}
	global := s.Start("", "removal", "test", "", "")
	s.Update(global.ID, "failed", "done", true)
	final, got := s.RuntimeTerminals(next, scopes)
	if final == next || got[""] <= got["web"] {
		t.Fatal("global removal generation missing")
	}
	unknown := s.Start("", "deployment", "test", "not-discovered", "")
	s.Update(unknown.ID, "completed", "done", true)
	_, got = s.RuntimeTerminals(final, scopes)
	if _, exists := got["not-discovered"]; exists {
		t.Fatal("unrelated terminal scope retained")
	}
}
