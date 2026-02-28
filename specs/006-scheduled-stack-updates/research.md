# Research: Scheduled Stack Updates

**Feature**: 006-scheduled-stack-updates  
**Phase**: 0 (Research)  
**Date**: February 27, 2026

## Research Summary

This document captures technical research for implementing automatic periodic stack updates with configurable cron scheduling, image pulls, container updates, and image pruning.

## Decision 1: Cron Expression Parsing

**Question**: Which cron parsing library should be used for Go?

**Options Considered**:
1. **github.com/robfig/cron/v3** - Most popular Go cron library with v3 supporting standard cron expressions
2. **github.com/gorhill/cronexpr** - Lightweight parser focused only on expression parsing
3. Custom implementation - Build minimal cron parser for basic patterns

**Decision**: Use **github.com/robfig/cron/v3**

**Rationale**:
- Industry standard with 10k+ GitHub stars and active maintenance
- Supports standard 5-field cron expressions (minute, hour, day, month, weekday)
- Provides both parsing/validation AND scheduling execution
- Thread-safe scheduler with graceful shutdown support
- Well-tested with edge case handling (timezone, DST, etc.)
- Already used by many Docker-related projects

**Implementation Notes**:
```go
import "github.com/robfig/cron/v3"

// Parse and validate expression
parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
schedule, err := parser.Parse("0 3 * * *")

// Create scheduler
c := cron.New()
c.Schedule(schedule, cron.FuncJob(updateFunc))
c.Start()
```

**Alternatives Considered**:
- cronexpr: Too minimal, doesn't include scheduler
- Custom: Reinventing well-solved problem, prone to edge case bugs

---

## Decision 2: Image Pull Strategy

**Question**: How should we pull images for all stacks?

**Decision**: **Sequential pulls via `docker compose pull` per stack**

**Rationale**:
- `docker compose pull` automatically pulls all images referenced in compose file
- Handles image references, registries, and authentication automatically
- Respects compose file structure (build context vs image references)
- Returns zero exit code only if all pulls succeed
- Works with existing `ComposeRunner` abstraction pattern

**Implementation Approach**:
```go
// For each stack:
// docker compose -p <project> -f <compose-file> pull --quiet
args := append(docker.HostArgs(socket), 
    "compose", "-p", projectName,
    "-f", composeFile,
    "pull", "--quiet")
```

**Per-Image Tracking**:
- Parse compose file to extract image list before pull
- After pull, use `docker inspect` to get image IDs and check for updates
- Log each image pull with name and outcome

**Error Handling**:
- If pull fails for one stack, log error and continue with remaining stacks (FR-012)
- Network failures logged as warnings, not fatal errors (FR-013)
- Missing images logged as errors but don't block other stacks

**Alternatives Considered**:
- Parallel pulls: Risks overwhelming Docker daemon and network bandwidth
- Direct `docker pull`: Would need to parse compose files and handle each image type separately
- Image digest comparison first: Adds complexity and extra registry API calls

---

## Decision 3: Image Pruning Strategy

**Question**: How should unused images be pruned after updates?

**Decision**: **System-wide prune via `docker image prune -af`**

**Rationale**:
- Per clarification, system-wide pruning is acceptable (includes non-stack images)
- `-a` removes all unused images, not just dangling ones
- `-f` (force) bypasses confirmation prompt
- Returns space reclaimed in output for logging
- Single atomic operation versus per-image removal

**Implementation**:
```go
// After all stacks updated:
// docker image prune -af
args := append(docker.HostArgs(socket), 
    "image", "prune", "-af")
out, err := runner.Run(ctx, "docker", args...)

// Parse output for space reclaimed:
// "Total reclaimed space: 1.234GB"
```

**Timing**: Prune runs once after all stack updates complete in the cycle

**Logging**: Log total images removed and space reclaimed (FR-011)

**Safety**: Pruning is safe because:
- Only removes unused images (not referenced by any container)
- Stacks just updated will have their images in use
- If someone needs an image, they can pull it again

**Alternatives Considered**:
- Prune only stack-related images: Complex tracking, doesn't match clarified requirement
- Prune per-stack: Inefficient, same images might be checked multiple times
- Skip pruning: Doesn't meet feature requirement for disk space management

---

## Decision 4: Update Cycle Coordination

**Question**: How should the update cycle coordinate all operations?

**Decision**: **Sequential state machine with explicit phases**

