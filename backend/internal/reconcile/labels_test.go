package reconcile

import (
	"strings"
	"testing"
)

// --- generateLabelArgs tests ---

func TestGenerateLabelArgs_AllKeysPresent(t *testing.T) {
	m := generateLabelArgs("stacks/app", "abc123", "initial commit", "hash456")

	expectedKeys := []string{
		LabelStackPath,
		LabelDesiredRevision,
		LabelDesiredCommitMessage,
		LabelDesiredComposeHash,
		LabelSyncedAt,
		LabelSyncAt,
		LabelSyncStatus,
	}
	if len(m) != len(expectedKeys) {
		t.Fatalf("expected %d keys, got %d", len(expectedKeys), len(m))
	}
	for _, k := range expectedKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
}

func TestGenerateLabelArgs_ValuesMatch(t *testing.T) {
	m := generateLabelArgs("stacks/app", "abc123", "my commit", "hash789")

	if m[LabelStackPath] != "stacks/app" {
		t.Errorf("stack path = %q, want %q", m[LabelStackPath], "stacks/app")
	}
	if m[LabelDesiredRevision] != "abc123" {
		t.Errorf("revision = %q, want %q", m[LabelDesiredRevision], "abc123")
	}
	if m[LabelDesiredCommitMessage] != "my commit" {
		t.Errorf("commit message = %q, want %q", m[LabelDesiredCommitMessage], "my commit")
	}
	if m[LabelDesiredComposeHash] != "hash789" {
		t.Errorf("compose hash = %q, want %q", m[LabelDesiredComposeHash], "hash789")
	}
	if m[LabelSyncStatus] != "synced" {
		t.Errorf("sync status = %q, want %q", m[LabelSyncStatus], "synced")
	}
}

func TestGenerateLabelArgs_TimestampsNotEmpty(t *testing.T) {
	m := generateLabelArgs("p", "r", "c", "h")
	if m[LabelSyncedAt] == "" {
		t.Error("LabelSyncedAt is empty")
	}
	if m[LabelSyncAt] == "" {
		t.Error("LabelSyncAt is empty")
	}
}

// --- generateLabelOverride tests ---

func TestGenerateLabelOverride_EmptyServices(t *testing.T) {
	result := generateLabelOverride("path", "rev", "msg", "hash", nil)
	if result != "" {
		t.Errorf("expected empty string for no services, got %q", result)
	}
}

func TestGenerateLabelOverride_SingleService(t *testing.T) {
	result := generateLabelOverride("stacks/app", "abc", "msg", "hash", []string{"web"})

	if !strings.HasPrefix(result, "services:\n") {
		t.Errorf("expected to start with 'services:\\n', got %q", result[:20])
	}
	if !strings.Contains(result, "  web:\n") {
		t.Error("expected service 'web' in output")
	}
	if !strings.Contains(result, LabelStackPath) {
		t.Error("expected LabelStackPath in output")
	}
	if !strings.Contains(result, LabelDesiredRevision) {
		t.Error("expected LabelDesiredRevision in output")
	}
	if !strings.Contains(result, `"synced"`) {
		t.Error("expected sync status 'synced' in output")
	}
}

func TestGenerateLabelOverride_MultipleServices(t *testing.T) {
	result := generateLabelOverride("p", "r", "m", "h", []string{"web", "db", "cache"})

	for _, svc := range []string{"web", "db", "cache"} {
		if !strings.Contains(result, "  "+svc+":\n") {
			t.Errorf("expected service %q in output", svc)
		}
	}
}

func TestGenerateLabelOverride_EscapesQuotes(t *testing.T) {
	result := generateLabelOverride("p", "r", `commit with "quotes"`, "h", []string{"web"})

	if strings.Contains(result, `"quotes"`) && !strings.Contains(result, `\"quotes\"`) {
		t.Error("expected double quotes to be escaped")
	}
}

func TestGenerateLabelOverride_EscapesNewlines(t *testing.T) {
	result := generateLabelOverride("p", "r", "line1\nline2", "h", []string{"web"})

	if strings.Contains(result, "\nline2") {
		// The newline in the commit message should be replaced with space
		lines := strings.Split(result, "\n")
		for _, l := range lines {
			if strings.Contains(l, "line1") && strings.Contains(l, "line2") {
				return // good — both on same line
			}
		}
		t.Error("expected newline in commit message to be replaced with space")
	}
}

// --- AllLabelKeys tests ---

func TestAllLabelKeys_Count(t *testing.T) {
	keys := AllLabelKeys()
	if len(keys) != 8 {
		t.Errorf("expected 8 keys, got %d: %v", len(keys), keys)
	}
}

func TestAllLabelKeys_ContainsAll(t *testing.T) {
	keys := AllLabelKeys()
	expected := []string{
		LabelStackPath,
		LabelDesiredRevision,
		LabelDesiredCommitMessage,
		LabelDesiredComposeHash,
		LabelSyncedAt,
		LabelSyncAt,
		LabelSyncStatus,
		LabelSyncError,
	}
	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	for _, e := range expected {
		if !keySet[e] {
			t.Errorf("missing key %q", e)
		}
	}
}

func TestAllLabelKeys_NoDuplicates(t *testing.T) {
	keys := AllLabelKeys()
	seen := make(map[string]bool)
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate key %q", k)
		}
		seen[k] = true
	}
}
