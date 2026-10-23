package desiredstate

import (
	"crypto/sha256"
	"encoding/json"
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
	// FullID and ComposeProject are server-enumerated scope, never browser inputs.
	FullID         string         `json:"-"`
	ComposeProject string         `json:"-"`
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Service        string         `json:"service"`
	State          string         `json:"state"`  // running, exited, paused, restarting, dead, created
	Health         string         `json:"health"` // healthy, unhealthy, starting, none
	Image          string         `json:"image"`
	Ports          string         `json:"ports,omitempty"`
	ImageIdentity  *ImageIdentity `json:"imageIdentity,omitempty"`
}

// ImageIdentity contains only local, whitelisted identity metadata, not raw inspect data.
type ImageIdentity struct {
	Status                    string             `json:"status"`
	ObservedAt                time.Time          `json:"observedAt"`
	Source                    string             `json:"source"`
	ImageID                   string             `json:"imageId,omitempty"`
	Reference                 string             `json:"reference"`
	Platform                  *ImagePlatform     `json:"platform,omitempty"`
	RepositoryDigests         []RepositoryDigest `json:"repositoryDigests"`
	PreferredRepositoryDigest *RepositoryDigest  `json:"preferredRepositoryDigest,omitempty"`
	Reason                    string             `json:"reason,omitempty"`
	Freshness                 string             `json:"freshness"`
}

type ImagePlatform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Variant      string `json:"variant,omitempty"`
}

type RepositoryDigest struct {
	Repository string `json:"repository"`
	Digest     string `json:"digest"`
	Kind       string `json:"kind"`
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
	SyncedRevision          string `json:"syncedRevision,omitempty"`
	SyncedCommitMessage     string `json:"syncedCommitMessage,omitempty"`
	SyncedComposeHash       string `json:"syncedComposeHash,omitempty"`
	SyncedAt                string `json:"syncedAt,omitempty"`
	LastSyncAt              string `json:"lastSyncAt,omitempty"`
	LastSyncStatus          string `json:"lastSyncStatus,omitempty"`
	LastSyncError           string `json:"lastSyncError,omitempty"`
	StatusReason            string `json:"statusReason,omitempty"`
	RequiresAcknowledgement bool   `json:"requiresAcknowledgement"`
	DesiredRevision         string `json:"desiredRevision,omitempty"`
}

// Snapshot represents the latest desired state loaded from Git.
type Snapshot struct {
	Revision       string        `json:"revision"`
	CommitMessage  string        `json:"commitMessage,omitempty"`
	Ref            string        `json:"ref"`
	RefType        string        `json:"refType"`
	RefreshedAt    time.Time     `json:"refreshedAt"`
	RefreshStatus  RefreshStatus `json:"refreshStatus"`
	RefreshError   string        `json:"refreshError,omitempty"`
	UpdatesBlocked bool          `json:"updatesBlocked"`
	BlockedReason  string        `json:"blockedReason,omitempty"`
	LocalPath      string        `json:"localPath,omitempty"`
	Stacks         []StackRecord `json:"stacks"`
}

// Store provides thread-safe access to the desired state snapshot.
type Store struct {
	mu               sync.RWMutex
	snapshot         *Snapshot
	runtimeRevision  uint64
	runtimeStacks    []RuntimeStack
	runtimePathBytes int
	runtimeMaxPath   int
}

// RuntimeStack is immutable observation scope metadata, never Compose payload.
type RuntimeStack struct {
	Path      string
	Signature [sha256.Size]byte
}

// RuntimeStacks only allocates after a desired-state replacement. Cache hits
// check a scalar under the store lock and never copy/hash Compose content.
func (s *Store) RuntimeStacks(since uint64, maxStacks, maxPathBytes, maxPathLength int) (uint64, []RuntimeStack, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.runtimeStacks) > maxStacks || s.runtimePathBytes > maxPathBytes || s.runtimeMaxPath > maxPathLength {
		return s.runtimeRevision, nil, false
	}
	if since == s.runtimeRevision {
		return s.runtimeRevision, nil, true
	}
	return s.runtimeRevision, append([]RuntimeStack{}, s.runtimeStacks...), true
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
	s.runtimeRevision++
	s.runtimeStacks = nil
	s.runtimePathBytes, s.runtimeMaxPath = 0, 0
	if snap != nil {
		s.runtimeStacks = make([]RuntimeStack, 0, len(snap.Stacks))
		for _, stack := range snap.Stacks {
			s.runtimePathBytes += len(stack.Path)
			s.runtimeMaxPath = max(s.runtimeMaxPath, len(stack.Path))
			// Content is excluded by the StackRecord JSON contract. No payload
			// copy occurs, and hashing happens once at write time, not per reader.
			encoded, _ := json.Marshal(stack)
			s.runtimeStacks = append(s.runtimeStacks, RuntimeStack{stack.Path, sha256.Sum256(encoded)})
		}
	}
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

// SetRefreshState updates refresh status alongside blocking metadata.
func (s *Store) SetRefreshState(status RefreshStatus, refreshErr string, updatesBlocked bool, blockedReason, localPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot == nil {
		s.snapshot = &Snapshot{}
	}
	s.snapshot.RefreshStatus = status
	s.snapshot.RefreshError = refreshErr
	s.snapshot.UpdatesBlocked = updatesBlocked
	s.snapshot.BlockedReason = blockedReason
	s.snapshot.LocalPath = localPath
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
		Revision:       s.snapshot.Revision,
		CommitMessage:  s.snapshot.CommitMessage,
		Ref:            s.snapshot.Ref,
		RefType:        s.snapshot.RefType,
		RefreshedAt:    s.snapshot.RefreshedAt,
		RefreshStatus:  s.snapshot.RefreshStatus,
		RefreshError:   s.snapshot.RefreshError,
		UpdatesBlocked: s.snapshot.UpdatesBlocked,
		BlockedReason:  s.snapshot.BlockedReason,
		LocalPath:      s.snapshot.LocalPath,
	}
}
