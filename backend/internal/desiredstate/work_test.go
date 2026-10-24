package desiredstate

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/lucasreiners/docker-cd/internal/operations"
)

func workSnapshot(t *testing.T, b *Broadcaster) WorkSnapshot {
	t.Helper()
	sub, events := b.SubscribeWithSnapshot(NewStore())
	defer b.Unsubscribe(sub)
	var snapshot WorkSnapshot
	if err := json.Unmarshal([]byte(events[len(events)-1].Data), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion != 2 || snapshot.Epoch == "" || snapshot.Runs == nil || snapshot.Operations == nil {
		t.Fatalf("invalid snapshot: %+v", snapshot)
	}
	return snapshot
}

func nextWork(t *testing.T, sub *Subscriber) WorkUpsert {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case e := <-sub.Events:
			if e.Type != EventWorkUpsert {
				continue
			}
			var upsert WorkUpsert
			if err := json.Unmarshal([]byte(e.Data), &upsert); err != nil {
				t.Fatal(err)
			}
			return upsert
		case <-deadline:
			t.Fatal("missing work upsert")
			return WorkUpsert{}
		}
	}
}

func drainWork(sub *Subscriber) []WorkUpsert {
	updates := []WorkUpsert{}
	for {
		select {
		case event := <-sub.Events:
			if event.Type == EventWorkUpsert {
				var u WorkUpsert
				_ = json.Unmarshal([]byte(event.Data), &u)
				updates = append(updates, u)
			}
		default:
			return updates
		}
	}
}

func TestWorkOperationOnlyAndCompleteActiveAncestry(t *testing.T) {
	b := NewBroadcaster()
	parent := b.BeginOperation(context.Background(), "refresh", "manual", "", "main")
	b.EndOperation(parent, "completed", "checked")
	child := b.BeginOperation(parent, "deployment", "manual", "nested/web", "revision")
	for i := 0; i < 55; i++ {
		ctx := b.BeginOperation(context.Background(), "refresh", "poll", "", "")
		b.EndOperation(ctx, "completed", "done")
	}
	independent := b.BeginOperation(context.Background(), "action", "retry", "other/web", "revision")
	snapshot := workSnapshot(t, b)
	if len(snapshot.Runs) != 0 || len(snapshot.Operations) != 3 {
		t.Fatalf("history page used as active authority: %+v", snapshot)
	}
	roots := map[string]bool{}
	for _, op := range snapshot.Operations {
		roots[op.RootOperationID] = true
		if op.ID == operations.ID(child) && op.ParentID != operations.ID(parent) {
			t.Fatal("wrong ancestry")
		}
	}
	if len(roots) != 2 || !roots[operations.ID(parent)] || !roots[operations.ID(independent)] {
		t.Fatalf("duplicated workflow roots: %+v", roots)
	}
	sub := b.Subscribe()
	defer b.Unsubscribe(sub)
	b.OperationStep(independent, "waiting", "Waiting for execution lock")
	step := nextWork(t, sub)
	if step.Run != nil || len(step.Operations) != 1 || step.Operations[0].Version != 2 {
		t.Fatalf("not an operation-only metadata update: %+v", step)
	}
	b.EndOperation(independent, "failed", "secret://user:password@registry")
	ended := nextWork(t, sub)
	if ended.Operations[0].FinishedAt == nil || strings.Contains(ended.Operations[0].Message, "password") {
		t.Fatal("invalid or unsafe terminal summary")
	}
	if len(workSnapshot(t, b).Operations) != 2 {
		t.Fatal("ended independent root remains active")
	}
	b.EndOperation(child, "completed", "done")
	if len(workSnapshot(t, b).Operations) != 0 {
		t.Fatal("completed ancestor retained without active descendants")
	}
}

