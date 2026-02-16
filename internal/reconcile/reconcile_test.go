package reconcile_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
)

// --- Stubs ---

type stubComposeRunner struct {
	upCalls   []composeCall
	downCalls []composeCall
	upErr     error
	downErr   error
}

type composeCall struct {
	ProjectName  string
	ComposeFile  string
	OverrideFile string
	WorkDir      string
}

func (s *stubComposeRunner) ComposeUp(_ context.Context, projectName, composeFile, overrideFile, workDir string) error {
	s.upCalls = append(s.upCalls, composeCall{
		ProjectName:  projectName,
		ComposeFile:  composeFile,
		OverrideFile: overrideFile,
		WorkDir:      workDir,
	})
	return s.upErr
}

func (s *stubComposeRunner) ComposeDown(_ context.Context, projectName, composeFile, workDir string) error {
	s.downCalls = append(s.downCalls, composeCall{
		ProjectName: projectName,
		ComposeFile: composeFile,
		WorkDir:     workDir,
	})
	return s.downErr
}

type stubInspector struct {
	labels map[string]reconcile.StackSyncMetadata
	err    error
}

func (s *stubInspector) GetStackLabels(_ context.Context) (map[string]reconcile.StackSyncMetadata, error) {
	return s.labels, s.err
}

type dynamicInspector struct {
	labels map[string]reconcile.StackSyncMetadata
}

func (d *dynamicInspector) GetStackLabels(_ context.Context) (map[string]reconcile.StackSyncMetadata, error) {
	if d.labels == nil {
		return map[string]reconcile.StackSyncMetadata{}, nil
	}
	return d.labels, nil
}

// --- T011: Drift detection tests ---

func TestDetectDrift_NoRuntime_NeedsSync(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Revision: "rev1"})
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), nil, nil, reconcile.NewAckStore(), "")

	desired := []desiredstate.StackRecord{
		{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1"},
	}
	runtime := map[string]reconcile.StackSyncMetadata{}

	drifts := r.DetectDrift(desired, runtime)

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift result, got %d", len(drifts))
	}
	if !drifts[0].NeedSync {
		t.Error("expected NeedSync=true for missing runtime")
	}
	if drifts[0].Reason != "no runtime metadata found" {
		t.Errorf("unexpected reason: %s", drifts[0].Reason)
	}
}

func TestDetectDrift_MissingMetadata_NeedsSync(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Revision: "rev1"})
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), nil, nil, reconcile.NewAckStore(), "")

	desired := []desiredstate.StackRecord{
		{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1"},
	}
	runtime := map[string]reconcile.StackSyncMetadata{
		"app1": {StackPath: "app1", DesiredRevision: "", DesiredComposeHash: ""},
	}

	drifts := r.DetectDrift(desired, runtime)

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift result, got %d", len(drifts))
	}
	if !drifts[0].NeedSync {
		t.Error("expected NeedSync=true for missing metadata")
	}
}

func TestDetectDrift_RevisionDrift(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Revision: "new-rev"})
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), nil, nil, reconcile.NewAckStore(), "")

	desired := []desiredstate.StackRecord{
		{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1"},
	}
	runtime := map[string]reconcile.StackSyncMetadata{
		"app1": {StackPath: "app1", DesiredRevision: "old-rev", DesiredComposeHash: "hash1"},
	}

	drifts := r.DetectDrift(desired, runtime)

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift result, got %d", len(drifts))
	}
	if !drifts[0].NeedSync {
		t.Error("expected NeedSync=true for revision drift")
	}
}

func TestDetectDrift_ComposeHashDrift(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Revision: "rev1"})
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), nil, nil, reconcile.NewAckStore(), "")

	desired := []desiredstate.StackRecord{
		{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "new-hash"},
	}
	runtime := map[string]reconcile.StackSyncMetadata{
		"app1": {StackPath: "app1", DesiredRevision: "rev1", DesiredComposeHash: "old-hash"},
	}

	drifts := r.DetectDrift(desired, runtime)

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift result, got %d", len(drifts))
	}
	if !drifts[0].NeedSync {
		t.Error("expected NeedSync=true for compose hash drift")
	}
}

