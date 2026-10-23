package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
)

type largeRuntimeIdentitySDK struct {
	runtimeSDK
	digests []string
}

func (s *largeRuntimeIdentitySDK) ImageInspectWithRaw(_ context.Context, id string) (image.InspectResponse, []byte, error) {
	return image.InspectResponse{ID: id, RepoDigests: s.digests}, nil, nil
}

func TestRuntimeEnrichmentBudgetRejectsWholeObservation(t *testing.T) {
	r, l, _ := runtimeFixture("apps/web")
	l.rows["apps/web"] = []desiredstate.ContainerInfo{{ID: "last-success"}}
	old := readRuntime(t, r)["apps/web"]
	sdk := &largeRuntimeIdentitySDK{}
	for i := 0; i < 16; i++ {
		sdk.digests = append(sdk.digests, fmt.Sprintf("registry.example/%s%d@sha256:%s", strings.Repeat("a", 150), i, strings.Repeat("c", 64)))
	}
	r.inspector = sdk
	l.mu.Lock()
	l.rows["apps/web"] = make([]desiredstate.ContainerInfo, 200)
	for i := range l.rows["apps/web"] {
		l.rows["apps/web"][i] = desiredstate.ContainerInfo{ID: "short", FullID: strings.Repeat("a", 64), ComposeProject: "apps-web", Service: "web", Image: "nginx:stable"}
	}
	l.mu.Unlock()
	r.Invalidate()
	got := readRuntime(t, r)["apps/web"]
	if got.Error == nil || *got.Error != errRuntimeLimit.Error() || len(got.Containers) != 1 || got.Containers[0].ID != "last-success" || !got.ObservedAt.Equal(*old.ObservedAt) {
		t.Fatal("enrichment budget returned a truncated successful listing", got)
	}
	// An oversized digest set is instead an identity-only unavailable result;
	// its bounded empty metadata does not erase the valid container listing.
	sdk.digests = append(sdk.digests, "nginx@sha256:"+strings.Repeat("c", 64))
	r.Invalidate()
	got = readRuntime(t, r)["apps/web"]
	if got.Error != nil || len(got.Containers) != 200 || got.Containers[0].ImageIdentity.Reason != "inspection_failed" || len(got.Containers[0].ImageIdentity.RepositoryDigests) != 0 {
		t.Fatal("identity limit hid listing or exposed partial digests", got)
	}
}

func TestRuntimeLimitsPreserveWholeLastSuccess(t *testing.T) {
	for _, kind := range []string{"rows", "string", "bytes"} {
		t.Run(kind, func(t *testing.T) {
			r, l, _ := runtimeFixture("web")
			l.rows["web"] = []desiredstate.ContainerInfo{{ID: "last-success"}}
			old := readRuntime(t, r)["web"]
			rows := []desiredstate.ContainerInfo{{Name: strings.Repeat("x", runtimeStringLimit+1)}}
			if kind == "rows" {
				rows = make([]desiredstate.ContainerInfo, runtimeStackRowLimit+1)
			}
			if kind == "bytes" {
				rows = make([]desiredstate.ContainerInfo, runtimeStackRowLimit)
				for i := range rows {
					rows[i].Name = strings.Repeat("x", runtimeStringLimit)
				}
			}
			l.mu.Lock()
			l.rows["web"] = rows
			l.mu.Unlock()
			r.Invalidate()
			got := readRuntime(t, r)["web"]
			if got.Error == nil || *got.Error != errRuntimeLimit.Error() || len(got.Containers) != 1 || got.Containers[0].ID != "last-success" || !got.ObservedAt.Equal(*old.ObservedAt) || !got.AttemptedAt.After(old.AttemptedAt) {
				t.Fatalf("limit fabricated partial success or discarded last success: %+v", got)
			}
			if _, err := r.GetContainers(context.Background(), "web"); err == nil {
				t.Fatal("detail presented limited observation as complete")
			}
		})
	}
}

func TestRuntimeTotalRowAndByteBudgets(t *testing.T) {
	for _, kind := range []string{"rows", "bytes"} {
		t.Run(kind, func(t *testing.T) {
			paths := make([]string, 64)
			for i := range paths {
				paths[i] = fmt.Sprintf("stack-%02d", i)
			}
			r, l, _ := runtimeFixture(paths...)
			for _, path := range paths {
				rows := make([]desiredstate.ContainerInfo, runtimeStackRowLimit)
				if kind == "bytes" {
					rows = make([]desiredstate.ContainerInfo, 128)
				}
				for i := range rows {
					rows[i].ID = "short"
					if kind == "bytes" {
						rows[i].Name = strings.Repeat("<", 500)
					}
				}
				l.rows[path] = rows
			}
			got := readRuntime(t, r)
			rows, cost, dataCost, failures := 0, 0, 0, 0
			for path, observation := range got {
				key, _ := json.Marshal(path)
				rows += len(observation.Containers)
				cost += len(key) + runtimeEnvelopeReserve + runtimeRowsCost(observation.Containers)
				dataCost += runtimeRowsCost(observation.Containers)
				if observation.Error != nil {
					failures++
					if observation.ObservedAt != nil || len(observation.Containers) != 0 {
						t.Fatal("limit returned partial current rows as success")
					}
				}
			}
			encoded, _ := json.Marshal(got)
			if failures == 0 || rows > runtimeRowLimit || cost > runtimeByteBudget || dataCost > runtimeDataByteBudget || len(encoded) > runtimeByteBudget {
				t.Fatalf("budget rows=%d cost=%d wire=%d failures=%d", rows, cost, len(encoded), failures)
			}
			if kind == "bytes" && (dataCost < runtimeDataByteBudget-runtimeStackByteLimit || rows == runtimeRowLimit) {
				t.Fatalf("did not exercise aggregate byte cap: rows=%d bytes=%d", rows, cost)
			}
			if _, err := r.Read(context.Background()); err != nil || l.count() != len(paths) {
				t.Fatal("limit errors not coalesced/cached", err, l.count())
			}
		})
	}
}

