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
	stopChan     chan struct{}
	store        *desiredstate.Store
	dockerClient *docker.Client
	reconciler   *reconcile.Reconciler
	broadcaster  *desiredstate.Broadcaster
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
	}, nil
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

	// Trigger update cycle in background
	go s.executeUpdateCycleManual(ctx, cycle)
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
	s.mu.Lock()
	// Terminate existing cycle if running (FR-020)
	if s.activeUpdate != nil {
		s.logger.Warn("terminating existing update cycle - new scheduled time arrived",
			"existing_cycle_id", s.activeUpdate.CycleID)
		s.activeUpdate = nil
	}

	// Create new update cycle
	cycle := NewUpdateCycle()
	s.activeUpdate = cycle
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
	s.executeUpdateCycle(ctx, cycle)
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

		result := s.updateStack(ctx, stack)
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

// updateStack updates a single stack (T014-T016)
func (s *SchedulerService) updateStack(ctx context.Context, stack desiredstate.StackRecord) StackUpdateResult {
	result := StackUpdateResult{
		StackName:   stack.Path,
		ProjectName: stack.Path, // Use path as project name
	}
	result.PullStartTime = time.Now()

	s.logger.Info("updating stack", "stack", stack.Path)

	// TODO: Get image digests before pull (T015)
	// This would require parsing the compose file to get image names

	// Pull images (T014)
	err := s.dockerClient.PullImages(ctx, result.ProjectName, stack.ComposeFile)
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

	// TODO: Get image digests after pull and compare (T015)
	// For MVP, always trigger reconciliation after pull
	// In a full implementation, we'd compare digests to detect changes

	// Trigger reconciliation (T016)
	runs := s.reconciler.Reconcile(ctx)
	for _, run := range runs {
		if run.StackPath == stack.Path {
			result.ReconcileTriggered = true
			if run.Result == "success" {
				result.Success = true
				s.logger.Info("stack reconciled",
					"stack", stack.Path,
					"result", run.Result)
			} else {
				result.Success = false
				result.Error = fmt.Errorf("reconcile failed: %s", run.Error)
				s.logger.Error("stack reconcile failed",
					"stack", stack.Path,
					"error", run.Error)
			}
			break
		}
	}

	return result
}