func TestDetectDrift_InSync(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Revision: "rev1"})
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), nil, nil, reconcile.NewAckStore(), "")

	desired := []desiredstate.StackRecord{
		{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1"},
	}
	runtime := map[string]reconcile.StackSyncMetadata{
		"app1": {StackPath: "app1", DesiredRevision: "rev1", DesiredComposeHash: "hash1"},
	}

	drifts := r.DetectDrift(desired, runtime)

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift result, got %d", len(drifts))
	}
	if drifts[0].NeedSync {
		t.Error("expected NeedSync=false for in-sync stack")
	}
	if drifts[0].Reason != "in sync" {
		t.Errorf("expected 'in sync' reason, got %q", drifts[0].Reason)
	}
}

func TestDetectDrift_RemovalDetected(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Revision: "rev1"})
	policy := reconcile.DefaultPolicy()
	policy.RemoveEnabled = true
	r := reconcile.NewReconciler(store, policy, nil, nil, reconcile.NewAckStore(), "")

	desired := []desiredstate.StackRecord{}
	runtime := map[string]reconcile.StackSyncMetadata{
		"old-app": {StackPath: "old-app", DesiredRevision: "rev1", DesiredComposeHash: "hash1"},
	}

	drifts := r.DetectDrift(desired, runtime)

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift result, got %d", len(drifts))
	}
	if !drifts[0].NeedRemove {
		t.Error("expected NeedRemove=true for stack not in desired state")
	}
}

func TestDetectDrift_RemovalDisabled(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Revision: "rev1"})
	policy := reconcile.DefaultPolicy()
	policy.RemoveEnabled = false
	r := reconcile.NewReconciler(store, policy, nil, nil, reconcile.NewAckStore(), "")

	desired := []desiredstate.StackRecord{}
	runtime := map[string]reconcile.StackSyncMetadata{
		"old-app": {StackPath: "old-app"},
	}

	drifts := r.DetectDrift(desired, runtime)

	if len(drifts) != 0 {
		t.Errorf("expected 0 drift results with removal disabled, got %d", len(drifts))
	}
}

// --- T012/T030: Reconcile cycle tests ---

func TestReconcile_DriftedStack_Synced(t *testing.T) {
	store := desiredstate.NewStore()
	composeContent := []byte("services:\n  web:\n    image: nginx\n")
	store.Set(&desiredstate.Snapshot{
		Revision:      "rev1",
		CommitMessage: "deploy v1",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Status: desiredstate.StackSyncMissing, Content: composeContent},
		},
	})

	compose := &stubComposeRunner{}
	inspector := &stubInspector{labels: map[string]reconcile.StackSyncMetadata{}}

	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), compose, inspector, reconcile.NewAckStore(), "")
	runs := r.Reconcile(context.Background())

	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Result != "success" {
		t.Errorf("expected success, got %q (error: %s)", runs[0].Result, runs[0].Error)
	}
	if len(compose.upCalls) != 1 {
		t.Fatalf("expected 1 compose up call, got %d", len(compose.upCalls))
	}
	if compose.upCalls[0].ProjectName != "app1" {
		t.Errorf("expected project app1, got %q", compose.upCalls[0].ProjectName)
	}

	// Verify stack is now synced
	snap := store.Get()
	if snap.Stacks[0].Status != desiredstate.StackSyncSynced {
		t.Errorf("expected synced status, got %q", snap.Stacks[0].Status)
	}
	if snap.Stacks[0].SyncedRevision != "rev1" {
		t.Errorf("expected synced revision rev1, got %q", snap.Stacks[0].SyncedRevision)
	}
	if snap.Stacks[0].SyncedComposeHash != "hash1" {
		t.Errorf("expected synced hash hash1, got %q", snap.Stacks[0].SyncedComposeHash)
	}
	if snap.Stacks[0].SyncedCommitMessage != "deploy v1" {
		t.Errorf("expected commit message, got %q", snap.Stacks[0].SyncedCommitMessage)
	}
}

func TestReconcile_AlreadySynced_NoOp(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "rev1",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Status: desiredstate.StackSyncSynced},
		},
	})

	compose := &stubComposeRunner{}
	inspector := &stubInspector{
		labels: map[string]reconcile.StackSyncMetadata{
			"app1": {StackPath: "app1", DesiredRevision: "rev1", DesiredComposeHash: "hash1"},
		},
	}

	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), compose, inspector, reconcile.NewAckStore(), "")
	runs := r.Reconcile(context.Background())

	if len(runs) != 0 {
		t.Errorf("expected 0 runs for in-sync stack, got %d", len(runs))
	}
	if len(compose.upCalls) != 0 {
		t.Errorf("expected 0 compose up calls, got %d", len(compose.upCalls))
	}
}

