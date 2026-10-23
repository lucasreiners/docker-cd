package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
)

type runtimeLister struct {
	mu                       sync.Mutex
	calls, active, maxActive int
	rows                     map[string][]desiredstate.ContainerInfo
	errors                   map[string]error
	started                  chan struct{}
	release                  chan struct{}
}

func (l *runtimeLister) Reconcile(context.Context) []reconcile.ReconciliationRun { return nil }
func (l *runtimeLister) GetContainers(ctx context.Context, path string) ([]desiredstate.ContainerInfo, error) {
	l.mu.Lock()
	l.calls++
	l.active++
	l.maxActive = max(l.maxActive, l.active)
	first := l.calls == 1
	started, release := l.started, l.release
	rows := append([]desiredstate.ContainerInfo{}, l.rows[path]...)
	err := l.errors[path]
	l.mu.Unlock()
	defer func() { l.mu.Lock(); l.active--; l.mu.Unlock() }()
	if first && started != nil {
		close(started)
	}
	if first && release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return rows, err
}
func (l *runtimeLister) count() int { l.mu.Lock(); defer l.mu.Unlock(); return l.calls }
func runtimeFixture(paths ...string) (*RuntimeReader, *runtimeLister, *desiredstate.Store) {
	store := desiredstate.NewStore()
	stacks := make([]desiredstate.StackRecord, 0, len(paths))
	for _, path := range paths {
		stacks = append(stacks, desiredstate.StackRecord{Path: path})
	}
	store.Set(&desiredstate.Snapshot{Stacks: stacks})
	lister := &runtimeLister{rows: map[string][]desiredstate.ContainerInfo{}, errors: map[string]error{}}
	return NewRuntimeReader(store, lister, nil, nil), lister, store
}
func readRuntime(t *testing.T, r *RuntimeReader) map[string]RuntimeObservation {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := r.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRuntimeSharedCoalescingAndAccurateCacheTimes(t *testing.T) {
	r, l, _ := runtimeFixture("apps/web")
	l.started = make(chan struct{})
	l.release = make(chan struct{})
	l.rows["apps/web"] = []desiredstate.ContainerInfo{{ID: "short", Image: "nginx:stable"}}
	results := make(chan map[string]RuntimeObservation, 20)
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		go func() { out, err := r.Read(context.Background()); results <- out; errs <- err }()
	}
	<-l.started
	close(l.release)
	var first RuntimeObservation
	for i := 0; i < 20; i++ {
		out := <-results
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		first = out["apps/web"]
		if len(first.Containers) != 1 {
			t.Fatal(out)
		}
	}
	if l.count() != 1 {
		t.Fatal("uncoalesced compose calls", l.count())
	}
	cached := readRuntime(t, r)["apps/web"]
	if !cached.ObservedAt.Equal(*first.ObservedAt) || !cached.AttemptedAt.Equal(first.AttemptedAt) || !cached.Containers[0].ImageIdentity.ObservedAt.Equal(first.Containers[0].ImageIdentity.ObservedAt) {
		t.Fatal("cache fabricated observation times")
	}
	rows, err := r.GetContainers(context.Background(), "apps/web")
	if err != nil || len(rows) != 1 || l.count() != 1 {
		t.Fatal("detail did not share observation", err)
	}
	rows[0].ImageIdentity.Reason = "corrupted"
	if readRuntime(t, r)["apps/web"].Containers[0].ImageIdentity.Reason == "corrupted" {
		t.Fatal("mutable cache escaped")
	}
	if _, err := r.GetContainers(context.Background(), "other/host"); !errors.Is(err, errUnknownRuntimeStack) || l.count() != 1 {
		t.Fatal("arbitrary stack reached compose")
	}
}

