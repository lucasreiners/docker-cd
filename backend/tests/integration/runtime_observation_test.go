//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	gitreader "github.com/lucasreiners/docker-cd/internal/git"
	handler "github.com/lucasreiners/docker-cd/internal/http"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/tests/integration/dind"
)

type runtimeComposeMetrics struct {
	Calls, Errors, Timeouts, MaxConcurrency int
	Total, Longest                          time.Duration
}

// Measured commands must explicitly target the disposable daemon. Neither an
// accidental host fallback nor a Compose mutation is permitted by this runner.
type measuredRuntimeCompose struct {
	host    string
	mu      sync.Mutex
	metrics runtimeComposeMetrics
	active  int
}

func (r *measuredRuntimeCompose) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name != "docker" || len(args) != 10 || args[0] != "-H" || args[1] != r.host || args[2] != "compose" || args[3] != "-p" || args[5] != "ps" || args[6] != "-a" || args[7] != "--no-trunc" || args[8] != "--format" || args[9] != "json" {
		return nil, errors.New("measurement rejected a non-isolated or non-read-only command")
	}
	r.mu.Lock()
	r.active++
	r.metrics.Calls++
	r.metrics.MaxConcurrency = max(r.metrics.MaxConcurrency, r.active)
	r.mu.Unlock()
	start := time.Now()
	out, err := (&docker.ExecRunner{}).Run(ctx, name, args...)
	elapsed := time.Since(start)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active--
	r.metrics.Total += elapsed
	r.metrics.Longest = max(r.metrics.Longest, elapsed)
	if err != nil {
		r.metrics.Errors++
	}
	if ctx.Err() != nil {
		r.metrics.Timeouts++
	}
	return out, err
}

func (r *measuredRuntimeCompose) snapshot() runtimeComposeMetrics {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.metrics
}