func TestReconcile_Disabled(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision: "rev1",
		Stacks:   []desiredstate.StackRecord{{Path: "app1"}},
	})

	policy := reconcile.DefaultPolicy()
	policy.Enabled = false

	compose := &stubComposeRunner{}
	inspector := &stubInspector{labels: map[string]reconcile.StackSyncMetadata{}}

	r := reconcile.NewReconciler(store, policy, compose, inspector, reconcile.NewAckStore(), "")
	runs := r.Reconcile(context.Background())

	if runs != nil {
		t.Errorf("expected nil runs when disabled, got %v", runs)
	}
}

func TestReconcile_ComposeUpFailure(t *testing.T) {
	store := desiredstate.NewStore()
	composeContent := []byte("services:\n  web:\n    image: nginx\n")
	store.Set(&desiredstate.Snapshot{
		Revision: "rev1",
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Status: desiredstate.StackSyncMissing, Content: composeContent},
		},
	})

	compose := &stubComposeRunner{upErr: fmt.Errorf("image not found")}
	inspector := &stubInspector{labels: map[string]reconcile.StackSyncMetadata{}}

	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), compose, inspector, reconcile.NewAckStore(), "")
	runs := r.Reconcile(context.Background())

	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Result != "failed" {
		t.Errorf("expected failed result, got %q", runs[0].Result)
	}

	// FR-012: desired state cache should be preserved on failure
	snap := store.Get()
	if snap == nil {
		t.Fatal("snapshot should be preserved after failure")
	}
	if len(snap.Stacks) != 1 {
		t.Fatal("stacks should be preserved after failure")
	}
	if snap.Stacks[0].Status != desiredstate.StackSyncFailed {
		t.Errorf("expected failed status, got %q", snap.Stacks[0].Status)
	}
	if snap.Stacks[0].LastSyncError == "" {
		t.Error("expected error message on failed stack")
	}
}

// --- T030: Drift policy tests ---

func TestReconcile_DriftPolicy_Flag_NoAck(t *testing.T) {
	store := desiredstate.NewStore()
	composeContent := []byte("services:\n  web:\n    image: nginx\n")
	store.Set(&desiredstate.Snapshot{
		Revision: "rev1",
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Status: desiredstate.StackSyncMissing, Content: composeContent},
		},
	})

	policy := reconcile.DefaultPolicy()
	policy.DriftPolicy = "flag"

	compose := &stubComposeRunner{}
	inspector := &stubInspector{labels: map[string]reconcile.StackSyncMetadata{}}

	r := reconcile.NewReconciler(store, policy, compose, inspector, reconcile.NewAckStore(), "")
	runs := r.Reconcile(context.Background())

	// No runs should execute without acknowledgement
	if len(runs) != 0 {
		t.Errorf("expected 0 runs without ack, got %d", len(runs))
	}
	if len(compose.upCalls) != 0 {
		t.Errorf("expected 0 compose up calls, got %d", len(compose.upCalls))
	}
}

func TestReconcile_DriftPolicy_Flag_WithAck(t *testing.T) {
	store := desiredstate.NewStore()
	composeContent := []byte("services:\n  web:\n    image: nginx\n")
	store.Set(&desiredstate.Snapshot{
		Revision: "rev1",
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Status: desiredstate.StackSyncMissing, Content: composeContent},
		},
	})

	policy := reconcile.DefaultPolicy()
	policy.DriftPolicy = "flag"

	compose := &stubComposeRunner{}
	inspector := &stubInspector{labels: map[string]reconcile.StackSyncMetadata{}}
	ackStore := reconcile.NewAckStore()
	ackStore.Acknowledge("app1")

	r := reconcile.NewReconciler(store, policy, compose, inspector, ackStore, "")
	runs := r.Reconcile(context.Background())

	if len(runs) != 1 {
		t.Fatalf("expected 1 run with ack, got %d", len(runs))
	}
	if runs[0].Result != "success" {
		t.Errorf("expected success, got %q", runs[0].Result)
	}

	// Ack should be cleared after use
	if ackStore.IsAcknowledged("app1") {
		t.Error("expected ack to be cleared after use")
	}
}

