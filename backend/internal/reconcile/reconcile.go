package reconcile

import (
	"context"
	"fmt"
	"log"
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
	store         *desiredstate.Store
	policy        ReconciliationPolicy
	compose       ComposeRunner
	inspector     ContainerInspector
	ackStore      *AckStore
	deployDir     string
	driftDetector *DriftDetector
	stateManager  *StateManager
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
	}
}

// Reconcile performs a full reconciliation cycle.
func (r *Reconciler) Reconcile(ctx context.Context) []ReconciliationRun {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.policy.Enabled {
		log.Printf("[info] reconciliation disabled, skipping")
		return nil
	}

	snap := r.store.Get()
	if snap == nil {
		log.Printf("[info] no desired state available, skipping reconciliation")
		return nil
	}

	runtime, err := r.inspector.GetStackLabels(ctx)
	if err != nil {
		log.Printf("[error] failed to inspect runtime state: %v", err)
		return nil
	}

	log.Printf("[debug] runtime labels found for %d stack(s)", len(runtime))
	for path := range runtime {
		log.Printf("[debug]   runtime stack: %s", path)
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
				log.Printf("[info] correcting store status for in-sync stack %s (%s → synced)", drift.Path, st.Status)
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
				log.Printf("[info] stack %s has drift but policy is 'flag' and not acknowledged, skipping", drift.Path)
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

	log.Printf("[info] reconciling stack %s (reason: %s)", drift.Path, drift.Reason)

	// Update status to syncing
	r.stateManager.UpdateStatus(drift.Path, desiredstate.StackSyncSyncing, "", "")

	// Derive project name
	projectName := deriveProjectName(r.projectNamePrefix(), drift.Path)

	// Generate override file with labels applied to each service
	commitMessage := r.getCommitMessage(snap)
	serviceNames := extractServiceNames(stack.Content)
	if len(serviceNames) == 0 {
		log.Printf("[warn] no service names extracted from compose file for stack %s — labels will not be applied", drift.Path)
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

	// Run docker compose up
	// workDir is set to the stack path so Docker Compose resolves relative
	// volume mounts and build contexts correctly.
	workDir := drift.Path
	err = r.compose.ComposeUp(ctx, projectName, composeFile, overrideFile, workDir)
	if err != nil {
		run.Result = "failed"
		run.Error = fmt.Sprintf("compose up failed: %v", err)
		run.FinishedAt = time.Now()
		log.Printf("[error] reconcile failed for stack %s: %v", drift.Path, err)
		r.stateManager.UpdateStatus(drift.Path, desiredstate.StackSyncFailed, "", truncateError(run.Error))
		return run
	}

	run.Result = "success"
	run.FinishedAt = time.Now()
	log.Printf("[info] reconcile succeeded for stack %s", drift.Path)

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

	log.Printf("[info] removing stack %s (reason: %s)", drift.Path, drift.Reason)

	r.stateManager.UpdateStatus(drift.Path, desiredstate.StackSyncDeleting, "", "")

	projectName := deriveProjectName(r.projectNamePrefix(), drift.Path)

	// For removal, we only need the project name — no compose file or workDir required.
	// docker compose -p <project> down --remove-orphans is sufficient.
	err := r.compose.ComposeDown(ctx, projectName, "", "")
	if err != nil {
		run.Result = "failed"
		run.Error = fmt.Sprintf("compose down failed: %v", err)
		run.FinishedAt = time.Now()
		log.Printf("[error] removal failed for stack %s: %v", drift.Path, err)
		r.stateManager.UpdateStatus(drift.Path, desiredstate.StackSyncFailed, "", truncateError(run.Error))
		return run
	}

	run.Result = "success"
	run.FinishedAt = time.Now()
	log.Printf("[info] removal succeeded for stack %s", drift.Path)

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