func TestWorkIsolationRetryTerminalAndReconnect(t *testing.T) {
	b := NewBroadcaster()
	root := b.BeginOperation(context.Background(), "image_update", "manual", "", "")
	first := b.BeginWork(root, WorkOptions{Source: "scheduler", Stack: "one/web", Phase: "pulling"})
	second := b.BeginWork(root, WorkOptions{Source: "scheduler", Stack: "two/web", Phase: "pulling"})
	b.WorkPullProgress(first, WorkPullProgress{Image: "nginx", Current: 1, Total: 1, LayerID: "same", Status: "Downloading", BytesCurrent: 12})
	b.WorkPullProgress(second, WorkPullProgress{Image: "nginx", Current: 1, Total: 1, LayerID: "same", Status: "Downloading", BytesCurrent: 99, BytesTotal: 100})
	snapshot := workSnapshot(t, b)
	if len(snapshot.Runs) != 2 {
		t.Fatal("same-image runs collided")
	}
	byID := map[string]WorkRun{}
	for _, r := range snapshot.Runs {
		byID[r.RunID] = r
	}
	a, c := byID[first].Images[0], byID[second].Images[0]
	if first == second || a.PullAttemptID == c.PullAttemptID || *a.Layers[0].BytesCurrent != 12 || a.Layers[0].BytesTotal != nil || *c.Layers[0].BytesCurrent != 99 {
		t.Fatal("image/layer identity or unknown totals lost")
	}
	sub := b.Subscribe()
	defer b.Unsubscribe(sub)
	b.EndWork(first, "completed", "Pull returned", "")
	terminal := nextWork(t, sub)
	if terminal.Run == nil || terminal.Run.Status != "completed" || terminal.Run.FinishedAt == nil || terminal.Run.Phase != "finished" {
		t.Fatalf("bad terminal: %+v", terminal)
	}
	sequence := terminal.Sequence
	b.WorkPullProgress(first, WorkPullProgress{Image: "nginx", LayerID: "same", Status: "Downloading"})
	b.EndWork(first, "failed", "late", "")
	if got := drainWork(sub); len(got) != 0 {
		t.Fatal("terminal run mutated")
	}
	retry := b.BeginWork(root, WorkOptions{Source: "scheduler", Stack: "one/web", Phase: "pulling"})
	b.WorkPullProgress(retry, WorkPullProgress{Image: "nginx", LayerID: "same", Status: "Downloading"})
	if retry == first {
		t.Fatal("retry reused run id")
	}
	reconnected := workSnapshot(t, b)
	if reconnected.Sequence <= sequence || len(reconnected.Runs) != 2 || len(reconnected.Operations) != 1 || reconnected.Operations[0].Status != "running" {
		t.Fatal("pull completion ended workflow or resurrected terminal")
	}
	b.EndWork(second, "failed", "failed", "")
	b.EndWork(retry, "cancelled", "cancelled", "")
	b.EndOperation(root, "completed_with_errors", "done")
	if latest := workSnapshot(t, b); len(latest.Runs) != 0 || len(latest.Operations) != 0 {
		t.Fatal("terminal cache retained")
	}
}