func TestReconcile_DriftPolicy_Revert(t *testing.T) {
	store := desiredstate.NewStore()
	composeContent := []byte("services:\n  web:\n    image: nginx\n")
	store.Set(&desiredstate.Snapshot{
		Revision: "rev1",
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Status: desiredstate.StackSyncMissing, Content: composeContent},
		},
	})

	compose := &stubComposeRunner{}
	inspector := &stubInspector{labels: map[string]reconcile.StackSyncMetadata{}}

	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), compose, inspector, reconcile.NewAckStore(), "")
	runs := r.Reconcile(context.Background())

	// Revert mode should auto-reconcile without ack
	if len(runs) != 1 {
		t.Fatalf("expected 1 run with revert policy, got %d", len(runs))
	}
	if runs[0].Result != "success" {
		t.Errorf("expected success, got %q", runs[0].Result)
	}
}

// --- T031: Concurrency and cache preservation tests ---

func TestReconcile_ConcurrencyMutex(t *testing.T) {
	store := desiredstate.NewStore()
	composeContent := []byte("services:\n  web:\n    image: nginx\n")
	store.Set(&desiredstate.Snapshot{
		Revision: "rev1",
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Status: desiredstate.StackSyncMissing, Content: composeContent},
		},
	})

	compose := &stubComposeRunner{}
	// Inspector that updates after first sync (simulating containers getting labels)
	inspector := &dynamicInspector{}

	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), compose, inspector, reconcile.NewAckStore(), "")

	// Run reconcile — first should sync
	runs1 := r.Reconcile(context.Background())
	if len(runs1) != 1 {
		t.Errorf("first reconcile: expected 1 run, got %d", len(runs1))
	}

	// Now update inspector to reflect synced state
	inspector.labels = map[string]reconcile.StackSyncMetadata{
		"app1": {StackPath: "app1", DesiredRevision: "rev1", DesiredComposeHash: "hash1"},
	}

	// Second run should be no-op since runtime now matches desired
	runs2 := r.Reconcile(context.Background())
	if len(runs2) != 0 {
		t.Errorf("second reconcile should be no-op, got %d runs", len(runs2))
	}
}

func TestReconcile_CachePreservedOnFailure(t *testing.T) {
	store := desiredstate.NewStore()
	composeContent := []byte("services:\n  web:\n    image: nginx\n")
	store.Set(&desiredstate.Snapshot{
		Revision:      "rev1",
		CommitMessage: "test commit",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Content: composeContent},
			{Path: "app2", ComposeFile: "docker-compose.yml", ComposeHash: "hash2", Content: composeContent},
		},
	})

	compose := &stubComposeRunner{upErr: fmt.Errorf("deploy failed")}
	inspector := &stubInspector{labels: map[string]reconcile.StackSyncMetadata{}}

	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), compose, inspector, reconcile.NewAckStore(), "")
	r.Reconcile(context.Background())

	// Desired state cache should be intact
	snap := store.Get()
	if snap == nil {
		t.Fatal("snapshot should not be nil after failure")
	}
	if len(snap.Stacks) != 2 {
		t.Errorf("expected 2 stacks preserved, got %d", len(snap.Stacks))
	}
	if snap.Revision != "rev1" {
		t.Errorf("expected revision preserved, got %q", snap.Revision)
	}
}

// --- AckStore tests ---

func TestAckStore_AcknowledgeAndCheck(t *testing.T) {
	store := reconcile.NewAckStore()

	if store.IsAcknowledged("app1") {
		t.Error("expected not acknowledged initially")
	}

	store.Acknowledge("app1")
	if !store.IsAcknowledged("app1") {
		t.Error("expected acknowledged after Acknowledge")
	}

	store.Clear("app1")
	if store.IsAcknowledged("app1") {
		t.Error("expected not acknowledged after Clear")
	}
}

// --- ReconciliationRun timestamps ---

func TestReconciliationRun_Timestamps(t *testing.T) {
	store := desiredstate.NewStore()
	composeContent := []byte("services:\n  web:\n    image: nginx\n")
	store.Set(&desiredstate.Snapshot{
		Revision: "rev1",
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Content: composeContent},
		},
	})

	compose := &stubComposeRunner{}
	inspector := &stubInspector{labels: map[string]reconcile.StackSyncMetadata{}}

	before := time.Now()
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), compose, inspector, reconcile.NewAckStore(), "")
	runs := r.Reconcile(context.Background())
	after := time.Now()

	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].StartedAt.Before(before) || runs[0].StartedAt.After(after) {
		t.Error("StartedAt should be within test execution window")
	}
	if runs[0].FinishedAt.Before(runs[0].StartedAt) {
		t.Error("FinishedAt should be after StartedAt")
	}
}

// --- Multiple stacks ---

