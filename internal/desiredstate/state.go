package desiredstate

import (
	"sync"
	"time"
)

// RefreshStatus represents the system-wide Git refresh status.
type RefreshStatus string

const (
	RefreshStatusRefreshing RefreshStatus = "refreshing"
	RefreshStatusQueued     RefreshStatus = "queued"
	RefreshStatusCompleted  RefreshStatus = "completed"
	RefreshStatusFailed     RefreshStatus = "failed"
)

// StackSyncStatus represents the per-stack sync status.
type StackSyncStatus string

const (
	StackSyncMissing  StackSyncStatus = "missing"
	StackSyncSyncing  StackSyncStatus = "syncing"
	StackSyncSynced   StackSyncStatus = "synced"
	StackSyncDeleting StackSyncStatus = "deleting"
	StackSyncFailed   StackSyncStatus = "failed"
)

// ContainerInfo describes a single container within a stack.
type ContainerInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Service string `json:"service"`
	State   string `json:"state"`  // running, exited, paused, restarting, dead, created
	Health  string `json:"health"` // healthy, unhealthy, starting, none
	Image   string `json:"image"`
	Ports   string `json:"ports,omitempty"`
}

// StackRecord represents a stack discovered in the repository.
type StackRecord struct {
	Path        string          `json:"path"`
	ComposeFile string          `json:"composeFile"`
	ComposeHash string          `json:"composeHash"`
	Status      StackSyncStatus `json:"status"`
	Content     []byte          `json:"-"` // raw compose file content, not exposed via API

	// Container summary
	ContainersRunning int `json:"containersRunning"`
	ContainersTotal   int `json:"containersTotal"`

	// Sync metadata (populated after reconciliation)
	SyncedRevision      string `json:"syncedRevision,omitempty"`
	SyncedCommitMessage string `json:"syncedCommitMessage,omitempty"`
	SyncedComposeHash   string `json:"syncedComposeHash,omitempty"`
	SyncedAt            string `json:"syncedAt,omitempty"`
	LastSyncAt          string `json:"lastSyncAt,omitempty"`
	LastSyncStatus      string `json:"lastSyncStatus,omitempty"`
	LastSyncError       string `json:"lastSyncError,omitempty"`
}

// Snapshot represents the latest desired state loaded from Git.
type Snapshot struct {
	Revision      string        `json:"revision"`
	CommitMessage string        `json:"commitMessage,omitempty"`
	Ref           string        `json:"ref"`
	RefType       string        `json:"refType"`
	RefreshedAt   time.Time     `json:"refreshedAt"`
	RefreshStatus RefreshStatus `json:"refreshStatus"`
	RefreshError  string        `json:"refreshError,omitempty"`
	Stacks        []StackRecord `json:"stacks"`
}

// Store provides thread-safe access to the desired state snapshot.
type Store struct {
	mu       sync.RWMutex
	snapshot *Snapshot
}

// NewStore creates an empty Store.
func NewStore() *Store {
	return &Store{}
}

// Get returns a copy of the current snapshot, or nil if no snapshot exists.
func (s *Store) Get() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.snapshot == nil {
		return nil
	}
	cp := *s.snapshot
	cp.Stacks = make([]StackRecord, len(s.snapshot.Stacks))
	for i, st := range s.snapshot.Stacks {
		cp.Stacks[i] = st
		if st.Content != nil {
			cp.Stacks[i].Content = make([]byte, len(st.Content))
			copy(cp.Stacks[i].Content, st.Content)
		}
	}
	return &cp
}

// Set replaces the current snapshot.
func (s *Store) Set(snap *Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = snap
}

// UpdateStatus updates the refresh status and optionally the error message.
func (s *Store) UpdateStatus(status RefreshStatus, refreshErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot == nil {
		s.snapshot = &Snapshot{}
	}
	s.snapshot.RefreshStatus = status
	s.snapshot.RefreshError = refreshErr
}

// GetStacks returns a copy of the current stacks, or nil if no snapshot exists.
func (s *Store) GetStacks() []StackRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.snapshot == nil {
		return nil
	}
	stacks := make([]StackRecord, len(s.snapshot.Stacks))
	for i, st := range s.snapshot.Stacks {
		stacks[i] = st
		if st.Content != nil {
			stacks[i].Content = make([]byte, len(st.Content))
			copy(stacks[i].Content, st.Content)
		}
	}
	return stacks
}

// GetRefreshStatus returns the refresh status fields (without stacks).
func (s *Store) GetRefreshStatus() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.snapshot == nil {
		return nil
	}
	return &Snapshot{
		Revision:      s.snapshot.Revision,
		CommitMessage: s.snapshot.CommitMessage,
		Ref:           s.snapshot.Ref,
		RefType:       s.snapshot.RefType,
		RefreshedAt:   s.snapshot.RefreshedAt,
		RefreshStatus: s.snapshot.RefreshStatus,
		RefreshError:  s.snapshot.RefreshError,
	}
}