**Update Cycle Phases**:
1. **Start** - Log cycle start, acquire exclusive lock
2. **Pull** - For each stack, run `docker compose pull`
3. **Update** - For each stack with changed images, trigger reconciliation
4. **Prune** - Run system-wide image prune
5. **Complete** - Log summary, release lock

**State Management**:
```go
type UpdateCycle struct {
    StartTime       time.Time
    EndTime         time.Time
    StacksProcessed int
    ImagesPulled    int
    ContainersUpdated int
    ImagesP runed   int
    SpaceReclaimed  string
    Errors          []error
}
```

**Concurrency Control**:
- Use `sync.Mutex` to prevent overlapping update cycles
- TryLock pattern: if lock fails, log and skip (cycle already running)
- Per clarification, if new schedule triggers while running, terminate current and start fresh (FR-020)

**Integration with Reconciliation**:
- After pulling images for a stack, trigger existing reconciliation logic
- Reconciler compares image digests to detect changes
- Only recreates containers when images actually changed (FR-006)

**Alternatives Considered**:
- Parallel updates: Violates sequential constraint (FR-016), risks resource contention
- Separate pull/update phases: Would require storing state between phases, more complex
- No state tracking: Can't provide summary logs required by FR-011

---

## Decision 5: Scheduler Lifetime Management

**Question**: How should the scheduler integrate with service lifecycle?

**Decision**: **Independent service with graceful shutdown**

**Architecture**:
```go
type SchedulerService struct {
    cron         *cron.Cron
    config       config.UpdaterConfig
    updateRunner *UpdateRunner
    logger       *slog.Logger
}

func (s *SchedulerService) Start(ctx context.Context) {
    // Start cron scheduler
    s.cron.Start()
    
    // Wait for shutdown signal
    <-ctx.Done()
    
    // Graceful shutdown
    ctx := s.cron.Stop() // Returns context that closes when jobs finish
    <-ctx.Done()
}
```

**Startup**:
- Initialize scheduler during main.go startup
- Validate cron expression before starting (FR-003)
- Start scheduler in separate goroutine
- Log configured schedule on startup

**Shutdown**:
- Scheduler stops accepting new triggers
- If update cycle is in progress, allow it to finish (with timeout)
- Log shutdown and any incomplete operations

**Restart Behavior**:
- Per clarification, abandon in-progress updates on restart (FR-018)
- No state persistence needed
- Next scheduled time will trigger fresh update

**Configuration Reload**:
- Changing cron expression requires service restart
- Validated on startup, errors prevent service from starting
- UPDATER_ENABLED can toggle feature on/off

**Alternatives Considered**:
- State persistence: Not needed per clarification, adds complexity
- Resume on restart: Explicitly rejected per clarification
- Hot reload: Not required for MVP, config changes are infrequent

---

## Decision 6: Logging Strategy

**Question**: How should update operations be logged?

**Decision**: **Structured logging with slog at multiple levels**

**Log Levels**:
- **Info**: Cycle start/end, successful operations, summary stats
- **Warn**: Non-fatal errors (single stack failures, network issues)
- **Error**: Fatal errors (cron parse failure, scheduler crash)
- **Debug**: Detailed operation traces (when LOG_LEVEL=debug)

**Log Structure**:
```go
// Cycle start
logger.Info("update cycle started",
    "cycle_id", uuid,
    "scheduled_time", time,
    "stacks", count)

// Image pull
logger.Info("pulling images",
    "stack", stackName,
    "project", projectName,
    "images", imageCount)

// Pull result
logger.Info("images pulled",
    "stack", stackName,
    "duration", elapsed,
    "updated", updatedCount)

// Error
logger.Warn("stack update failed",
    "stack", stackName,
    "error", err,
    "continued", true)

// Cycle complete
logger.Info("update cycle completed",
    "cycle_id", uuid,
    "duration", elapsed,
    "stacks_processed", count,
    "containers_updated", count,
    "images_pruned", count,
    "space_reclaimed", "1.2GB",
    "errors", errorCount)
```

**Meets Requirements**:
- FR-008: Cycle start/end with timestamps
- FR-009: Each image pull with result
- FR-010: Each stack update with details
- FR-011: Prune operation with stats
- FR-013: Network failures logged
- FR-014: Skipped stacks logged with reason
- FR-019: All notifications via logs

