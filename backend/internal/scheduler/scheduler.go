package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	"github.com/lucasreiners/docker-cd/internal/git"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/robfig/cron/v3"
)

// SchedulerService manages scheduled update cycles using cron expressions.
type SchedulerService struct {
	config       config.Config
	cron         *cron.Cron
	logger       *slog.Logger
	mu           sync.Mutex
	activeUpdate *UpdateCycle
	activeCancel context.CancelFunc
	stopChan     chan struct{}
	store        *desiredstate.Store
	dockerClient *docker.Client
	reconciler   *reconcile.Reconciler
	broadcaster  *desiredstate.Broadcaster
	repoPath     string // base path for local repo checkout; defaults to git.DefaultLocalRepoPath
}

// NewSchedulerService creates a new scheduler service.
func NewSchedulerService(
	cfg config.Config,
	logger *slog.Logger,
	store *desiredstate.Store,
	dockerClient *docker.Client,
	reconciler *reconcile.Reconciler,
	broadcaster *desiredstate.Broadcaster,
) (*SchedulerService, error) {
	if !cfg.UpdaterEnabled {
		return nil, nil // Scheduler disabled
	}

	// Validate cron expression
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(cfg.UpdaterCron)
	if err != nil {
		logger.Warn("invalid cron expression, using default",
			"provided", cfg.UpdaterCron,
			"default", "0 3 * * *",
			"error", err)
		cfg.UpdaterCron = "0 3 * * *"
	}

	return &SchedulerService{
		config:       cfg,
		logger:       logger,
		stopChan:     make(chan struct{}),
		store:        store,
		dockerClient: dockerClient,
		reconciler:   reconciler,
		broadcaster:  broadcaster,
		repoPath:     git.DefaultLocalRepoPath,
	}, nil
}

// SetRepoPath overrides the base repo path used to locate compose files during
// update cycles. This is primarily useful for testing, where /repo is not
// writable. In production the default (git.DefaultLocalRepoPath) is used.
func (s *SchedulerService) SetRepoPath(path string) {
	s.repoPath = path
}

// Start begins the scheduler service lifecycle.
// This method blocks until the context is cancelled.
func (s *SchedulerService) Start(ctx context.Context) {
	if s == nil {
		return // Scheduler disabled
	}

	s.logger.Info("scheduler configuration",
		"enabled", s.config.UpdaterEnabled,
		"schedule", s.config.UpdaterCron)

	// Initialize cron scheduler
	s.cron = cron.New()

	// Add scheduled update job (T012)
	_, err := s.cron.AddFunc(s.config.UpdaterCron, func() {
		s.runUpdateCycle(context.Background())
	})
	if err != nil {
		s.logger.Error("failed to schedule update job", "error", err)
		return
	}

	s.cron.Start()
	defer s.cron.Stop()

	// Wait for shutdown
	<-ctx.Done()
	s.logger.Info("scheduler shutting down")
}

// Stop gracefully stops the scheduler service.
// If an update cycle is in progress, it waits for completion with a timeout.
func (s *SchedulerService) Stop(ctx context.Context) error {
	if s == nil {
		return nil // Scheduler disabled
	}

	s.logger.Info("scheduler stop requested")

	// Stop accepting new scheduled triggers
	if s.cron != nil {
		cronCtx := s.cron.Stop()
		select {
		case <-cronCtx.Done():
			s.logger.Info("cron scheduler stopped")
		case <-ctx.Done():
			s.logger.Warn("cron scheduler stop timed out")
		}
	}

	// Check if update cycle is running
	s.mu.Lock()
	hasActive := s.activeUpdate != nil
	s.mu.Unlock()

	if hasActive {
		s.logger.Info("waiting for active update cycle to complete")
		// Wait for completion or timeout
		select {
		case <-s.stopChan:
			s.logger.Info("active update cycle completed")
		case <-ctx.Done():
			s.logger.Warn("active update cycle termination timed out")
		}
	}

	return nil
}

// TriggerUpdateCycle manually triggers an update cycle.
// Returns an error if the scheduler is disabled or if an update is already in progress.
func (s *SchedulerService) TriggerUpdateCycle(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("scheduler is disabled")
	}

	if ok, reason := s.refreshReady(); !ok {
		return fmt.Errorf("updates blocked: %s", reason)
	}

	s.logger.Info("TriggerUpdateCycle called")

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeUpdate != nil {
		s.logger.Warn("update cycle already in progress",
			"cycle_id", s.activeUpdate.CycleID,
			"start_time", s.activeUpdate.StartTime,
			"stacks_processed", s.activeUpdate.StacksProcessed)
		return fmt.Errorf("update cycle already in progress (cycle_id: %s)", s.activeUpdate.CycleID)
	}

	s.logger.Info("no active update found, creating new cycle")

	// Create and set active update immediately to prevent race condition
	cycle := NewUpdateCycle()
	s.activeUpdate = cycle

	s.logger.Info("update cycle created and starting",
		"cycle_id", cycle.CycleID,
		"start_time", cycle.StartTime)

	cycleCtx, cancel := context.WithCancel(ctx)
	s.activeCancel = cancel

	// Trigger update cycle in background
	go s.executeUpdateCycleManual(cycleCtx, cycle)
	return nil
}