func TestReconcile_MultipleStacks_Sequential(t *testing.T) {
	store := desiredstate.NewStore()
	composeContent := []byte("services:\n  web:\n    image: nginx\n")
	store.Set(&desiredstate.Snapshot{
		Revision: "rev1",
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "h1", Content: composeContent},
			{Path: "app2", ComposeFile: "docker-compose.yml", ComposeHash: "h2", Content: composeContent},
			{Path: "app3", ComposeFile: "docker-compose.yml", ComposeHash: "h3", Content: composeContent},
		},
	})

	compose := &stubComposeRunner{}
	inspector := &stubInspector{labels: map[string]reconcile.StackSyncMetadata{}}

	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), compose, inspector, reconcile.NewAckStore(), "")
	runs := r.Reconcile(context.Background())

	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
	if len(compose.upCalls) != 3 {
		t.Fatalf("expected 3 compose up calls, got %d", len(compose.upCalls))
	}

	// All should succeed
	for i, run := range runs {
		if run.Result != "success" {
			t.Errorf("run %d: expected success, got %q", i, run.Result)
		}
	}
}

// --- Removal tests ---

func TestReconcile_RemoveStack(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision: "rev1",
		Stacks:   []desiredstate.StackRecord{},
	})

	policy := reconcile.DefaultPolicy()
	policy.RemoveEnabled = true

	compose := &stubComposeRunner{}
	inspector := &stubInspector{
		labels: map[string]reconcile.StackSyncMetadata{
			"old-app": {StackPath: "old-app", DesiredRevision: "rev0", DesiredComposeHash: "oldhash"},
		},
	}

	r := reconcile.NewReconciler(store, policy, compose, inspector, reconcile.NewAckStore(), "")
	runs := r.Reconcile(context.Background())

	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Result != "success" {
		t.Errorf("expected success, got %q (error: %s)", runs[0].Result, runs[0].Error)
	}
	if len(compose.downCalls) != 1 {
		t.Fatalf("expected 1 compose down call, got %d", len(compose.downCalls))
	}
}

func TestReconcile_RemoveDisabled_Skips(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision: "rev1",
		Stacks:   []desiredstate.StackRecord{},
	})

	policy := reconcile.DefaultPolicy()
	policy.RemoveEnabled = false

	compose := &stubComposeRunner{}
	inspector := &stubInspector{
		labels: map[string]reconcile.StackSyncMetadata{
			"old-app": {StackPath: "old-app"},
		},
	}

	r := reconcile.NewReconciler(store, policy, compose, inspector, reconcile.NewAckStore(), "")
	runs := r.Reconcile(context.Background())

	if len(runs) != 0 {
		t.Errorf("expected 0 runs when removal disabled, got %d", len(runs))
	}
	if len(compose.downCalls) != 0 {
		t.Errorf("expected 0 compose down calls, got %d", len(compose.downCalls))
	}
}

// TestReconcile_NoOp_MultipleStacksInSync is a regression test that verifies
// no compose operations occur when multiple stacks are all in sync.
func TestReconcile_NoOp_MultipleStacksInSync(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "rev1",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "app1", ComposeFile: "docker-compose.yml", ComposeHash: "hash1", Status: desiredstate.StackSyncSynced},
			{Path: "app2", ComposeFile: "docker-compose.yml", ComposeHash: "hash2", Status: desiredstate.StackSyncSynced},
			{Path: "app3", ComposeFile: "docker-compose.yml", ComposeHash: "hash3", Status: desiredstate.StackSyncSynced},
		},
	})

	compose := &stubComposeRunner{}
	inspector := &stubInspector{
		labels: map[string]reconcile.StackSyncMetadata{
			"app1": {StackPath: "app1", DesiredRevision: "rev1", DesiredComposeHash: "hash1"},
			"app2": {StackPath: "app2", DesiredRevision: "rev1", DesiredComposeHash: "hash2"},
			"app3": {StackPath: "app3", DesiredRevision: "rev1", DesiredComposeHash: "hash3"},
		},
	}

	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), compose, inspector, reconcile.NewAckStore(), "")
	runs := r.Reconcile(context.Background())

	if len(runs) != 0 {
		t.Errorf("expected 0 runs for all in-sync stacks, got %d", len(runs))
	}
	if len(compose.upCalls) != 0 {
		t.Errorf("expected 0 compose up calls, got %d", len(compose.upCalls))
	}
	if len(compose.downCalls) != 0 {
		t.Errorf("expected 0 compose down calls, got %d", len(compose.downCalls))
	}
}
