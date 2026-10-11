package reconcile_test

import (
	"context"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"log/slog"
	"testing"
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