func TestRuntimeScopeLimitsRejectBeforeCompose(t *testing.T) {
	for _, kind := range []string{"count", "path", "path-total"} {
		t.Run(kind, func(t *testing.T) {
			r, l, store := runtimeFixture()
			stacks := []desiredstate.StackRecord{{Path: strings.Repeat("x", runtimeStringLimit+1)}}
			if kind == "count" {
				stacks = make([]desiredstate.StackRecord, runtimeStackLimit+1)
			}
			if kind == "path-total" {
				stacks = make([]desiredstate.StackRecord, runtimePathBudget/1024+1)
				for i := range stacks {
					stacks[i].Path = fmt.Sprintf("%04d%s", i, strings.Repeat("x", 1020))
				}
			}
			store.Set(&desiredstate.Snapshot{Stacks: stacks})
			for i := 0; i < 3; i++ {
				if _, err := r.Read(context.Background()); !errors.Is(err, errRuntimeLimit) {
					t.Fatal("scope limit not rejected", err)
				}
			}
			if l.count() != 0 || len(r.cache) != 0 {
				t.Fatal("oversized scope reached Compose/cache")
			}
		})
	}
}

func TestRuntimeCachedSuccessBudgetSurvivesScopeGrowth(t *testing.T) {
	paths := make([]string, 20)
	for i := range paths {
		paths[i] = fmt.Sprintf("stack-%04d", i)
	}
	r, l, store := runtimeFixture(paths...)
	for _, path := range paths {
		rows := make([]desiredstate.ContainerInfo, 128)
		for i := range rows {
			rows[i].Name = strings.Repeat("<", 500)
		}
		l.rows[path] = rows
	}
	old := readRuntime(t, r)
	stacks := make([]desiredstate.StackRecord, runtimeStackLimit)
	for i := range stacks {
		stacks[i].Path = fmt.Sprintf("stack-%04d", i)
	}
	store.Set(&desiredstate.Snapshot{Stacks: stacks})
	now := readRuntime(t, r)
	for path, observation := range old {
		if observation.Error == nil && (now[path].Error != nil || !now[path].ObservedAt.Equal(*observation.ObservedAt)) {
			t.Fatal("new scopes discarded/reobserved cached success", path)
		}
	}
	encoded, _ := json.Marshal(now)
	data := 0
	for _, observation := range now {
		data += runtimeRowsCost(observation.Containers)
	}
	if len(now) != runtimeStackLimit || len(encoded) > runtimeByteBudget || data > runtimeDataByteBudget {
		t.Fatalf("scope growth exceeded budgets: scopes=%d wire=%d data=%d", len(now), len(encoded), data)
	}
}

// A shared 32MiB payload models expensive Compose content without needing a
// daemon. Cache-hit allocations must be independent of payload length, even for
// detail reads and with a large operation history (no step/message copies).
func TestRuntimeCacheHitNeverCopiesComposeContent(t *testing.T) {
	r, l, store := runtimeFixture("web")
	store.Set(&desiredstate.Snapshot{Stacks: []desiredstate.StackRecord{{Path: "web", Content: make([]byte, 32*1024*1024)}}})
	readRuntime(t, r)
	result := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := r.GetContainers(context.Background(), "web"); err != nil {
				b.Fatal(err)
			}
		}
	})
	if result.AllocedBytesPerOp() > 64*1024 || l.count() != 1 {
		t.Fatalf("cache copied payload/reobserved: %s %s calls=%d", result.String(), result.MemString(), l.count())
	}
	t.Logf("32MiB Compose cache hit: %s %s", result.String(), result.MemString())
}

func BenchmarkRuntimeCached250LargeCompose(b *testing.B) {
	paths := make([]string, 250)
	stacks := make([]desiredstate.StackRecord, 250)
	payload := make([]byte, 1024*1024)
	for i := range stacks {
		paths[i] = fmt.Sprintf("apps/%03d", i)
		stacks[i] = desiredstate.StackRecord{Path: paths[i], Content: payload}
	}
	r, _, store := runtimeFixture(paths...)
	store.Set(&desiredstate.Snapshot{Stacks: stacks})
	if _, err := r.Read(context.Background()); err != nil {
		b.Fatal(err)
	}
	r.ttl = time.Hour
	r.mu.Lock()
	for path := range r.expires {
		r.expires[path] = time.Now().Add(time.Hour)
	}
	r.mu.Unlock()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.Read(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}
