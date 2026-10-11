package reconcile

import (
	"context"
	"fmt"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
)

// ReconcileReviewed applies exactly the revision the operator reviewed. Validation
// happens under the same lock as scheduled reconciliation, before any Docker work.
func (r *Reconciler) ReconcileReviewed(ctx context.Context, path, revision string, ack bool) ReconciliationRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	fail := func(message string) ReconciliationRun {
		return ReconciliationRun{StackPath: path, Result: "failed", Error: message}
	}
	if !r.policy.Enabled {
		return fail("Reconciliation is disabled")
	}
	snap := r.store.Get()
	if snap == nil || snap.RefreshStatus != desiredstate.RefreshStatusCompleted || snap.UpdatesBlocked {
		return fail("Repository refresh must complete before deployment")
	}
	if snap.Revision != revision {
		return fail("Desired revision changed; review the stack again")
	}
	var stack *desiredstate.StackRecord
	for i := range snap.Stacks {
		if snap.Stacks[i].Path == path {
			stack = &snap.Stacks[i]
			break
		}
	}
	if stack == nil {
		return fail("Stack not found")
	}
	if stack.RequiresAcknowledgement && !ack {
		return fail("This stack requires drift acknowledgement")
	}
	if r.policy.DriftPolicy == "flag" && stack.ComposeHash != stack.SyncedComposeHash && !ack {
		return fail("This change requires acknowledgement")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	r.cancelMu.Lock()
	r.cancelActive = cancel
	r.cancelMu.Unlock()
	defer func() { r.cancelMu.Lock(); r.cancelActive = nil; r.cancelMu.Unlock() }()
	return r.syncStack(ctx, DriftResult{Path: path, NeedSync: true, Reason: fmt.Sprintf("operator action at %s", revision)}, snap)
}

func (sm *StateManager) MarkAwaitingAcknowledgement(path, reason string) {
	sm.UpdateStatus(path, desiredstate.StackSyncFailed, "", "drift detected, awaiting acknowledgement")
	snap := sm.store.Get()
	if snap == nil {
		return
	}
	for i := range snap.Stacks {
		if snap.Stacks[i].Path == path {
			snap.Stacks[i].RequiresAcknowledgement = true
			snap.Stacks[i].StatusReason = reason
		}
	}
	sm.store.Set(snap)
}
