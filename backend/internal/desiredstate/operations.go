package desiredstate

import (
	"context"
	"github.com/lucasreiners/docker-cd/internal/operations"
)

// SetOperations is called during startup, before any publishers are started.
func (b *Broadcaster) SetOperations(store *operations.Store) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if store == nil {
		return
	}
	b.operations = store
	b.work.operations = make(map[string]WorkOperation)
	for _, r := range store.List() {
		if r.Status == "running" {
			b.operationChainLocked(r.ID)
		}
	}
}
func (b *Broadcaster) Operations() *operations.Store { return b.operations }
func (b *Broadcaster) BeginOperation(ctx context.Context, kind, trigger, stack, revision string) context.Context {
	if b == nil {
		return ctx
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.operations.Start(operations.ID(ctx), kind, trigger, stack, revision)
	b.publishOperationLocked(r)
	return operations.WithID(ctx, r.ID)
}
func (b *Broadcaster) OperationStep(ctx context.Context, phase, message string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if r, ok := b.operations.Update(operations.ID(ctx), phase, message, false); ok {
		b.publishOperationLocked(r)
	}
}
func (b *Broadcaster) EndOperation(ctx context.Context, status, message string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if r, ok := b.operations.Update(operations.ID(ctx), status, message, true); ok {
		b.publishOperationLocked(r)
	}
}