func TestWorkBytesCoalescedAndPhasesTerminalFlushed(t *testing.T) {
	b := NewBroadcaster()
	ctx := b.BeginOperation(context.Background(), "deployment", "manual", "web", "")
	id := b.BeginWork(ctx, WorkOptions{Source: "reconcile", Stack: "web", Phase: "pulling"})
	sub := b.Subscribe()
	defer b.Unsubscribe(sub)
	p := WorkPullProgress{Image: "image", LayerID: "layer", Status: "Downloading", BytesCurrent: 1, BytesTotal: 100}
	b.WorkPullProgress(id, p)
	initial := nextWork(t, sub)
	for i := int64(2); i < 20; i++ {
		p.BytesCurrent = i
		b.WorkPullProgress(id, p)
	}
	if len(drainWork(sub)) != 0 {
		t.Fatal("byte-only burst not coalesced")
	}
	flushed := nextWork(t, sub)
	if flushed.Sequence <= initial.Sequence || *flushed.Run.Images[0].Layers[0].BytesCurrent != 19 {
		t.Fatal("timer did not deliver latest replacement")
	}
	p.Status = "Extracting"
	p.BytesCurrent = 0
	p.BytesTotal = 0
	b.WorkPullProgress(id, p)
	extract := nextWork(t, sub)
	layer := extract.Run.Images[0].Layers[0]
	if layer.Phase != "extracting" || *layer.BytesCurrent != 0 || layer.BytesTotal != nil {
		t.Fatal("phase-local bytes reused")
	}
	p.BytesCurrent = 7
	b.WorkPullProgress(id, p)
	b.OperationStep(ctx, "applying", "Applying Compose configuration")
	metadata := nextWork(t, sub)
	latest := nextWork(t, sub)
	if metadata.Run != nil || latest.Run == nil || *latest.Run.Images[0].Layers[0].BytesCurrent != 7 {
		t.Fatal("metadata did not flush pending bytes")
	}
	b.WorkPullFinished(id, []string{"image"}, nil)
	_ = nextWork(t, sub)
	b.WorkPhase(id, "applying", "Applying Compose configuration")
	apply := nextWork(t, sub)
	if apply.Run.Status != "running" || !apply.Run.Images[0].Done {
		t.Fatal("pull completion inferred deployment completion")
	}
	b.EndWork(id, "failed", "Apply failed", "apply_failed")
	terminal := nextWork(t, sub)
	if terminal.Run.Status != "failed" {
		t.Fatal("terminal not flushed")
	}
	time.Sleep(550 * time.Millisecond)
	if len(drainWork(sub)) != 0 {
		t.Fatal("pending timer resurrected terminal")
	}
}

func TestWorkAtomicBarrierUnderPublication(t *testing.T) {
	b := NewBroadcaster()
	ctx := b.BeginOperation(context.Background(), "deployment", "manual", "web", "")
	id := b.BeginWork(ctx, WorkOptions{Source: "reconcile", Phase: "applying"})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			b.OperationStep(ctx, "applying", "Applying")
			b.WorkPhase(id, "applying", "Applying")
		}
	}()
	for i := 0; i < 100; i++ {
		sub, events := b.SubscribeWithSnapshot(NewStore())
		var snap WorkSnapshot
		_ = json.Unmarshal([]byte(events[3].Data), &snap)
		if len(snap.Runs) != 1 || len(snap.Operations) != 1 || snap.Runs[0].OperationID != snap.Operations[0].ID {
			t.Fatal("torn operation/run snapshot")
		}
		updates := drainWork(sub)
		previous := snap.Sequence
		for _, u := range updates {
			if u.Epoch != snap.Epoch || u.Sequence <= previous {
				t.Fatal("event did not follow snapshot watermark")
			}
			previous = u.Sequence
		}
		b.Unsubscribe(sub)
	}
	wg.Wait()
	b.EndWork(id, "completed", "done", "")
	b.EndOperation(ctx, "completed", "done")
	if s := workSnapshot(t, b); len(s.Runs) != 0 {
		t.Fatal("stale terminal reconnect")
	}
}

func TestWorkLimitsBudgetAndRelease(t *testing.T) {
	b := NewBroadcaster()
	ctx := b.BeginOperation(context.Background(), "deployment", "manual", "web", "")
	id := b.BeginWork(ctx, WorkOptions{Source: "reconcile", Phase: "pulling"})
	for i := 0; i < WorkLayerLimit+10; i++ {
		b.WorkPullProgress(id, WorkPullProgress{Image: "image", LayerID: fmt.Sprint(i), Status: "Downloading"})
	}
	s := workSnapshot(t, b)
	image := s.Runs[0].Images[0]
	if len(image.Layers) != WorkLayerLimit || !image.DetailsTruncated || !s.DetailsTruncated || image.LayerCount != WorkLayerLimit+10 {
		t.Fatal("layer truncation not truthful")
	}
	b.EndWork(id, "completed", "done", "")
	// Large references/IDs cannot make the live-detail cache exceed its budget.
	for i := 0; i < 3; i++ {
		id = b.BeginWork(ctx, WorkOptions{Source: "reconcile", Phase: "pulling"})
		b.WorkPullProgress(id, WorkPullProgress{Image: strings.Repeat("reference", 100000), LayerID: strings.Repeat("layer", 100000), Status: "Downloading"})
	}
	s = workSnapshot(t, b)
	if len(s.Runs) != 3 || !s.DetailsTruncated {
		t.Fatal("budget discarded required summaries or missed truncation")
	}
	b.mu.Lock()
	total := 0
	for _, run := range b.work.runs {
		total += detailBytes(run)
	}
	b.mu.Unlock()
	if total > WorkDetailBudget {
		t.Fatalf("live detail budget exceeded: %d", total)
	}
	for _, run := range s.Runs {
		b.EndWork(run.RunID, "cancelled", "cancelled", "")
	}
	for i := 0; i < 100; i++ {
		id = b.BeginWork(ctx, WorkOptions{Source: "refresh", Phase: "checking_repository"})
		b.EndWork(id, "completed", "done", "")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.work.runs) != 0 {
		t.Fatal("terminal memory retained")
	}
}