**Integration**:
- Uses existing `slog.Logger` passed from main
- Follows existing logging patterns in codebase
- JSON format when `LOG_FORMAT=json` for machine parsing
- Text format by default for human readability

**Alternatives Considered**:
- Custom log format: Standard slog is sufficient and consistent
- External notifications: Explicitly not required per clarification
- Metrics/telemetry: Out of scope for MVP

---

## Decision 7: Configuration Schema

**Question**: What configuration options are needed?

**Decision**: **Environment variables following existing pattern**

**New Configuration Fields**:
```go
type Config struct {
    // Existing fields...
    
    // Updater settings
    UpdaterEnabled bool          // UPDATER_ENABLED
    UpdaterCron     string        // UPDATER_CRON
}
```

**Defaults**:
- `UPDATER_ENABLED`: `false` (opt-in to avoid surprise behavior changes)
- `UPDATER_CRON`: `"0 3 * * *"` (3 AM UTC daily)

**Validation**:
- Cron expression validated on startup using cron parser
- Invalid expression logs error and falls back to default
- If default also invalid (shouldn't happen), service fails to start

**Environment Variables**:
```bash
UPDATER_ENABLED=true
UPDATER_CRON="0 3 * * *"
```

**Rationale**:
- Follows existing config pattern (environment variables, Config struct)
- Disabled by default prevents unexpected updates
- Clear naming with "UPDATER_" prefix
- Validation at startup prevents runtime errors

**Alternatives Considered**:
- Config file: Breaks existing pattern, adds complexity
- Enabled by default: Risk of breaking existing deployments
- Multiple schedule options: Out of scope for MVP

---

## Decision 8: Image Change Detection

**Question**: How to detect if pulled images are different from running containers?

**Decision**: **Compare image digests via Docker inspect**

**Approach**:
1. Before pull, inspect running container to get current image digest
2. After pull, inspect image to get new digest
3. If digests differ, trigger reconciliation

**Implementation**:
```go
// Get container's current image digest
docker inspect --format='{{.Image}}' <container-id>
// Returns: sha256:abc123...

// Get pulled image digest
docker inspect --format='{{.Id}}' <image-name:tag>
// Returns: sha256:def456...

// If different, reconcile needed
```

**Reconciliation Trigger**:
- Digests differ → Call existing `Reconciler.ReconcileStack()`
- Reconciler will `docker compose up -d` which recreates changed containers
- Unchanged containers remain running (Docker Compose's default behavior)

**Edge Cases**:
- Container doesn't exist: Skip comparison, reconcile will handle
- Image not found: Already logged during pull, skip update
- Digest comparison fails: Log warning, attempt update anyway (safe)

**Rationale**:
- Digest is cryptographic hash - reliable change detection
- Works with any tag (not just "latest")
- Leverages existing reconciliation logic
- No need to parse compose files for image mapping

**Alternatives Considered**:
- Compare last pulled times: Not reliable, registry might update without changing image
- Force update all: Wasteful, violates FR-006 (don't recreate if unchanged)
- Parse compose ps output: More complex than direct inspect

---

## Research Checklist

- [x] Cron parsing library selected and validated
- [x] Image pull approach defined
- [x] Image pruning strategy clarified
- [x] Update cycle coordination designed
- [x] Scheduler lifecycle management specified
- [x] Logging strategy established
- [x] Configuration schema defined
- [x] Image change detection method chosen
- [x] All decisions align with constitutional principles
- [x] No NEEDS CLARIFICATION items remain

## Dependencies

**New External Dependencies**:
- `github.com/robfig/cron/v3` - Cron expression parsing and scheduling

**Existing Dependencies** (reused):
- Docker CLI via `CommandRunner`
- Existing `reconcile` package
- Existing `desiredstate` package
- Standard `slog` logging

## Risk Assessment

**Low Risk**:
- Cron library is mature and widely used
- Reuses proven reconciliation logic
- Sequential execution prevents race conditions

**Medium Risk**:
- System-wide pruning might remove images used elsewhere
  - Mitigation: Document prune scope, allow disabling scheduler
- Update cycles might be slow for many stacks
  - Mitigation: 30-minute goal, sequential processing, proper timeouts

**High Risk**: None identified

## Next Steps

Proceed to Phase 1: Design
- Define data model for update cycle tracking
- Design logging contracts (structured log format)
- Create quickstart guide for configuration
