package desiredstate

import (
	"context"
	"github.com/lucasreiners/docker-cd/internal/operations"
)

// SetOperations is called during startup, before any publishers are started.
func (b *Broadcaster) SetOperations(store *operations.Store) { b.operations = store }
func (b *Broadcaster) Operations() *operations.Store         { return b.operations }
func (b *Broadcaster) BeginOperation(ctx context.Context, kind, trigger, stack, revision string) context.Context {
	if b == nil {
		return ctx
	}
	r := b.operations.Start(operations.ID(ctx), kind, trigger, stack, revision)
	b.Publish(EventType("operation.upsert"), r)
	return operations.WithID(ctx, r.ID)
}
func (b *Broadcaster) OperationStep(ctx context.Context, phase, message string) {
	if b == nil {
		return
	}
	if r, ok := b.operations.Update(operations.ID(ctx), phase, message, false); ok {
		b.Publish(EventType("operation.upsert"), r)
	}
}
func (b *Broadcaster) EndOperation(ctx context.Context, status, message string) {
	if b == nil {
		return
	}
	if r, ok := b.operations.Update(operations.ID(ctx), status, message, true); ok {
		b.Publish(EventType("operation.upsert"), r)
	}
}