// GetUpdateStatus returns the current update cycle status.
// Returns nil (untyped) when no update is in progress to allow proper nil checks.
func (s *SchedulerService) GetUpdateStatus() interface{} {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeUpdate == nil {
		s.logger.Debug("GetUpdateStatus: no active update")
		return nil // Return untyped nil, not typed nil pointer
	}

	s.logger.Debug("GetUpdateStatus: active update found",
		"cycle_id", s.activeUpdate.CycleID,
		"stacks_processed", s.activeUpdate.StacksProcessed)
	return s.activeUpdate
}

// runUpdateCycle executes a full update cycle (T013)
// Used by cron scheduler - can terminate existing cycles
func (s *SchedulerService) runUpdateCycle(ctx context.Context) {
	if ok, reason := s.refreshReady(); !ok {
		s.logger.Warn("update cycle skipped - refresh not ready", "reason", reason)
		return
	}
	s.mu.Lock()
	// Terminate existing cycle if running (FR-020)
	if s.activeUpdate != nil {
		s.logger.Warn("terminating existing update cycle - new scheduled time arrived",
			"existing_cycle_id", s.activeUpdate.CycleID)
		s.activeUpdate = nil
	}

	// Create new update cycle
	cycle := NewUpdateCycle()
	cycleCtx, cancel := context.WithCancel(ctx)
	s.activeUpdate = cycle
	s.activeCancel = cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.activeUpdate = nil
		s.mu.Unlock()
		select {
		case s.stopChan <- struct{}{}:
		default:
		}
	}()

	// Execute update cycle
	s.executeUpdateCycle(cycleCtx, cycle)
}

func (s *SchedulerService) refreshReady() (bool, string) {
	if s.store == nil {
		return false, "refresh status unavailable"
	}
	status := s.store.GetRefreshStatus()
	if status == nil || status.RefreshStatus != desiredstate.RefreshStatusCompleted {
		return false, "refresh not completed"
	}
	if status.UpdatesBlocked {
		reason := status.BlockedReason
		if reason == "" {
			reason = "updates blocked"
		}
		return false, reason
	}
	return true, ""
}

// executeUpdateCycleManual executes an update cycle from manual trigger
// Does not terminate existing cycles (that check already happened)
func (s *SchedulerService) executeUpdateCycleManual(ctx context.Context, cycle *UpdateCycle) {
	s.logger.Info("executeUpdateCycleManual started",
		"cycle_id", cycle.CycleID)

	defer func() {
		s.logger.Info("executeUpdateCycleManual completing, clearing activeUpdate",
			"cycle_id", cycle.CycleID)
		s.mu.Lock()
		s.activeUpdate = nil
		s.activeCancel = nil
		s.mu.Unlock()
		s.logger.Info("activeUpdate cleared",
			"cycle_id", cycle.CycleID)
		select {
		case s.stopChan <- struct{}{}:
		default:
		}
	}()

	s.executeUpdateCycle(ctx, cycle)
	s.logger.Info("executeUpdateCycle completed",
		"cycle_id", cycle.CycleID)
}

