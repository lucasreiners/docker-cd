package events

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Event represents a domain event in the system.
type Event interface {
	// EventType returns the type identifier for this event.
	EventType() string

	// OccurredAt returns when the event occurred.
	OccurredAt() time.Time

	// Metadata returns additional event metadata.
	Metadata() map[string]any
}

// Handler is a function that processes events.
type Handler func(ctx context.Context, event Event) error

// EventBus coordinates event publishing and subscription.
type EventBus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	logger   *slog.Logger
}

// NewEventBus creates a new event bus.
func NewEventBus(logger *slog.Logger) *EventBus {
	return &EventBus{
		handlers: make(map[string][]Handler),
		logger:   logger,
	}
}

// Subscribe registers a handler for a specific event type.
func (b *EventBus) Subscribe(eventType string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[eventType] = append(b.handlers[eventType], handler)
	b.logger.Debug("event handler subscribed", "event_type", eventType)
}

// Publish dispatches an event to all registered handlers.
func (b *EventBus) Publish(ctx context.Context, event Event) {
	b.mu.RLock()
	handlers := b.handlers[event.EventType()]
	b.mu.RUnlock()

	if len(handlers) == 0 {
		return
	}

	b.logger.Debug("publishing event",
		"event_type", event.EventType(),
		"handler_count", len(handlers))

	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			b.logger.Error("event handler failed",
				"event_type", event.EventType(),
				"error", err)
		}
	}
}