func TestRuntimeFailureIsolationLastSuccessAndEmptySuccess(t *testing.T) {
	r, l, _ := runtimeFixture("good", "flaky", "never", "empty")
	l.rows["flaky"] = []desiredstate.ContainerInfo{{ID: "old"}}
	l.errors["never"] = errors.New("auth-token-secret")
	first := readRuntime(t, r)
	if first["never"].ObservedAt != nil || len(first["never"].Containers) != 0 || first["never"].Error == nil {
		t.Fatal("failure became empty success")
	}
	if first["empty"].ObservedAt == nil || first["empty"].Error != nil || first["empty"].Containers == nil {
		t.Fatal("empty success is not observed")
	}
	l.mu.Lock()
	l.errors["flaky"] = errors.New("https://user:password@registry/token")
	l.mu.Unlock()
	r.Invalidate()
	second := readRuntime(t, r)
	if second["flaky"].Error == nil || second["flaky"].Containers[0].ID != "old" || !second["flaky"].ObservedAt.Equal(*first["flaky"].ObservedAt) || !second["flaky"].AttemptedAt.After(first["flaky"].AttemptedAt) {
		t.Fatal("last success lost", second)
	}
	if second["good"].Error != nil || !second["good"].ObservedAt.After(*first["good"].ObservedAt) {
		t.Fatal("other stack failure hid success")
	}
	encoded, _ := json.Marshal(second)
	for _, secret := range []string{"password", "auth-token-secret", "registry/token"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("raw compose error leaked")
		}
	}
	if _, err := r.GetContainers(context.Background(), "flaky"); err == nil {
		t.Fatal("detail erased listing failure")
	}
}

func TestRuntimeInvalidatesStackChangesTerminalDeploymentReconnectAndExpiry(t *testing.T) {
	r, l, store := runtimeFixture("apps/web")
	b := desiredstate.NewBroadcaster()
	r.broadcaster = b
	readRuntime(t, r)
	op := b.BeginOperation(context.Background(), "deployment", "test", "apps/web", "")
	readRuntime(t, r)
	if l.count() != 1 {
		t.Fatal("running work unexpectedly invalidated")
	}
	b.EndOperation(op, "failed", "done")
	readRuntime(t, r)
	if l.count() != 2 {
		t.Fatal("terminal deployment not invalidated")
	}
	store.Set(&desiredstate.Snapshot{Stacks: []desiredstate.StackRecord{{Path: "apps/web", ComposeHash: "new"}, {Path: "other"}}})
	readRuntime(t, r)
	if l.count() != 4 {
		t.Fatal("stack change not invalidated")
	}
	r.Reconnect()
	readRuntime(t, r)
	if l.count() != 6 {
		t.Fatal("reconnect not invalidated")
	}
	r.Reconnect()
	readRuntime(t, r)
	if l.count() != 6 {
		t.Fatal("reconnect burst caused poll fanout")
	}
	r.mu.Lock()
	for path := range r.expires {
		r.expires[path] = time.Now().Add(-time.Second)
	}
	r.mu.Unlock()
	readRuntime(t, r)
	if l.count() != 8 {
		t.Fatal("TTL did not expire")
	}
	store.Set(&desiredstate.Snapshot{Stacks: []desiredstate.StackRecord{{Path: "other"}}})
	if out := readRuntime(t, r); len(out) != 1 {
		t.Fatal("removed stacks retained", out)
	}
	if _, err := r.GetContainers(context.Background(), "apps/web"); !errors.Is(err, errUnknownRuntimeStack) {
		t.Fatal(err)
	}
}

func TestRuntimeInflightInvalidationAndCancelledWaiter(t *testing.T) {
	r, l, store := runtimeFixture("old")
	l.started = make(chan struct{})
	l.release = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := r.Read(ctx); done <- err }()
	<-l.started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("waiter not cancelled", err)
	}
	// A cancelled client must not cancel other clients' shared observation.
	r.Invalidate()
	store.Set(&desiredstate.Snapshot{Stacks: []desiredstate.StackRecord{{Path: "new"}}})
	close(l.release)
	out := readRuntime(t, r)
	if len(out) != 1 || out["new"].ObservedAt == nil || l.count() != 2 {
		t.Fatal("stale inflight observation escaped", out, l.count())
	}
	if l.maxActive != 1 {
		t.Fatal("unbounded Compose concurrency", l.maxActive)
	}
}

func TestRuntimeCycleTimeoutPreservesUnknownAndDoesNotFanOut(t *testing.T) {
	r, l, _ := runtimeFixture("blocked", "later")
	r.cycleTimeout = 20 * time.Millisecond
	l.started = make(chan struct{})
	l.release = make(chan struct{})
	out := readRuntime(t, r)
	if out["blocked"].ObservedAt != nil || out["later"].ObservedAt != nil || out["blocked"].Error == nil || out["later"].Error == nil {
		t.Fatal("timeout fabricated success", out)
	}
	if l.count() != 1 || l.maxActive != 1 {
		t.Fatal("called Compose after cycle timeout")
	}
}