// executeUpdateCycle performs the actual update operations (T013-T019)
func (s *SchedulerService) executeUpdateCycle(ctx context.Context, cycle *UpdateCycle) {
	if ctx.Err() != nil {
		s.logger.Warn("update cycle canceled before start", "cycle_id", cycle.CycleID)
		return
	}
	// Log cycle start (T018)
	s.logger.Info("update cycle started",
		"cycle_id", cycle.CycleID,
		"scheduled_time", cycle.StartTime)

	// Publish started event
	if s.broadcaster != nil {
		s.broadcaster.PublishUpdateProgress(map[string]interface{}{
			"type":      "started",
			"cycle_id":  cycle.CycleID,
			"message":   "Update cycle started",
			"timestamp": cycle.StartTime,
		})
	}

	snap := s.store.Get()
	if snap == nil {
		s.logger.Warn("no desired state available, skipping update cycle")
		if s.broadcaster != nil {
			s.broadcaster.PublishUpdateProgress(map[string]interface{}{
				"type":      "completed",
				"cycle_id":  cycle.CycleID,
				"message":   "No stacks to update",
				"timestamp": time.Now(),
			})
		}
		return
	}

	totalStacks := len(snap.Stacks)

	// Process each stack sequentially (T014, FR-016)
	for i, stack := range snap.Stacks {
		if ctx.Err() != nil {
			s.logger.Warn("update cycle canceled", "cycle_id", cycle.CycleID)
			return
		}
		// Skip if stack is in error state (FR-014, T019)
		if stack.Status == desiredstate.StackSyncFailed {
			s.logger.Warn("skipping stack in failed state",
				"stack", stack.Path,
				"status", stack.Status)
			continue
		}

		// Publish stack progress event
		if s.broadcaster != nil {
			s.broadcaster.PublishUpdateProgress(map[string]interface{}{
				"type":      "stack_progress",
				"cycle_id":  cycle.CycleID,
				"stack":     stack.Path,
				"current":   i + 1,
				"total":     totalStacks,
				"message":   fmt.Sprintf("Updating stack %s (%d/%d)", stack.Path, i+1, totalStacks),
				"timestamp": time.Now(),
			})
		}

		// Build per-image pull progress callback for SSE
		var onPullProgress docker.PullProgressFn
		if s.broadcaster != nil {
			onPullProgress = func(p docker.PullProgress) {
				s.broadcaster.PublishUpdateProgress(map[string]interface{}{
					"type":      "image_pull_progress",
					"cycle_id":  cycle.CycleID,
					"stack":     stack.Path,
					"image":     p.Image,
					"status":    p.Status,
					"progress":  p.Progress,
					"current":   p.Current,
					"total":     p.Total,
					"timestamp": time.Now(),
				})
			}
		}

		result := s.updateStack(ctx, stack, onPullProgress)
		cycle.StacksProcessed++

		if !result.Success {
			cycle.Errors = append(cycle.Errors, result.Error)
			s.logger.Warn("stack update failed, continuing with remaining stacks",
				"stack", stack.Path,
				"error", result.Error) // T019 - Continue on error (FR-012)

			// Publish error event
			if s.broadcaster != nil {
				s.broadcaster.PublishUpdateProgress(map[string]interface{}{
					"type":      "stack_error",
					"cycle_id":  cycle.CycleID,
					"stack":     stack.Path,
					"error":     result.Error.Error(),
					"timestamp": time.Now(),
				})
			}
		} else {
			cycle.ImagesPulled += len(result.ImagesPulled)
			cycle.ContainersUpdated += result.ContainersUpdated

			// Publish success event
			if s.broadcaster != nil {
				s.broadcaster.PublishUpdateProgress(map[string]interface{}{
					"type":      "stack_success",
					"cycle_id":  cycle.CycleID,
					"stack":     stack.Path,
					"timestamp": time.Now(),
				})
			}
		}
	}

	// Prune images (T017)
	if s.broadcaster != nil {
		s.broadcaster.PublishUpdateProgress(map[string]interface{}{
			"type":      "pruning",
			"cycle_id":  cycle.CycleID,
			"message":   "Pruning unused images",
			"timestamp": time.Now(),
		})
	}

	imagesRemoved, spaceReclaimed, err := s.dockerClient.PruneImages(ctx)
	if err != nil {
		s.logger.Error("image prune failed", "error", err)
		cycle.Errors = append(cycle.Errors, err)
	} else {
		cycle.ImagesPruned = imagesRemoved
		cycle.SpaceReclaimed = spaceReclaimed
		s.logger.Info("images pruned",
			"images_removed", imagesRemoved,
			"space_reclaimed", spaceReclaimed)
	}

	// Complete cycle (T018)
	cycle.Complete()
	s.logger.Info("update cycle completed",
		"cycle_id", cycle.CycleID,
		"duration", cycle.EndTime.Sub(cycle.StartTime),
		"stacks_processed", cycle.StacksProcessed,
		"images_pulled", cycle.ImagesPulled,
		"containers_updated", cycle.ContainersUpdated,
		"images_pruned", cycle.ImagesPruned,
		"space_reclaimed", cycle.SpaceReclaimed,
		"errors", len(cycle.Errors))

	// Publish completion event
	if s.broadcaster != nil {
		s.broadcaster.PublishUpdateProgress(map[string]interface{}{
			"type":               "completed",
			"cycle_id":           cycle.CycleID,
			"message":            "Update cycle completed",
			"timestamp":          cycle.EndTime,
			"duration":           cycle.EndTime.Sub(cycle.StartTime).String(),
			"stacks_processed":   cycle.StacksProcessed,
			"images_pulled":      cycle.ImagesPulled,
			"containers_updated": cycle.ContainersUpdated,
			"images_pruned":      cycle.ImagesPruned,
			"space_reclaimed":    cycle.SpaceReclaimed,
			"errors":             len(cycle.Errors),
		})
	}
}

