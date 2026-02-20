package reconcile

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
)

// DriftDetector compares desired state against runtime state to detect configuration drift.
type DriftDetector struct {
	logger    *slog.Logger
	deployDir string
}

// NewDriftDetector creates a new drift detector.
func NewDriftDetector(deployDir string, logger *slog.Logger) *DriftDetector {
	return &DriftDetector{
		deployDir: deployDir,
		logger:    logger,
	}
}

// DriftResult describes the drift status for a single stack.
type DriftResult struct {
	Path       string
	NeedSync   bool
	NeedRemove bool
	Reason     string
}

// DetectChanges compares desired state against runtime labels and returns drift results.
func (d *DriftDetector) DetectChanges(
	ctx context.Context,
	desired []desiredstate.StackRecord,
	runtime map[string]StackSyncMetadata,
	removeEnabled bool,
) []DriftResult {
	var results []DriftResult

	desiredPaths := make(map[string]bool)
	for _, stk := range desired {
		desiredPaths[stk.Path] = true

		// Filter by deploy scope
		if d.deployDir != "" && !isInDeployScope(stk.Path, d.deployDir) {
			continue
		}

		rt, exists := runtime[stk.Path]
		if !exists {
			d.logger.DebugContext(ctx, "stack has no runtime metadata",
				"stack_path", stk.Path)
			results = append(results, DriftResult{
				Path:     stk.Path,
				NeedSync: true,
				Reason:   "no runtime metadata found",
			})
			continue
		}

		if rt.DesiredRevision == "" || rt.DesiredComposeHash == "" {
			d.logger.DebugContext(ctx, "stack has incomplete runtime metadata",
				"stack_path", stk.Path)
			results = append(results, DriftResult{
				Path:     stk.Path,
				NeedSync: true,
				Reason:   "missing or invalid sync metadata",
			})
			continue
		}

		if rt.DesiredComposeHash != stk.ComposeHash {
			d.logger.InfoContext(ctx, "stack runtime hash differs from desired",
				"stack_path", stk.Path,
				"runtime_hash", rt.DesiredComposeHash,
				"desired_hash", stk.ComposeHash)
			results = append(results, DriftResult{
				Path:     stk.Path,
				NeedSync: true,
				Reason:   fmt.Sprintf("compose hash drift: runtime=%s desired=%s", rt.DesiredComposeHash, stk.ComposeHash),
			})
			continue
		}

		// No drift
		results = append(results, DriftResult{
			Path:     stk.Path,
			NeedSync: false,
			Reason:   "in sync",
		})
	}

	// Check for stacks that exist in runtime but not in desired state
	if removeEnabled {
		for path := range runtime {
			if !desiredPaths[path] {
				if d.deployDir != "" && !isInDeployScope(path, d.deployDir) {
					continue
				}
				d.logger.InfoContext(ctx, "stack exists in runtime but not in desired state",
					"stack_path", path)
				results = append(results, DriftResult{
					Path:       path,
					NeedRemove: true,
					Reason:     "not in desired state",
				})
			}
		}
	}

	return results
}