// Embedding the legacy API lets these tests assert that no other Docker operation
// (pull/prune/list/mutation) is invoked: any such call would panic immediately.
type runtimeSDK struct {
	DockerAPI
	containers, images int
	fail               bool
}

func (s *runtimeSDK) ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error) {
	s.containers++
	return container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{ID: id, Image: "sha256:" + strings.Repeat("b", 64)}, Config: &container.Config{Image: "nginx:stable", Env: []string{"SECRET=env-secret"}, Labels: map[string]string{"com.docker-cd.stack.path": "apps/web", "com.docker.compose.project": "apps-web", "com.docker.compose.service": "web"}}}, nil
}
func (s *runtimeSDK) ImageInspectWithRaw(ctx context.Context, id string) (image.InspectResponse, []byte, error) {
	s.images++
	if s.fail {
		return image.InspectResponse{}, nil, errors.New("token-secret")
	}
	return image.InspectResponse{ID: id, RepoDigests: []string{"nginx@sha256:" + strings.Repeat("c", 64)}}, []byte("raw-secret"), nil
}

func TestRuntimeAndDetailRoutesShareWhitelistedDTOAndCapabilities(t *testing.T) {
	_, l, store := runtimeFixture("apps/web")
	l.rows["apps/web"] = []desiredstate.ContainerInfo{{ID: strings.Repeat("a", 12), FullID: strings.Repeat("a", 64), ComposeProject: "apps-web", Service: "web", Image: "nginx:stable"}}
	sdk := &runtimeSDK{}
	b := desiredstate.NewBroadcaster()
	router := NewRouter(config.Config{}, nil, store, nil, l, nil, sdk, b)
	get := func(url string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
		return rec
	}
	global := get("/api/runtime")
	detail := get("/api/stacks/containers/apps/web")
	if global.Code != 200 || detail.Code != 200 || l.count() != 1 || sdk.containers != 1 || sdk.images != 1 {
		t.Fatal("routes not shared", global.Body, detail.Body)
	}
	var observed map[string]RuntimeObservation
	var rows []desiredstate.ContainerInfo
	if err := json.Unmarshal(global.Body.Bytes(), &observed); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if rows[0].ID != strings.Repeat("a", 12) || rows[0].ImageIdentity.Status != "available" || !rows[0].ImageIdentity.ObservedAt.Equal(observed["apps/web"].Containers[0].ImageIdentity.ObservedAt) {
		t.Fatal("wire mismatch", rows)
	}
	for _, secret := range []string{"env-secret", "raw-secret", "FullID", "ComposeProject", strings.Repeat("a", 64)} {
		if strings.Contains(global.Body.String(), secret) || strings.Contains(detail.Body.String(), secret) {
			t.Fatal("internal data leaked", secret)
		}
	}
	if got := get("/api/stacks/containers/arbitrary"); got.Code != 404 || l.count() != 1 {
		t.Fatal("unscoped container listing")
	}
	instance := get("/api/instance")
	if !strings.Contains(instance.Body.String(), `"liveWorkVersion":2`) || !strings.Contains(instance.Body.String(), `"imageIdentity":{"source":"container_image_inspect","version":1}`) {
		t.Fatal("capabilities changed", instance.Body.String())
	}
}

func TestRuntimeIdentityFailureDoesNotHideContainerListing(t *testing.T) {
	r, l, _ := runtimeFixture("apps/web")
	r.inspector = &runtimeSDK{fail: true}
	l.rows["apps/web"] = []desiredstate.ContainerInfo{{ID: "short", FullID: strings.Repeat("a", 64), ComposeProject: "apps-web", Service: "web", Image: "nginx:stable"}}
	got := readRuntime(t, r)["apps/web"]
	if got.Error != nil || got.ObservedAt == nil || len(got.Containers) != 1 || got.Containers[0].ImageIdentity.Reason != "inspection_failed" {
		t.Fatal("identity failure erased successful listing", got)
	}
}

