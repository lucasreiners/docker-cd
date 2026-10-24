package refresh

import (
	"context"
	"log"
	"time"

	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/git"
	"github.com/lucasreiners/docker-cd/internal/operations"
)

// ReconcileFunc is a callback invoked after each successful refresh to trigger reconciliation.
type ReconcileFunc func(ctx context.Context)

// Service orchestrates desired-state refreshes from Git.
type Service struct {
	cfg         config.Config
	store       *desiredstate.Store
	queue       *Queue
	reader      git.ComposeReader
	reconcileF  ReconcileFunc
	broadcaster *desiredstate.Broadcaster
	cancelRecon func()
	cancelUpd   func()
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

// SetBroadcaster sets the SSE broadcaster for publishing state changes.
func (s *Service) SetBroadcaster(b *desiredstate.Broadcaster) {
	s.broadcaster = b
}

// SetReconcileFunc sets the optional callback that runs after each successful refresh.
func (s *Service) SetReconcileFunc(f ReconcileFunc) {
	s.reconcileF = f
}

// SetCancelFuncs sets optional callbacks to cancel running reconcile and update tasks.
func (s *Service) SetCancelFuncs(cancelReconcile, cancelUpdate func()) {
	s.cancelRecon = cancelReconcile
	s.cancelUpd = cancelUpdate
}

// Start begins the refresh loop: listens for triggers from the queue and
// performs refreshes. Also starts the periodic poll ticker if configured.
// Blocks until ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	// Trigger initial startup refresh
	s.enqueueRefresh(TriggerStartup)

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
func (s *Service) RequestRefresh(source TriggerSource, parent ...string) QueueResult {
	return s.enqueueRefresh(source, parent...)
}

func (s *Service) enqueueRefresh(source TriggerSource, parent ...string) QueueResult {
	// Polling observes Git on a schedule, not a deployment deadline. Queue it
	// behind an active sync so large pulls are not repeatedly cancelled.
	if source != TriggerPeriodic && s.cancelRecon != nil {
		s.cancelRecon()
	}
	if source != TriggerPeriodic && s.cancelUpd != nil {
		s.cancelUpd()
	}

	parentID := ""
	if len(parent) > 0 {
		parentID = parent[0]
	}
	return s.queue.Enqueue(Trigger{
		ParentID:    parentID,
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
			s.enqueueRefresh(TriggerPeriodic)
		}
	}
}

func (s *Service) doRefresh(ctx context.Context, trigger Trigger) {
	if trigger.ParentID != "" {
		ctx = operations.WithID(ctx, trigger.ParentID)
	}
	ctx = s.broadcaster.BeginOperation(ctx, "refresh", string(trigger.Source), "", s.cfg.GitRevision)
	workID := s.broadcaster.BeginWork(ctx, desiredstate.WorkOptions{Source: "refresh", Revision: s.cfg.GitRevision, Phase: "checking_repository"})
	ctx = operations.WithTrigger(ctx, string(trigger.Source))
	log.Printf("[info] starting refresh (source: %s)", trigger.Source)

	s.store.SetRefreshState(desiredstate.RefreshStatusRefreshing, "", true, "refreshing", git.DefaultLocalRepoPath)
	if s.broadcaster != nil {
		s.broadcaster.PublishRefreshStatus(s.store.GetRefreshStatus())
	}

	entries, commitHash, commitMessage, err := s.reader.ReadComposeFiles(
		ctx,
		s.cfg.GitRepoURL,
		s.cfg.GitAccessToken,
		s.cfg.GitRevision,
		s.cfg.GitDeployDir,
	)

	if err != nil {
		log.Printf("[error] refresh failed: %v", err)
		status := "failed"
		if ctx.Err() != nil {
			status = "cancelled"
		}
		s.broadcaster.EndWork(workID, status, "Repository check "+status, "")
		s.broadcaster.EndOperation(ctx, "failed", err.Error())
		s.store.SetRefreshState(desiredstate.RefreshStatusFailed, err.Error(), true, "refresh failed", git.DefaultLocalRepoPath)
		if s.broadcaster != nil {
			s.broadcaster.PublishRefreshStatus(s.store.GetRefreshStatus())
		}
		s.queue.Done()
		return
	}

	newStacks := s.buildStacksPreservingStatus(entries)
	for i := range newStacks {
		newStacks[i].DesiredRevision = commitHash
	}

	snap := &desiredstate.Snapshot{
		Revision:       commitHash,
		CommitMessage:  commitMessage,
		Ref:            s.cfg.GitRevision,
		RefType:        "branch",
		RefreshedAt:    time.Now(),
		RefreshStatus:  desiredstate.RefreshStatusCompleted,
		RefreshError:   "",
		UpdatesBlocked: false,
		BlockedReason:  "",
		LocalPath:      git.DefaultLocalRepoPath,
		Stacks:         newStacks,
	}

	s.store.Set(snap)
	s.broadcaster.EndWork(workID, "completed", "Repository check completed", "")
	s.broadcaster.EndOperation(ctx, "completed", "Repository checked at "+truncate(commitHash, 12))
	log.Printf("[info] refresh completed: %d stacks at %s", len(newStacks), truncate(commitHash, 12))

	// Publish SSE events for connected frontends
	if s.broadcaster != nil {
		s.broadcaster.PublishStackSnapshot(newStacks)
		s.broadcaster.PublishRefreshStatus(s.store.GetRefreshStatus())
	}

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
		if prev, ok := existing[e.StackPath]; ok {
			rec = prev
			if prev.ComposeHash == hash {
				status = prev.Status
			}
			log.Printf("[debug] buildStacksPreservingStatus: stack=%s status=%s (runtime metadata preserved)", e.StackPath, status)
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