func TestWorkImageLimitAndSequenceExhaustion(t *testing.T) {
	b := NewBroadcaster()
	ctx := b.BeginOperation(context.Background(), "deployment", "manual", "web", "")
	id := b.BeginWork(ctx, WorkOptions{Source: "reconcile", Phase: "pulling"})
	for i := 1; i <= 40; i++ {
		b.WorkPullProgress(id, WorkPullProgress{Image: fmt.Sprintf("image-%d", i), Current: i, Total: 40, Done: true})
	}
	snapshot := workSnapshot(t, b)
	if snapshot.Runs[0].ImageCount != 40 || len(snapshot.Runs[0].Images) != WorkImageLimit || !snapshot.DetailsTruncated {
		t.Fatal("image attempts not bounded/counts lost")
	}
	for _, image := range snapshot.Runs[0].Images {
		if !image.Done || image.Status != "completed" || image.Layers == nil {
			t.Fatal("invalid image wire shape")
		}
	}
	sub := b.Subscribe()
	defer b.Unsubscribe(sub)
	b.mu.Lock()
	b.work.sequence = 9007199254740991
	b.mu.Unlock()
	b.WorkPhase(id, "applying", "Applying")
	select {
	case <-sub.Done():
	default:
		t.Fatal("sequence reset did not force subscription barrier")
	}
	after := workSnapshot(t, b)
	if after.Epoch == snapshot.Epoch || after.Sequence != 1 || len(after.Runs) != 1 {
		t.Fatal("unsafe sequence wrap/reset")
	}
	b.EndWork(id, "completed", "done", "")
}

func TestWorkPullWithoutCallbacksAndBoundedOutcomes(t *testing.T) {
	b := NewBroadcaster()
	ctx := b.BeginOperation(context.Background(), "deployment", "manual", "web", "")
	id := b.BeginWork(ctx, WorkOptions{Source: "reconcile", Phase: "pulling"})
	b.WorkPullFinished(id, []string{"a", "b"}, fmt.Errorf("private password"))
	run := workSnapshot(t, b).Runs[0]
	if run.ImageCount != 1 || run.Images[0].Reference != "a" || run.Images[0].Status != "failed" {
		t.Fatal("uninvoked image fabricated or first failure lost")
	}
	b.EndWork(id, "failed", "Pull failed", "")
	id = b.BeginWork(ctx, WorkOptions{Source: "scheduler", Phase: "pruning"})
	results := WorkStackResults{}
	for i := 0; i < 150; i++ {
		status := "completed"
		if i%3 == 0 {
			status = "skipped"
		}
		if i%3 == 1 {
			status = "failed"
		}
		AddWorkStackOutcome(&results, WorkStackOutcome{Stack: fmt.Sprint(i), Status: status})
	}
	b.SetWorkStackResults(id, results)
	run = workSnapshot(t, b).Runs[0]
	if run.Stack != "" || len(run.Images) != 0 || run.StackResults.Processed != 100 || run.StackResults.Skipped != 50 || run.StackResults.Failed != 50 || run.StackResults.OutcomeCount != 150 || len(run.StackResults.Outcomes) != 100 || !run.DetailsTruncated {
		t.Fatalf("bad aggregate: %+v", run)
	}
}