func runtimeDiscoveredStacks(t *testing.T) []desiredstate.StackRecord {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote")
	repo, err := gogit.PlainInitWithOptions(remote, &gogit.PlainInitOptions{InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}})
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 250; i++ {
		path := fmt.Sprintf("apps/stack-%03d/compose.yaml", i)
		full := filepath.Join(remote, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		// Valid services, intentionally never pulled, created, or started.
		if err := os.WriteFile(full, []byte("services:\n  web:\n    image: busybox:latest\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Add(path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := wt.Commit("250 observation-only stack fixtures", &gogit.CommitOptions{Author: &object.Signature{Name: "Runtime Measurement", Email: "test@example.invalid", When: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	reader := gitreader.NewLocalComposeReader(gitreader.NewLocalClone(filepath.Join(root, "clone"), remote, "", "main"))
	entries, _, _, err := reader.ReadComposeFiles(context.Background(), remote, "", "main", "")
	if err != nil {
		t.Fatal(err)
	}
	stacks := make([]desiredstate.StackRecord, 0, len(entries))
	for _, entry := range entries {
		stacks = append(stacks, desiredstate.StackRecord{Path: entry.StackPath, ComposeFile: entry.ComposeFile, ComposeHash: desiredstate.ComposeHash(entry.Content), Content: entry.Content})
	}
	if len(stacks) != 250 {
		t.Fatalf("discovered %d stacks, want 250", len(stacks))
	}
	return stacks
}

func TestDinD_RuntimeObservation250Stacks(t *testing.T) {
	env := dind.StartT(t)
	api, err := dind.NewSDKClient(env.DockerHost)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = api.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	before, err := api.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("disposable daemon is not empty: %d containers", len(before))
	}
	version, err := (&docker.ExecRunner{}).Run(ctx, "docker", "-H", env.DockerHost, "version", "--format", "{{.Client.Version}} / {{.Server.Version}}")
	if err != nil {
		t.Fatal(err)
	}
	composeVersion, err := (&docker.ExecRunner{}).Run(ctx, "docker", "-H", env.DockerHost, "compose", "version", "--short")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("isolated daemon=%s Docker client/server=%s Compose=%s actual application containers=0", env.Container.GetContainerID()[:12], strings.TrimSpace(string(version)), strings.TrimSpace(string(composeVersion)))
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Stacks: runtimeDiscoveredStacks(t)})
	commands := &measuredRuntimeCompose{host: env.DockerHost}
	compose := reconcile.NewDockerComposeRunner(commands, env.DockerHost)
	lister := reconcile.NewReconciler(store, reconcile.ReconciliationPolicy{}, compose, nil, nil, "", nil, nil, nil, nil, nil)
	runtime := handler.NewRuntimeReader(store, lister, api, nil)
	measure := func(phase string) map[string]handler.RuntimeObservation {
		t.Helper()
		prior := commands.snapshot()
		start := time.Now()
		observations, err := runtime.Read(ctx)
		elapsed := time.Since(start)
		metrics := commands.snapshot()
		successes, failures, timeouts, containers := 0, 0, 0, 0
		for _, observation := range observations {
			if observation.Error != nil {
				failures++
				if strings.Contains(*observation.Error, "timed out") {
					timeouts++
				}
			} else if observation.ObservedAt != nil {
				successes++
			}
			containers += len(observation.Containers)
		}
		var earliest, latest time.Time
		for _, observation := range observations {
			if observation.ObservedAt == nil {
				continue
			}
			if earliest.IsZero() || observation.ObservedAt.Before(earliest) {
				earliest = *observation.ObservedAt
			}
			if latest.IsZero() || observation.ObservedAt.After(latest) {
				latest = *observation.ObservedAt
			}
		}
		t.Logf("%s elapsed=%s observations=%d successful=%d observationErrors=%d observationTimeouts=%d actualContainers=%d ComposeCalls=%d commandErrors=%d commandTimeouts=%d maxConcurrency=%d cumulativeLongestCommand=%s cumulativeCommandTime=%s readError=%v", phase, elapsed, len(observations), successes, failures, timeouts, containers, metrics.Calls-prior.Calls, metrics.Errors-prior.Errors, metrics.Timeouts-prior.Timeouts, metrics.MaxConcurrency, metrics.Longest, metrics.Total, err)
		if !earliest.IsZero() {
			t.Logf("%s listingWindow=%s oldestObservationAgeAtReturn=%s", phase, latest.Sub(earliest), time.Since(earliest))
		}
		if err != nil {
			t.Fatalf("%s: %v", phase, err)
		}
		if len(observations) != 250 || successes != 250 || failures != 0 || containers != 0 {
			t.Errorf("%s did not successfully observe all 250 empty stacks", phase)
		}
		if phase != "immediate-cache-hit" && (metrics.Calls-prior.Calls != 250 || elapsed >= 20*time.Second) {
			t.Errorf("%s did not complete 250 real Compose calls within the unchanged 20-second cycle budget", phase)
		}
		return observations
	}
	cold := measure("cold")
	prior := commands.snapshot()
	cached := measure("immediate-cache-hit")
	if commands.snapshot().Calls != prior.Calls {
		t.Error("cache hit started Compose subprocesses")
	}
	for path, observation := range cold {
		if observation.ObservedAt != nil && (cached[path].ObservedAt == nil || !cached[path].ObservedAt.Equal(*observation.ObservedAt)) {
			t.Error("cache hit fabricated observation time")
		}
	}
	detailStart := time.Now()
	if _, err := runtime.GetContainers(ctx, "apps/stack-000"); err != nil {
		t.Errorf("cached detail: %v", err)
	}
	t.Logf("targeted-detail-cache-hit elapsed=%s ComposeCalls=%d", time.Since(detailStart), commands.snapshot().Calls-prior.Calls)
	if commands.snapshot().Calls != prior.Calls {
		t.Error("detail did not share runtime cache")
	}
	// Exercise the production 15-second TTL, not a shortened test-only TTL.
	time.Sleep(15*time.Second + 100*time.Millisecond)
	refreshed := measure("after-15s-TTL")
	var minInterval, maxInterval time.Duration
	for path, observation := range refreshed {
		if previous := cold[path]; previous.ObservedAt != nil && (observation.ObservedAt == nil || !observation.ObservedAt.After(*previous.ObservedAt)) {
			t.Error("TTL refresh reused a stale success")
		}
		if previous := cold[path]; previous.ObservedAt != nil && observation.ObservedAt != nil {
			interval := observation.ObservedAt.Sub(*previous.ObservedAt)
			if minInterval == 0 || interval < minInterval {
				minInterval = interval
			}
			maxInterval = max(maxInterval, interval)
		}
	}
	t.Logf("same-stack successful observation intervals min=%s max=%s (production TTL is 15s after cycle publication, not a fixed 15s start-to-start cadence)", minInterval, maxInterval)
	after, err := api.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatal("read-only measurement created application containers")
	}
}
