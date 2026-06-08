package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
)

// ComposeRunner abstracts docker compose command execution.
type ComposeRunner interface {
	// ComposeUp runs docker compose up -d with the given project name, compose file,
	// and optional override file for labels.
	ComposeUp(ctx context.Context, projectName, composeFile, overrideFile, workDir string) error
	// ComposeDown runs docker compose down --remove-orphans for the given project.
	ComposeDown(ctx context.Context, projectName, composeFile, workDir string) error
	// ComposePs lists running containers for a compose project.
	ComposePs(ctx context.Context, projectName string) ([]desiredstate.ContainerInfo, error)
}

// PullProgress reports per-image pull status.
type PullProgress struct {
	Image    string
	Status   string
	Progress string
	Current  int
	Total    int
}

// PullProgressFn is called with pull progress updates.
type PullProgressFn func(PullProgress)

// ImagePuller pulls container images with progress reporting.
type ImagePuller interface {
	PullImages(ctx context.Context, images []string, onProgress PullProgressFn) error
}

// ContainerInspector reads runtime container labels.
type ContainerInspector interface {
	// GetStackLabels returns sync metadata labels grouped by stack path.
	GetStackLabels(ctx context.Context) (map[string]StackSyncMetadata, error)
}

// StackSyncMetadata holds sync metadata read from container labels.
type StackSyncMetadata struct {
	StackPath            string
	DesiredRevision      string
	DesiredCommitMessage string
	DesiredComposeHash   string
	SyncedAt             string
	LastSyncAt           string
	SyncStatus           string
	SyncError            string
}

// ReconciliationRun tracks a single reconciliation attempt.
type ReconciliationRun struct {
	StackPath       string
	DesiredRevision string
	DesiredHash     string
	StartedAt       time.Time
	FinishedAt      time.Time
	Result          string // "success", "failed", "skipped"
	Error           string
}

// Reconciler compares desired state with runtime state and applies changes.
type Reconciler struct {
	mu            sync.Mutex
	cancelMu      sync.Mutex
	cancelActive  context.CancelFunc
	store         *desiredstate.Store
	policy        ReconciliationPolicy
	compose       ComposeRunner
	inspector     ContainerInspector
	ackStore      *AckStore
	deployDir     string
	driftDetector *DriftDetector
	stateManager  *StateManager
	imagePuller   ImagePuller
	broadcaster   *desiredstate.Broadcaster
	logger        *slog.Logger
}

// NewReconciler creates a Reconciler.
func NewReconciler(
	store *desiredstate.Store,
	policy ReconciliationPolicy,
	compose ComposeRunner,
	inspector ContainerInspector,
	ackStore *AckStore,
	deployDir string,
	driftDetector *DriftDetector,
	stateManager *StateManager,
	logger *slog.Logger,
	imagePuller ImagePuller,
	broadcaster *desiredstate.Broadcaster,
) *Reconciler {
	return &Reconciler{
		store:         store,
		policy:        policy,
		compose:       compose,
		inspector:     inspector,
		ackStore:      ackStore,
		deployDir:     deployDir,
		driftDetector: driftDetector,
		stateManager:  stateManager,
		imagePuller:   imagePuller,
		broadcaster:   broadcaster,
		logger:        logger,
	}
}

