package events

import (
	"context"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
)

// ForwardToSSE connects domain state changes to the UI event stream.
func ForwardToSSE(bus *EventBus, broadcaster *desiredstate.Broadcaster, store *desiredstate.Store) {
	for _, eventType := range []string{EventTypeStackStatusChanged, EventTypeStackSynced, EventTypeContainersUpdated} {
		bus.Subscribe(eventType, func(_ context.Context, event Event) error {
			if broadcaster == nil {
				return nil
			}
			path, _ := event.Metadata()["stack_path"].(string)
			for _, stack := range store.GetStacks() {
				if stack.Path == path {
					broadcaster.PublishStackUpsert(stack)
					break
				}
			}
			return nil
		})
	}
}
