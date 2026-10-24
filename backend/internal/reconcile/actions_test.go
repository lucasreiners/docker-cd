package reconcile_test

import (
	"context"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestReviewedActionsEnforcePolicyRevisionAndScope(t *testing.T) {
	for _, tc := range []struct {
		name, revision        string
		enabled, blocked, ack bool
		want                  bool
	}{
		{"approved current revision", "current", true, false, true, true},
		{"stale revision", "old", true, false, true, false},
		{"missing acknowledgement", "current", true, false, false, false},
		{"reconcile disabled", "current", false, false, true, false},
		{"refresh blocked", "current", true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := desiredstate.NewStore()
			store.Set(&desiredstate.Snapshot{Revision: "current", RefreshStatus: desiredstate.RefreshStatusCompleted, UpdatesBlocked: tc.blocked, Stacks: []desiredstate.StackRecord{
				{Path: "nested/web", ComposeFile: "compose.yaml", ComposeHash: "new", SyncedComposeHash: "old", RequiresAcknowledgement: true, Content: []byte("services:\n  web:\n    image: nginx\n")},
				{Path: "other", ComposeFile: "compose.yaml", ComposeHash: "new", RequiresAcknowledgement: true},
			}})
			compose := &stubComposeRunner{}
			b := desiredstate.NewBroadcaster()
			r := reconcile.NewReconciler(store, reconcile.ReconciliationPolicy{Enabled: tc.enabled, DriftPolicy: "flag"}, compose, nil, reconcile.NewAckStore(), "", nil, newTestStateManager(store, compose), slog.Default(), nil, b)
			run := r.ReconcileReviewed(context.Background(), "nested/web", tc.revision, tc.ack)
			if (run.Result == "success") != tc.want {
				t.Fatalf("unexpected result: %+v", run)
			}
			if tc.want {
				if len(compose.upCalls) != 1 || compose.upCalls[0].WorkDir != "nested/web" {
					t.Fatalf("wrong scope: %+v", compose.upCalls)
				}
				if !store.GetStacks()[1].RequiresAcknowledgement {
					t.Fatal("other stack approved")
				}
				if store.GetStacks()[0].RequiresAcknowledgement {
					t.Fatal("approval not cleared")
				}
			} else if len(compose.upCalls) != 0 {
				t.Fatal("rejected action modified runtime")
			}
		})
	}
}

// Hold the real reconciliation lock in an earlier workflow. The second action
// must validate its frozen reviewed revision after acquiring that lock, not just
// when the HTTP request was accepted.
type reviewedLockCompose struct {
	stubComposeRunner
	entered chan struct{}
	release chan struct{}
}

func (c *reviewedLockCompose) ComposeUp(ctx context.Context, project, file, override, dir string) error {
	close(c.entered)
	select {
	case <-c.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return c.stubComposeRunner.ComposeUp(ctx, project, file, override, dir)
}

func TestReviewedActionRejectsRevisionChangedWhileWaitingForExecutionLock(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Revision: "reviewed", RefreshStatus: desiredstate.RefreshStatusCompleted, Stacks: []desiredstate.StackRecord{
		{Path: "nested/web", ComposeFile: "compose.yaml", ComposeHash: "same", SyncedComposeHash: "same", Content: []byte("services:\n  web:\n    image: nginx\n")},
	}})
	compose := &reviewedLockCompose{entered: make(chan struct{}), release: make(chan struct{})}
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), compose, nil, reconcile.NewAckStore(), "", nil, newTestStateManager(store, compose), slog.Default(), nil, desiredstate.NewBroadcaster())
	first := make(chan reconcile.ReconciliationRun, 1)
	go func() { first <- r.ReconcileReviewed(context.Background(), "nested/web", "reviewed", false) }()
	select {
	case <-compose.entered:
	case <-time.After(time.Second):
		t.Fatal("first workflow never acquired execution lock")
	}
	second := make(chan reconcile.ReconciliationRun, 1)
	attempted := make(chan struct{})
	go func() {
		close(attempted)
		second <- r.ReconcileReviewed(context.Background(), "nested/web", "reviewed", true)
	}()
	<-attempted
	select {
	case <-second:
		t.Fatal("queued action bypassed held execution lock")
	default:
	}
	newer := store.Get()
	newer.Revision = "new-unreviewed-revision"
	store.Set(newer)
	close(compose.release)
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("first action did not finish")
	}
	select {
	case result := <-second:
		if result.Result != "failed" || !strings.Contains(result.Error, "Desired revision changed") {
			t.Fatalf("queued approval adopted a newer revision: %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("queued action did not finish")
	}
	if len(compose.upCalls) != 1 || len(compose.downCalls) != 0 {
		t.Fatalf("rejected queued action mutated runtime: up=%v down=%v", compose.upCalls, compose.downCalls)
	}
}

type reviewedScopePuller struct {
	stubImagePuller
	pruneCalls int
}

func (p *reviewedScopePuller) PruneImages(context.Context) (int, string, error) {
	p.pruneCalls++
	return 0, "0B", nil
}

func TestReviewedDeploymentPullsOnlyTargetImagesWithoutPruneOrStackRemoval(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Revision: "reviewed", RefreshStatus: desiredstate.RefreshStatusCompleted, Stacks: []desiredstate.StackRecord{
		{Path: "nested/web", ComposeFile: "compose.yaml", ComposeHash: "same", SyncedComposeHash: "same", Content: []byte("services:\n  web:\n    image: nginx:reviewed\n")},
		{Path: "other", ComposeFile: "compose.yaml", Content: []byte("services:\n  other:\n    image: postgres:other\n")},
	}})
	compose := &stubComposeRunner{}
	puller := &reviewedScopePuller{}
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), compose, nil, reconcile.NewAckStore(), "", nil, newTestStateManager(store, compose), slog.Default(), puller, desiredstate.NewBroadcaster())
	result := r.ReconcileReviewed(context.Background(), "nested/web", "reviewed", false)
	if result.Result != "success" {
		t.Fatalf("reviewed action failed: %+v", result)
	}
	if len(puller.pullCalls) != 1 || len(puller.pullCalls[0]) != 1 || puller.pullCalls[0][0] != "nginx:reviewed" {
		t.Fatalf("targeted action pulled outside target: %v", puller.pullCalls)
	}
	if len(compose.upCalls) != 1 || compose.upCalls[0].WorkDir != "nested/web" || len(compose.downCalls) != 0 {
		t.Fatalf("targeted action changed other resources: %+v", compose)
	}
	if puller.pruneCalls != 0 {
		t.Fatal("single-stack action performed a host-wide image prune")
	}
}