func TestWorkSeedAndRestartEpoch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store, _ := operations.Open(path)
	parent := store.Start("", "refresh", "manual", "", "")
	store.Update(parent.ID, "completed", "done", true)
	child := store.Start(parent.ID, "deployment", "manual", "web", "")
	b := NewBroadcaster()
	b.SetOperations(store)
	before := workSnapshot(t, b)
	if len(before.Operations) != 2 {
		t.Fatal("restored in-memory running ancestry not seeded")
	}
	reopened, err := operations.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewBroadcaster()
	restarted.SetOperations(reopened)
	after := workSnapshot(t, restarted)
	if after.Epoch == before.Epoch || len(after.Operations) != 0 || len(after.Runs) != 0 {
		t.Fatal("restart resurrected live state")
	}
	r, _ := reopened.Get(child.ID)
	if r.Status != "interrupted" {
		t.Fatal("restart inferred completion")
	}
}

func TestWorkMalformedInputAndSafeText(t *testing.T) {
	b := NewBroadcaster()
	ctx := b.BeginOperation(context.Background(), "deployment", "manual", "web", "")
	id := b.BeginWork(ctx, WorkOptions{Source: "reconcile", Phase: "pulling"})
	b.WorkPullProgress(id, WorkPullProgress{Image: "image", Current: 3, Total: 2})
	if workSnapshot(t, b).Runs[0].ImageCount != 0 {
		t.Fatal("malformed position accepted")
	}
	b.SetWorkStackResults(id, WorkStackResults{Processed: 1, Failed: 2, OutcomeCount: 1})
	if workSnapshot(t, b).Runs[0].StackResults != nil {
		t.Fatal("inconsistent aggregate accepted")
	}
	b.WorkPullProgress(id, WorkPullProgress{Image: "image", LayerID: "layer", Status: "password=secret", BytesCurrent: -1, BytesTotal: -1})
	layer := workSnapshot(t, b).Runs[0].Images[0].Layers[0]
	if layer.Phase != "unknown" || layer.BytesCurrent != nil || layer.BytesTotal != nil || strings.Contains(layer.Status, "secret") {
		t.Fatal("unknown/unsafe progress accepted")
	}
	b.WorkPhase(id, "applying", strings.Repeat("界", 500))
	message := workSnapshot(t, b).Runs[0].Message
	if len(message) > 1024 || !utf8.ValidString(message) {
		t.Fatal("message bound corrupted UTF-8")
	}
	var nilBroadcaster *Broadcaster
	if nilBroadcaster.BeginWork(context.Background(), WorkOptions{}) != "" {
		t.Fatal("nil publisher created identity")
	}
	nilBroadcaster.WorkPhase("", "applying", "")
	nilBroadcaster.WorkPullProgress("", WorkPullProgress{})
	nilBroadcaster.WorkPullFinished("", nil, nil)
	nilBroadcaster.EndWork("", "completed", "", "")
	nilBroadcaster.SetWorkStackResults("", WorkStackResults{})
}

func TestWorkOverflowRequiresFreshBarrier(t *testing.T) {
	b := NewBroadcaster()
	ctx := b.BeginOperation(context.Background(), "deployment", "manual", "web", "")
	id := b.BeginWork(ctx, WorkOptions{Source: "reconcile", Phase: "applying"})
	sub, events := b.SubscribeWithSnapshot(NewStore())
	defer b.Unsubscribe(sub)
	var initial WorkSnapshot
	_ = json.Unmarshal([]byte(events[3].Data), &initial)
	for i := 0; i < 70; i++ {
		b.WorkPhase(id, "applying", "Applying")
	}
	select {
	case <-sub.Done():
	default:
		t.Fatal("overflow silently dropped ordered work")
	}
	reconnected := workSnapshot(t, b)
	if reconnected.Sequence <= initial.Sequence || len(reconnected.Runs) != 1 || reconnected.Runs[0].Status != "running" {
		t.Fatal("overflow reconnect fabricated terminal or lost membership")
	}
	b.EndWork(id, "completed", "done", "")
}
