//go:build integration

package integration_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/refresh"
	"github.com/lucasreiners/docker-cd/tests/integration/harness"
)

func TestSlowPullLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	h, err := harness.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	server := httptest.NewServer(h.Router)
	defer server.Close()
	stream := observeWork(t, ctx, server.URL)
	defer stream.Close()
	eventually(t, "initial authoritative snapshot", func() bool { return len(stream.snapshots()) == 1 })
	if snapshot := stream.snapshots()[0]; snapshot.SchemaVersion != 2 || snapshot.Epoch == "" || len(snapshot.Runs) != 0 {
		t.Fatalf("initial snapshot: %+v", snapshot)
	}
	h.SlowPull.Arm(false)
	h.Gate.Arm()
	defer h.Gate.Release()
	h.Start()
	eventually(t, "slow pull starts", h.SlowPull.Active)
	var initialRun desiredstate.WorkRun
	eventually(t, "initial reconcile byte events", func() bool {
		for _, run := range stream.runs() {
			if run.Source == "reconcile" && hasControlledLayers(run) {
				initialRun = run
				return true
			}
		}
		return false
	})
	if initialRun.Status != "running" || initialRun.Phase != "pulling" || initialRun.RootOperationID == "" || initialRun.OperationID == "" {
		t.Fatalf("uncorrelated pull: %+v", initialRun)
	}
	assertSyncing := func() {
		t.Helper()
		for _, s := range h.Store.GetStacks() {
			if s.Path == "apps/web" && (s.Status != desiredstate.StackSyncSyncing || s.LastSyncError != "") {
				t.Fatalf("long pull incorrectly failed: %+v", s)
			}
		}
	}
	// Repeated scheduled refreshes must not kill an in-flight image pull.
	for range 3 {
		h.Refresh.RequestRefresh(refresh.TriggerPeriodic)
		time.Sleep(600 * time.Millisecond)
		assertSyncing()
	}
	for _, phase := range []string{"Downloading", "Extracting", "Extracting"} {
		h.SlowPull.Advance()
		time.Sleep(600 * time.Millisecond)
		assertSyncing()
		sub, initial := h.Broadcaster.SubscribeWithSnapshot(h.Store)
		h.Broadcaster.Unsubscribe(sub)
		var snapshot struct {
			Progress []struct {
				LayerID string `json:"layerId"`
				Status  string `json:"status"`
			} `json:"progress"`
		}
		if err := json.Unmarshal([]byte(initial[2].Data), &snapshot); err != nil {
			t.Fatal(err)
		}
		layers := map[string]string{}
		for _, p := range snapshot.Progress {
			if p.LayerID != "" {
				layers[p.LayerID] = p.Status
			}
		}
		if len(layers) != 2 {
			t.Fatalf("reconnect lost layers: %s", initial[2].Data)
		}
		if layers["layer-a"] != phase && layers["layer-b"] != phase {
			t.Fatalf("missing %s phase: %v", phase, layers)
		}
	}
	// A fresh HTTP subscription must restore the exact run/image attempt and
	// current layers from the production snapshot barrier, not history or UI state.
	reconnected := observeWork(t, ctx, server.URL)
	eventually(t, "mid-pull work snapshot", func() bool { return len(reconnected.snapshots()) == 1 })
	snapshot := reconnected.snapshots()[0]
	if snapshot.Epoch != stream.snapshots()[0].Epoch {
		t.Fatal("reconnect changed server epoch")
	}
	var restored *desiredstate.WorkRun
	for i := range snapshot.Runs {
		if snapshot.Runs[i].RunID == initialRun.RunID {
			restored = &snapshot.Runs[i]
		}
	}
	if restored == nil || len(restored.Images) != 1 || len(restored.Images[0].Layers) != 2 || restored.Images[0].PullAttemptID != initialRun.Images[0].PullAttemptID || restored.OperationID != initialRun.OperationID {
		t.Fatalf("reconnect failed to restore identified layers: %+v", snapshot)
	}
	for _, layer := range restored.Images[0].Layers {
		if layer.ID != "layer-a" && layer.ID != "layer-b" {
			t.Fatalf("reconnect changed layer identity: %+v", layer)
		}
		if layer.ID == "layer-b" && (layer.Phase != "extracting" || layer.BytesCurrent == nil || *layer.BytesCurrent != 32*1024*1024) {
			t.Fatalf("reconnect lost current phase-local bytes: %+v", layer)
		}
	}
	reconnected.Close()
	h.SlowPull.Advance()
	eventually(t, "pull finishes before Compose apply", h.Gate.Waiting)
	eventually(t, "same deployment remains running while applying", func() bool {
		for _, run := range stream.runs() {
			if run.RunID == initialRun.RunID && run.Phase == "applying" && run.Status == "running" {
				return true
			}
		}
		return false
	})
	for _, run := range stream.runs() {
		if run.RunID == initialRun.RunID && run.Status == "completed" {
			t.Fatal("pull completion falsely completed deployment before Compose")
		}
	}
	h.Gate.Release()
	waitSynced(t, h, 2)
	assertTerminalWork(t, stream, initialRun.RunID, "completed")
	// A real stream error should fail the initial deployment, then recover on retry.
	newStack := harness.Compose("slow-new")
	if _, err := h.Commit(map[string]*string{"aaa/new/compose.yaml": &newStack}, "add slow stack"); err != nil {
		t.Fatal(err)
	}
	h.SlowPull.Arm(true)
	h.Refresh.RequestRefresh(refresh.TriggerManual)
	eventually(t, "failure fixture starts", h.SlowPull.Active)
	var failedRun string
	eventually(t, "failed attempt has identified bytes", func() bool {
		for _, run := range stream.runs() {
			if run.Stack == "aaa/new" && hasControlledLayers(run) {
				failedRun = run.RunID
				return true
			}
		}
		return false
	})
	h.SlowPull.Advance()
	h.SlowPull.Advance()
	eventually(t, "pull error ends deployment", func() bool { return !h.Queue.IsRunning() })
	for _, s := range h.Store.GetStacks() {
		if s.Path == "aaa/new" && (s.Status != desiredstate.StackSyncFailed || s.LastSyncError == "") {
			t.Fatalf("pull failure not reported: %+v", s)
		}
	}
	assertTerminalWork(t, stream, failedRun, "failed")
	h.Refresh.RequestRefresh(refresh.TriggerManual)
	waitSynced(t, h, 3)
	// The existing manual refresh cancellation path interrupts a real paused SDK
	// stream. It must publish a non-success terminal before the retry succeeds.
	cancelCompose := harness.Compose("cancel-fixture")
	if _, err := h.Commit(map[string]*string{"000/cancel/compose.yaml": &cancelCompose}, "add cancelled stack"); err != nil {
		t.Fatal(err)
	}
	h.SlowPull.Arm(false)
	h.Refresh.RequestRefresh(refresh.TriggerManual)
	eventually(t, "cancellation fixture starts", h.SlowPull.Active)
	var cancelledRun string
	eventually(t, "cancel attempt bytes", func() bool {
		for _, run := range stream.runs() {
			if run.Stack == "000/cancel" && hasControlledLayers(run) {
				cancelledRun = run.RunID
				return true
			}
		}
		return false
	})
	h.Refresh.RequestRefresh(refresh.TriggerManual)
	assertTerminalWork(t, stream, cancelledRun, "cancelled")
	waitSynced(t, h, 4)
	// Completed and failed attempts do not survive authoritative reconnects.
	final := observeWork(t, ctx, server.URL)
	defer final.Close()
	eventually(t, "final snapshot", func() bool { return len(final.snapshots()) == 1 })
	if len(final.snapshots()[0].Runs) != 0 {
		t.Fatalf("terminal runs resurrected: %+v", final.snapshots()[0])
	}
	t.Logf("HTTP SSE reconcile run=%s restored mid-pull; explicit completed=%s failed=%s cancelled=%s; final snapshot has no active runs (absence is not used as success)", initialRun.RunID, initialRun.RunID, failedRun, cancelledRun)
}