// Reconcile performs a full reconciliation cycle.
func (r *Reconciler) Reconcile(ctx context.Context) []ReconciliationRun {
	r.mu.Lock()
	defer r.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	r.cancelMu.Lock()
	r.cancelActive = cancel
	r.cancelMu.Unlock()
	defer func() {
		r.cancelMu.Lock()
		r.cancelActive = nil
		r.cancelMu.Unlock()
	}()

	if !r.policy.Enabled {
		r.logger.Info("reconciliation disabled, skipping")
		return nil
	}

	refresh := r.store.GetRefreshStatus()
	if refresh == nil || refresh.RefreshStatus != desiredstate.RefreshStatusCompleted || refresh.UpdatesBlocked {
		r.logger.Info("refresh not completed or updates blocked, skipping reconciliation")
		return nil
	}

	snap := r.store.Get()
	if snap == nil {
		r.logger.Info("no desired state available, skipping reconciliation")
		return nil
	}
	if ctx.Err() != nil {
		r.logger.Info("reconciliation canceled before runtime inspection")
		return nil
	}

	runtime, err := r.inspector.GetStackLabels(ctx)
	if err != nil {
		r.logger.Error("failed to inspect runtime state", "error", err)
		return nil
	}

	r.logger.Debug("runtime labels found", "stack_count", len(runtime))
	for path := range runtime {
		r.logger.Debug("runtime stack discovered", "stack_path", path)
	}

	drifts := r.driftDetector.DetectChanges(ctx, snap.Stacks, runtime, r.policy.RemoveEnabled)

	// For stacks that are in sync at runtime but have a stale store status
	// (e.g. "missing" after a fresh startup), correct the store from runtime metadata.
	for _, drift := range drifts {
		if drift.NeedSync || drift.NeedRemove {
			continue
		}
		rt, ok := runtime[drift.Path]
		if !ok {
			continue
		}
		// Find the corresponding store record
		for _, st := range snap.Stacks {
			if st.Path == drift.Path && st.Status != desiredstate.StackSyncSynced {
				r.logger.Info("correcting store status for in-sync stack", "stack_path", drift.Path, "old_status", st.Status)
				r.stateManager.MarkSynced(drift.Path, rt.DesiredRevision, rt.DesiredCommitMessage, rt.DesiredComposeHash, rt.SyncedAt)
				break
			}
		}
		// Always refresh container counts for in-sync stacks
		projectName := deriveProjectName(r.projectNamePrefix(), drift.Path)
		r.stateManager.UpdateContainerCounts(ctx, drift.Path, projectName)
	}

	var runs []ReconciliationRun

	for _, drift := range drifts {
		if ctx.Err() != nil {
			r.logger.Info("reconciliation canceled before applying changes")
			return runs
		}
		if !drift.NeedSync && !drift.NeedRemove {
			continue
		}

		if drift.NeedRemove {
			run := r.removeStack(ctx, drift, snap)
			runs = append(runs, run)
			continue
		}

		// Check drift policy
		if r.policy.DriftPolicy == "flag" {
			if !r.ackStore.IsAcknowledged(drift.Path) {
				r.logger.Info("stack has drift but policy is 'flag' and not acknowledged, skipping", "stack_path", drift.Path)
				r.stateManager.UpdateStatus(drift.Path, desiredstate.StackSyncFailed, "", "drift detected, awaiting acknowledgement")
				continue
			}
			// Clear acknowledgement after use
			r.ackStore.Clear(drift.Path)
		}

		run := r.syncStack(ctx, drift, snap)
		runs = append(runs, run)
	}

	return runs
}

