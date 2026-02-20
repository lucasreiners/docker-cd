package reconcile

import "sync"

// AckStore tracks operator acknowledgements for stacks under "flag" drift policy.
type AckStore struct {
	mu   sync.RWMutex
	acks map[string]bool
}

// NewAckStore creates an empty acknowledgement store.
func NewAckStore() *AckStore {
	return &AckStore{
		acks: make(map[string]bool),
	}
}

// Acknowledge records an operator acknowledgement for a stack path.
func (a *AckStore) Acknowledge(path string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.acks[path] = true
}

// IsAcknowledged returns whether a stack path has been acknowledged.
func (a *AckStore) IsAcknowledged(path string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.acks[path]
}

// Clear removes an acknowledgement for a stack path.
func (a *AckStore) Clear(path string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.acks, path)
}