// Controlled bytes exercise the raw Engine NDJSON parser and production work
// events. The following actual DinD pull/apply is real; byte sizes are simulated.
func hasControlledLayers(run desiredstate.WorkRun) bool {
	for _, image := range run.Images {
		found := 0
		for _, layer := range image.Layers {
			if (layer.ID == "layer-a" || layer.ID == "layer-b") && layer.BytesCurrent != nil && layer.BytesTotal != nil && *layer.BytesCurrent > 0 {
				found++
			}
		}
		if found == 2 && image.PullAttemptID != "" {
			return true
		}
	}
	return false
}

func assertTerminalWork(t *testing.T, stream *workStream, id, status string) desiredstate.WorkRun {
	t.Helper()
	var terminal desiredstate.WorkRun
	eventually(t, "explicit "+status+" terminal for "+id, func() bool {
		for _, run := range stream.runs() {
			if run.RunID == id && run.Status != "running" {
				terminal = run
				return true
			}
		}
		return false
	})
	if terminal.Status != status || terminal.FinishedAt == nil {
		t.Fatalf("terminal: %+v, want %s", terminal, status)
	}
	// The terminal event may carry the last transient detail. Server live caches
	// must release it immediately: a reconnect cannot resurrect that run/layers.
	reconnected := observeWork(t, stream.ctx, stream.base)
	defer reconnected.Close()
	eventually(t, "terminal absent from live cache", func() bool { return len(reconnected.snapshots()) == 1 })
	for _, run := range reconnected.snapshots()[0].Runs {
		if run.RunID == id {
			t.Fatalf("terminal run/layers retained in live cache: %+v", run)
		}
	}
	return terminal
}

func TestAllStackWorkStagesOverHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	h, err := harness.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	server := httptest.NewServer(h.Router)
	defer server.Close()
	h.Start()
	waitSynced(t, h, 2)
	before := runtimeIDs(t, h)
	// A genuinely failed discovered stack must remain skipped by all-stack
	// maintenance. The updater must not turn its absence into a success.
	broken := "services:\n  worker:\n    definitely_invalid_property: true\n"
	if _, err := h.Commit(map[string]*string{"zzz/failed/compose.yaml": &broken}, "failed stack is skipped by maintenance"); err != nil {
		t.Fatal(err)
	}
	h.Refresh.RequestRefresh(refresh.TriggerManual)
	eventually(t, "invalid stack failed", func() bool {
		if h.Queue.IsRunning() {
			return false
		}
		for _, stack := range h.Store.GetStacks() {
			if stack.Path == "zzz/failed" {
				return stack.Status == desiredstate.StackSyncFailed
			}
		}
		return false
	})
	stream := observeWork(t, ctx, server.URL)
	defer stream.Close()
	h.SlowPull.Arm(false)
	w := httptest.NewRecorder()
	h.Router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/update", nil))
	if w.Code != http.StatusOK && w.Code != http.StatusAccepted {
		t.Fatalf("update status %d: %s", w.Code, w.Body.String())
	}
	eventually(t, "all-stack controlled pull", h.SlowPull.Active)
	var pulling desiredstate.WorkRun
	eventually(t, "scheduler correlated bytes", func() bool {
		for _, run := range stream.runs() {
			if run.Source == "scheduler" && hasControlledLayers(run) {
				pulling = run
				return true
			}
		}
		return false
	})
	if pulling.CycleID == "" || pulling.RootOperationID == "" || pulling.Status != "running" {
		t.Fatalf("all-stack identity: %+v", pulling)
	}
	// Existing mutation locks deliberately serialize pulls. No second stack may
	// start while the first one's controlled Engine stream is held.
	for _, run := range stream.runs() {
		if run.Source == "scheduler" && run.Stack != "" && run.Stack != pulling.Stack {
			t.Fatalf("second stack started before first pull ended: %+v", run)
		}
	}
	for range 4 {
		h.SlowPull.Advance()
	}
	assertTerminalWork(t, stream, pulling.RunID, "completed")
	var prune desiredstate.WorkRun
	eventually(t, "explicit host-wide prune stage", func() bool {
		for _, run := range stream.runs() {
			if run.Source == "scheduler" && run.Phase == "pruning" {
				prune = run
				return true
			}
		}
		return false
	})
	if prune.RootOperationID != pulling.RootOperationID || prune.CycleID != pulling.CycleID {
		t.Fatalf("prune lost workflow/cycle ownership: pull=%+v prune=%+v", pulling, prune)
	}
	pruneTerminal := assertTerminalWork(t, stream, prune.RunID, "completed")
	results := pruneTerminal.StackResults
	if results == nil || results.Processed != 2 || results.Skipped != 1 || results.Failed != 0 {
		t.Fatalf("all-stack outcome counts: %+v", results)
	}
	skipped := false
	for _, outcome := range results.Outcomes {
		if outcome.Stack == "zzz/failed" && outcome.Status == "skipped" {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("failed stack missing explicit skipped outcome: %+v", results)
	}
	var terminal desiredstate.WorkOperation
	eventually(t, "root update operation explicitly completes", func() bool {
		for _, op := range stream.operations() {
			if op.ID == pulling.RootOperationID && op.Status != "running" {
				terminal = op
				return true
			}
		}
		return false
	})
	if terminal.Status != "completed" {
		t.Fatalf("update root: %+v", terminal)
	}
	stacks := map[string]bool{}
	for _, run := range stream.runs() {
		if run.Source == "scheduler" && run.Stack != "" {
			stacks[run.Stack] = true
		}
	}
	if len(stacks) != 2 || stacks["zzz/failed"] {
		t.Fatalf("scheduler pull scope: %v", stacks)
	}
	for stack, id := range before {
		if after := runtimeIDs(t, h)[stack]; after != id {
			t.Fatalf("unchanged update replaced %s: %s -> %s", stack, id, after)
		}
	}
	t.Logf("all-stack SSE cycle=%s root=%s: sequential pulls %v; failed stack skipped; pull terminal separate from prune run=%s and root terminal=%s", pulling.CycleID, pulling.RootOperationID, stacks, prune.RunID, terminal.Status)
}