// CancelActive requests cancellation of a running reconciliation.
func (r *Reconciler) CancelActive() {
	r.cancelMu.Lock()
	cancel := r.cancelActive
	r.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ReconcileStack performs a targeted reconciliation of a single stack.
// It bypasses drift detection entirely — always syncs the specified stack.
// After an image pull, docker compose up -d natively detects digest changes
// and recreates only the affected containers.
func (r *Reconciler) ReconcileStack(ctx context.Context, stackPath string) ReconciliationRun {
	r.mu.Lock()
	defer r.mu.Unlock()

	snap := r.store.Get()
	if snap == nil {
		return ReconciliationRun{StackPath: stackPath, Result: "failed", Error: "no desired state"}
	}

	// Build a synthetic DriftResult — always needs sync
	drift := DriftResult{
		Path:     stackPath,
		NeedSync: true,
		Reason:   "targeted reconciliation after image pull",
	}

	return r.syncStack(ctx, drift, snap)
}

func (r *Reconciler) syncStack(ctx context.Context, drift DriftResult, snap *desiredstate.Snapshot) ReconciliationRun {
	run := ReconciliationRun{
		StackPath:       drift.Path,
		DesiredRevision: snap.Revision,
		StartedAt:       time.Now(),
	}

	// Find the stack record to get compose file and hash
	var stack *desiredstate.StackRecord
	for i := range snap.Stacks {
		if snap.Stacks[i].Path == drift.Path {
			stack = &snap.Stacks[i]
			break
		}
	}
	if stack == nil {
		run.Result = "failed"
		run.Error = "stack not found in desired state"
		run.FinishedAt = time.Now()
		return run
	}

	run.DesiredHash = stack.ComposeHash

	r.logger.Info("reconciling stack", "stack_path", drift.Path, "reason", drift.Reason)

	// Update status to syncing
	r.stateManager.UpdateStatus(drift.Path, desiredstate.StackSyncSyncing, "", "")

	// Derive project name
	projectName := deriveProjectName(r.projectNamePrefix(), drift.Path)

	// Generate override file with labels applied to each service
	commitMessage := r.getCommitMessage(snap)
	serviceNames := extractServiceNames(stack.Content)
	if len(serviceNames) == 0 {
		r.logger.Warn("no service names extracted from compose file — labels will not be applied", "stack_path", drift.Path)
	}
	overrideContent := generateLabelOverride(drift.Path, snap.Revision, commitMessage, stack.ComposeHash, serviceNames)

	// Write compose file and override to temp directory so docker compose
	// can find them regardless of the process's working directory.
	composeFile, overrideFile, cleanup, err := writeTempComposeDir(stack.ComposeFile, stack.Content, overrideContent)
	if err != nil {
		run.Result = "failed"
		run.Error = fmt.Sprintf("failed to write compose files: %v", err)
		run.FinishedAt = time.Now()
		r.stateManager.UpdateStatus(drift.Path, desiredstate.StackSyncFailed, "", run.Error)
		return run
	}
	defer cleanup()

	// Explicitly pull images with progress reporting before compose up.
	// When imagePuller is nil, ComposeUp will pull images implicitly (no progress).
	if r.imagePuller != nil {
		images := ExtractComposeImages(stack.Content)
		if len(images) > 0 {
			var onProgress PullProgressFn
			if r.broadcaster != nil {
				onProgress = func(p PullProgress) {
					r.broadcaster.PublishUpdateProgress(map[string]interface{}{
						"type":      "image_pull_progress",
						"stack":     drift.Path,
						"image":     p.Image,
						"status":    p.Status,
						"progress":  p.Progress,
						"current":   p.Current,
						"total":     p.Total,
						"timestamp": time.Now(),
					})
				}
			}
			if err := r.imagePuller.PullImages(ctx, images, onProgress); err != nil {
				run.Result = "failed"
				run.Error = fmt.Sprintf("image pull failed: %v", err)
				run.FinishedAt = time.Now()
				r.logger.Error("image pull failed", "stack_path", drift.Path, "error", err)
				r.stateManager.UpdateStatus(drift.Path, desiredstate.StackSyncFailed, "", truncateError(run.Error))
				return run
			}
		}
	}

	// Run docker compose up
	// workDir is set to the stack path so Docker Compose resolves relative
	// volume mounts and build contexts correctly.
	workDir := drift.Path
	err = r.compose.ComposeUp(ctx, projectName, composeFile, overrideFile, workDir)
	if err != nil {
		run.Result = "failed"
		run.Error = fmt.Sprintf("compose up failed: %v", err)
		run.FinishedAt = time.Now()
		r.logger.Error("reconcile failed", "stack_path", drift.Path, "error", err)
		r.stateManager.UpdateStatus(drift.Path, desiredstate.StackSyncFailed, "", truncateError(run.Error))
		return run
	}

	run.Result = "success"
	run.FinishedAt = time.Now()
	r.logger.Info("reconcile succeeded", "stack_path", drift.Path)

	// Update status to synced with metadata
	now := time.Now().UTC().Format(time.RFC3339)
	r.stateManager.MarkSynced(drift.Path, snap.Revision, commitMessage, stack.ComposeHash, now)

	// Update container counts
	r.stateManager.UpdateContainerCounts(ctx, drift.Path, projectName)

	return run
}

func (r *Reconciler) removeStack(ctx context.Context, drift DriftResult, snap *desiredstate.Snapshot) ReconciliationRun {
	run := ReconciliationRun{
		StackPath:       drift.Path,
		DesiredRevision: snap.Revision,
		StartedAt:       time.Now(),
	}

	if !r.policy.RemoveEnabled {
		run.Result = "skipped"
		run.FinishedAt = time.Now()
		return run
	}

	r.logger.Info("removing stack", "stack_path", drift.Path, "reason", drift.Reason)

	r.stateManager.UpdateStatus(drift.Path, desiredstate.StackSyncDeleting, "", "")

	projectName := deriveProjectName(r.projectNamePrefix(), drift.Path)

	// For removal, we only need the project name — no compose file or workDir required.
	// docker compose -p <project> down --remove-orphans is sufficient.
	err := r.compose.ComposeDown(ctx, projectName, "", "")
	if err != nil {
		run.Result = "failed"
		run.Error = fmt.Sprintf("compose down failed: %v", err)
		run.FinishedAt = time.Now()
		r.logger.Error("removal failed", "stack_path", drift.Path, "error", err)
		r.stateManager.UpdateStatus(drift.Path, desiredstate.StackSyncFailed, "", truncateError(run.Error))
		return run
	}

	run.Result = "success"
	run.FinishedAt = time.Now()
	r.logger.Info("removal succeeded", "stack_path", drift.Path)

	// Mark stack as missing after removal
	r.stateManager.UpdateStatus(drift.Path, desiredstate.StackSyncMissing, "", "")

	return run
}

func (r *Reconciler) projectNamePrefix() string {
	return ""
}

// GetContainers returns container details for a stack.
func (r *Reconciler) GetContainers(ctx context.Context, stackPath string) ([]desiredstate.ContainerInfo, error) {
	projectName := deriveProjectName(r.projectNamePrefix(), stackPath)
	return r.compose.ComposePs(ctx, projectName)
}

func (r *Reconciler) getCommitMessage(snap *desiredstate.Snapshot) string {
	if snap == nil {
		return ""
	}
	return snap.CommitMessage
}

// deriveProjectName creates a compose project name from prefix and stack path.
func deriveProjectName(prefix, stackPath string) string {
	sanitized := strings.ReplaceAll(stackPath, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, "\\", "-")
	sanitized = strings.ToLower(sanitized)
	if prefix == "" {
		return sanitized
	}
	return prefix + "-" + sanitized
}

// isInDeployScope checks if a stack path is within the configured deploy directory.
func isInDeployScope(stackPath, deployDir string) bool {
	if deployDir == "" {
		return true
	}
	// Stack paths are relative to deploy dir, so they are always in scope
	// when provided from the desired state. This function filters out
	// runtime stacks that might not belong to the deploy scope.
	return true
}

// truncateError truncates an error message to a reasonable length.
func truncateError(s string) string {
	const maxLen = 256
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