// CancelActiveUpdate requests cancellation of a running update cycle.
func (s *SchedulerService) CancelActiveUpdate() {
	s.mu.Lock()
	cancel := s.activeCancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// updateStack updates a single stack (T014-T016)
func (s *SchedulerService) updateStack(ctx context.Context, stack desiredstate.StackRecord, onPullProgress docker.PullProgressFn) StackUpdateResult {
	result := StackUpdateResult{
		StackName:   stack.Path,
		ProjectName: stack.Path, // Use path as project name
	}
	result.PullStartTime = time.Now()

	s.logger.Info("updating stack", "stack", stack.Path)

	// Get list of images in compose file (T015)
	images := reconcile.ExtractComposeImages(stack.Content)
	if len(images) == 0 {
		s.logger.Warn("no images found in compose content, will reconcile unconditionally",
			"stack", stack.Path)
	}

	// Get image digests before pull (T015)
	digestsBefore := make(map[string]string, len(images))
	for _, img := range images {
		digest, err := s.dockerClient.GetImageDigest(ctx, img)
		if err != nil {
			s.logger.Debug("failed to get pre-pull digest",
				"stack", stack.Path,
				"image", img,
				"error", err)
			// Mark as unknown — will force reconcile later
			digestsBefore[img] = ""
		} else {
			digestsBefore[img] = digest
			s.logger.Debug("pre-pull digest",
				"stack", stack.Path,
				"image", img,
				"digest", digest)
		}
	}

	// Pull images (T014)
	err := s.dockerClient.PullImages(ctx, images, onPullProgress)
	result.PullEndTime = time.Now()

	if err != nil {
		result.Success = false
		result.Error = err
		s.logger.Error("image pull failed",
			"stack", stack.Path,
			"error", err) // T018 - Error logging
		return result
	}

	s.logger.Info("images pulled", "stack", stack.Path)

	// Get image digests after pull and compare (T015)
	anyChanged := false
	digestComparisonFailed := false

	for _, img := range images {
		pullResult := ImagePullResult{
			ImageName:      img,
			PreviousDigest: digestsBefore[img],
			Success:        true,
		}

		newDigest, err := s.dockerClient.GetImageDigest(ctx, img)
		if err != nil {
			s.logger.Warn("failed to get post-pull digest, assuming changed",
				"stack", stack.Path,
				"image", img,
				"error", err)
			pullResult.Changed = true
			digestComparisonFailed = true
			anyChanged = true
		} else {
			pullResult.NewDigest = newDigest
			if digestsBefore[img] == "" {
				// Pre-pull digest was unknown — assume changed
				pullResult.Changed = true
				anyChanged = true
				s.logger.Info("image updated (pre-pull digest unknown)",
					"stack", stack.Path,
					"image", img,
					"new_digest", newDigest)
			} else if digestsBefore[img] != newDigest {
				pullResult.Changed = true
				anyChanged = true
				s.logger.Info("image updated",
					"stack", stack.Path,
					"image", img,
					"old_digest", digestsBefore[img],
					"new_digest", newDigest)
			} else {
				pullResult.Changed = false
				s.logger.Debug("image unchanged",
					"stack", stack.Path,
					"image", img,
					"digest", newDigest)
			}
		}

		result.ImagesPulled = append(result.ImagesPulled, pullResult)
	}

	// If we couldn't get images list, or digest comparison failed, reconcile unconditionally
	if len(images) == 0 || digestComparisonFailed {
		s.logger.Info("reconciling unconditionally (digest comparison unavailable)",
			"stack", stack.Path)
		anyChanged = true
	}

	if !anyChanged {
		s.logger.Info("no images changed, skipping reconciliation",
			"stack", stack.Path,
			"images_checked", len(images))
		result.Success = true
		result.ReconcileTriggered = false
		return result
	}

	// Reconcile — only when images actually changed (T016)
	changedCount := 0
	for _, pr := range result.ImagesPulled {
		if pr.Changed {
			changedCount++
		}
	}
	s.logger.Info("images changed, triggering reconciliation",
		"stack", stack.Path,
		"images_changed", changedCount,
		"images_total", len(result.ImagesPulled))

	run := s.reconciler.ReconcileStack(ctx, stack.Path)
	result.ReconcileTriggered = true
	if run.Result == "success" {
		result.Success = true
		s.logger.Info("stack reconciled after image pull",
			"stack", stack.Path,
			"result", run.Result,
			"images_changed", changedCount)
	} else {
		result.Success = false
		result.Error = fmt.Errorf("reconcile failed: %s", run.Error)
		s.logger.Error("stack reconcile failed after image pull",
			"stack", stack.Path,
			"error", run.Error)
	}

	return result
}
