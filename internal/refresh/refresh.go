package refresh

import (
	"context"
	"log"
	"time"

	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/git"
)

// ReconcileFunc is a callback invoked after each successful refresh to trigger reconciliation.
type ReconcileFunc func(ctx context.Context)

// Service orchestrates desired-state refreshes from Git.
type Service struct {
	cfg        config.Config
	store      *desiredstate.Store
	queue      *Queue
	reader     git.ComposeReader
	reconcileF ReconcileFunc
}

// NewService creates a refresh service.
func NewService(cfg config.Config, store *desiredstate.Store, queue *Queue, reader git.ComposeReader) *Service {
	return &Service{
		cfg:    cfg,
		store:  store,
		queue:  queue,
		reader: reader,
	}
}

// SetReconcileFunc sets the optional callback that runs after each successful refresh.
func (s *Service) SetReconcileFunc(f ReconcileFunc) {
	s.reconcileF = f
}

// Start begins the refresh loop: listens for triggers from the queue and
// performs refreshes. Also starts the periodic poll ticker if configured.
// Blocks until ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	// Trigger initial startup refresh
	s.queue.Enqueue(Trigger{
		Source:      TriggerStartup,
		RequestedAt: time.Now(),
	})

	// Start periodic polling if configured
	if s.cfg.RefreshPollInterval > 0 {
		go s.pollLoop(ctx)
	}

	// Main refresh loop
	for {
		select {
		case <-ctx.Done():
			return
		case trigger := <-s.queue.TriggerChan():
			s.doRefresh(ctx, trigger)
		}
	}
}

// RequestRefresh enqueues a refresh trigger and returns the queue result.
func (s *Service) RequestRefresh(source TriggerSource) QueueResult {
	return s.queue.Enqueue(Trigger{
		Source:      source,
		RequestedAt: time.Now(),
	})
}

func (s *Service) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.RefreshPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Printf("[info] periodic refresh triggered (interval: %s)", s.cfg.RefreshPollInterval)
			s.queue.Enqueue(Trigger{
				Source:      TriggerPeriodic,
				RequestedAt: time.Now(),
			})
		}
	}
}

func (s *Service) doRefresh(ctx context.Context, trigger Trigger) {
	log.Printf("[info] starting refresh (source: %s)", trigger.Source)

	s.store.UpdateStatus(desiredstate.RefreshStatusRefreshing, "")

	entries, commitHash, commitMessage, err := s.reader.ReadComposeFiles(
		ctx,
		s.cfg.GitRepoURL,
		s.cfg.GitAccessToken,
		s.cfg.GitRevision,
		s.cfg.GitDeployDir,
	)

	if err != nil {
		log.Printf("[error] refresh failed: %v", err)
		s.store.UpdateStatus(desiredstate.RefreshStatusFailed, err.Error())
		s.queue.Done()
		return
	}

	newStacks := s.buildStacksPreservingStatus(entries)

	snap := &desiredstate.Snapshot{
		Revision:      commitHash,
		CommitMessage: commitMessage,
		Ref:           s.cfg.GitRevision,
		RefType:       "branch",
		RefreshedAt:   time.Now(),
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		RefreshError:  "",
		Stacks:        newStacks,
	}

	s.store.Set(snap)
	log.Printf("[info] refresh completed: %d stacks at %s", len(newStacks), truncate(commitHash, 12))

	// Trigger reconciliation after successful refresh (FR-001, FR-002)
	if s.reconcileF != nil {
		log.Printf("[info] triggering reconciliation after refresh")
		s.reconcileF(ctx)
	}

	s.queue.Done()
}

// buildStacksPreservingStatus creates StackRecords from Git entries,
// preserving the sync status of stacks that already exist with the same hash.
func (s *Service) buildStacksPreservingStatus(entries []git.ComposeEntry) []desiredstate.StackRecord {
	existingStacks := s.store.GetStacks()
	existing := make(map[string]desiredstate.StackRecord, len(existingStacks))
	for _, st := range existingStacks {
		existing[st.Path] = st
	}

	newStacks := make([]desiredstate.StackRecord, 0, len(entries))
	for _, e := range entries {
		hash := desiredstate.ComposeHash(e.Content)

		status := desiredstate.StackSyncMissing
		rec := desiredstate.StackRecord{}
		if prev, ok := existing[e.StackPath]; ok && prev.ComposeHash == hash {
			status = prev.Status
			log.Printf("[debug] buildStacksPreservingStatus: stack=%s status=%s (preserved, hash match)", e.StackPath, status)
			// Preserve sync metadata from previous state
			rec.SyncedRevision = prev.SyncedRevision
			rec.SyncedCommitMessage = prev.SyncedCommitMessage
			rec.SyncedComposeHash = prev.SyncedComposeHash
			rec.SyncedAt = prev.SyncedAt
			rec.LastSyncAt = prev.LastSyncAt
			rec.LastSyncStatus = prev.LastSyncStatus
			rec.LastSyncError = prev.LastSyncError
		} else if prev, ok := existing[e.StackPath]; ok {
			log.Printf("[debug] buildStacksPreservingStatus: stack=%s status=missing (hash changed: prev=%s new=%s)", e.StackPath, truncate(prev.ComposeHash, 12), truncate(hash, 12))
		} else {
			log.Printf("[debug] buildStacksPreservingStatus: stack=%s status=missing (new stack)", e.StackPath)
		}

		rec.Path = e.StackPath
		rec.ComposeFile = e.ComposeFile
		rec.ComposeHash = hash
		rec.Status = status
		rec.Content = e.Content

		newStacks = append(newStacks, rec)
	}

	return newStacks
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