func TestRuntime250MockStacksObservationMeasurement(t *testing.T) {
	paths := make([]string, 250)
	for i := range paths {
		paths[i] = fmt.Sprintf("apps/stack-%03d", i)
	}
	r, l, _ := runtimeFixture(paths...)
	for _, path := range paths {
		l.rows[path] = []desiredstate.ContainerInfo{{ID: "short", Service: "web", Image: "nginx:stable"}}
	}
	start := time.Now()
	out := readRuntime(t, r)
	elapsed := time.Since(start)
	if len(out) != 250 || l.count() != 250 || l.maxActive != 1 {
		t.Fatal("unexpected observation pressure", len(out), l.count(), l.maxActive)
	}
	start = time.Now()
	readRuntime(t, r)
	cached := time.Since(start)
	if l.count() != 250 {
		t.Fatal("cache repeated 250 Compose calls")
	}
	t.Logf("250 MOCK stacks: cold=%s cached=%s Compose calls=250 cached calls=0 max Compose concurrency=1; no real subprocess/daemon latency measured", elapsed, cached)
}

func TestRuntimeTargetedReadAndTerminalInvalidationDoNotReobserveOtherStacks(t *testing.T) {
	r, l, _ := runtimeFixture("apps/web", "other")
	b := desiredstate.NewBroadcaster()
	r.broadcaster = b
	if _, err := r.GetContainers(context.Background(), "apps/web"); err != nil {
		t.Fatal(err)
	}
	if l.count() != 1 {
		t.Fatal("detail triggered a full-inventory Compose cycle")
	}
	first := readRuntime(t, r)
	if l.count() != 2 {
		t.Fatal("global read repeated the detail observation")
	}
	op := b.BeginOperation(context.Background(), "deployment", "test", "apps/web", "")
	b.EndOperation(op, "completed", "done")
	second := readRuntime(t, r)
	if l.count() != 3 || !second["other"].ObservedAt.Equal(*first["other"].ObservedAt) || !second["apps/web"].ObservedAt.After(*first["apps/web"].ObservedAt) {
		t.Fatal("terminal deployment caused unrelated observation fanout")
	}
}

func TestRuntimeTerminalDuringInflightListingCannotReturnOldRows(t *testing.T) {
	r, l, _ := runtimeFixture("apps/web")
	b := desiredstate.NewBroadcaster()
	r.broadcaster = b
	l.rows["apps/web"] = []desiredstate.ContainerInfo{{ID: "old"}}
	l.started, l.release = make(chan struct{}), make(chan struct{})
	op := b.BeginOperation(context.Background(), "deployment", "test", "apps/web", "")
	done := make(chan map[string]RuntimeObservation, 1)
	errors := make(chan error, 1)
	go func() { out, err := r.Read(context.Background()); done <- out; errors <- err }()
	<-l.started
	l.mu.Lock()
	l.rows["apps/web"] = []desiredstate.ContainerInfo{{ID: "replacement"}}
	l.mu.Unlock()
	b.EndOperation(op, "completed", "done")
	close(l.release)
	out := <-done
	if err := <-errors; err != nil {
		t.Fatal(err)
	}
	if l.count() != 2 || out["apps/web"].Containers[0].ID != "replacement" {
		t.Fatal("terminal invalidation let an old observation win", out)
	}
}

type slowRuntimeSDK struct{ runtimeSDK }

func (s *slowRuntimeSDK) ContainerInspect(ctx context.Context, _ string) (container.InspectResponse, error) {
	s.containers++
	<-ctx.Done()
	return container.InspectResponse{}, ctx.Err()
}

func TestRuntimeSlowIdentityCannotStarveOtherContainerListings(t *testing.T) {
	r, l, _ := runtimeFixture("apps/web", "other")
	sdk := &slowRuntimeSDK{}
	r.inspector = sdk
	r.cycleTimeout = 20 * time.Millisecond
	for _, path := range []string{"apps/web", "other"} {
		l.rows[path] = []desiredstate.ContainerInfo{{FullID: strings.Repeat("a", 64), ComposeProject: "project", Service: "web", Image: "nginx:stable"}}
	}
	out := readRuntime(t, r)
	for path, observation := range out {
		if observation.Error != nil || observation.ObservedAt == nil || len(observation.Containers) != 1 || observation.Containers[0].ImageIdentity.Reason != "inspection_failed" {
			t.Fatalf("slow identity hid listing for %s: %+v", path, observation)
		}
	}
	if l.count() != 2 || sdk.containers != 1 || sdk.images != 0 {
		t.Fatal("identity cancellation caused unnecessary SDK calls")
	}
}