type workStream struct {
	ctx      context.Context
	base     string
	mu       sync.Mutex
	initial  []desiredstate.WorkSnapshot
	updates  []desiredstate.WorkRun
	ops      []desiredstate.WorkOperation
	errors   []string
	epoch    string
	sequence uint64
	close    context.CancelFunc
}

func observeWork(t *testing.T, ctx context.Context, base string) *workStream {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/events", nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		cancel()
		response.Body.Close()
		t.Fatalf("SSE status %d", response.StatusCode)
	}
	s := &workStream{close: cancel, ctx: ctx, base: base}
	t.Cleanup(func() {
		s.Close()
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.errors) > 0 {
			t.Errorf("invalid work SSE: %v", s.errors)
		}
	})
	go func() {
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 4096), 4*1024*1024)
		var event string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				event = strings.TrimPrefix(line, "event: ")
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := []byte(strings.TrimPrefix(line, "data: "))
			s.mu.Lock()
			switch event {
			case "work.snapshot":
				var value desiredstate.WorkSnapshot
				if err := json.Unmarshal(data, &value); err != nil {
					s.errors = append(s.errors, err.Error())
				} else {
					if value.SchemaVersion != 2 || value.Epoch == "" || len(s.initial) != 0 {
						s.errors = append(s.errors, "invalid authoritative snapshot header")
					}
					s.epoch, s.sequence = value.Epoch, value.Sequence
					s.initial = append(s.initial, value)
				}
			case "work.upsert":
				var value desiredstate.WorkUpsert
				if err := json.Unmarshal(data, &value); err != nil {
					s.errors = append(s.errors, err.Error())
				} else {
					if value.SchemaVersion != 2 || value.Epoch != s.epoch || value.Sequence <= s.sequence {
						s.errors = append(s.errors, fmt.Sprintf("invalid upsert barrier: epoch=%s sequence=%d after %s/%d", value.Epoch, value.Sequence, s.epoch, s.sequence))
					}
					s.sequence = value.Sequence
					s.ops = append(s.ops, value.Operations...)
					if value.Run != nil {
						s.updates = append(s.updates, *value.Run)
					}
				}
			}
			s.mu.Unlock()
		}
	}()
	return s
}

func (s *workStream) Close() { s.close() }
func (s *workStream) snapshots() []desiredstate.WorkSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]desiredstate.WorkSnapshot(nil), s.initial...)
}
func (s *workStream) runs() []desiredstate.WorkRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]desiredstate.WorkRun(nil), s.updates...)
}

func (s *workStream) operations() []desiredstate.WorkOperation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]desiredstate.WorkOperation(nil), s.ops...)
}
