# Data Model: Scheduled Stack Updates

**Feature**: 006-scheduled-stack-updates  
**Phase**: 1 (Design)  
**Date**: February 27, 2026

## Overview

This document defines the data structures for the scheduled stack updates feature. These entities capture update schedule configuration, update cycle state, and operation results.

## Entity Definitions

### 1. Update Schedule Configuration

**Purpose**: Defines when and how automatic updates should run

**Attributes**:
- `Enabled` (bool): Whether scheduled updates are active
- `CronExpression` (string): Cron expression defining update schedule (e.g., "0 3 * * *")
- `Timezone` (string): Always "UTC" for this version

**Validation Rules**:
- CronExpression must be valid 5-field cron format (minute hour day month weekday)
- CronExpression defaults to "0 3 * * *" if not provided
- Enabled defaults to false if not provided

**Source**: Environment variables via Config struct

**Go Representation**:
```go
type UpdaterConfig struct {
    Enabled        bool
    CronExpression string
}
```

**Relationships**:
- Read by SchedulerService on startup
- Immutable after service starts (requires restart to change)

---

### 2. Update Cycle

**Purpose**: Represents a single execution of the scheduled update process from start to finish

**Attributes**:
- `CycleID` (string): Unique identifier for this cycle (UUID)
- `StartTime` (time.Time): When cycle began
- `EndTime` (time.Time): When cycle completed (nil if in progress)
- `StacksProcessed` (int): Count of stacks attempted
- `ImagesPulled` (int): Total images successfully pulled across all stacks
- `ContainersUpdated` (int): Total containers recreated due to image changes
- `ImagesPruned` (int): Number of images removed during prune
- `SpaceReclaimed` (string): Disk space freed by prune (e.g., "1.2GB")
- `Errors` ([]error): Collection of non-fatal errors encountered

**State Transitions**:
1. **Created** → StartTime set, CycleID assigned
2. **Running** → Stacks being processed
3. **Completed** → EndTime set, final counts recorded
4. **Terminated** → If new cycle starts before completion (FR-020)

**Go Representation**:
```go
type UpdateCycle struct {
    CycleID           string
    StartTime         time.Time
    EndTime           time.Time
    StacksProcessed   int
    ImagesPulled      int
    ContainersUpdated int
    ImagesPruned      int
    SpaceReclaimed    string
    Errors            []error
}
```

**Persistence**: In-memory only (not persisted across restarts per FR-018)

**Relationships**:
- Created by SchedulerService when cron triggers
- Contains multiple StackUpdateResults
- Logged at start and completion

---

### 3. Stack Update Result

**Purpose**: Captures the outcome of updating a specific stack during an update cycle

**Attributes**:
- `StackName` (string): Identifier for the stack (directory name)
- `ProjectName` (string): Docker Compose project name
- `PullStartTime` (time.Time): When image pull began
- `PullEndTime` (time.Time): When image pull completed
- `ImagesPulled` ([]ImagePullResult): Details for each image
- `ReconcileTriggered` (bool): Whether reconciliation was invoked
- `ContainersUpdated` (int): Number of containers recreated
- `Success` (bool): Overall success status for this stack
- `Error` (error): Error if stack update failed (nil if successful)

**Success Criteria**:
- Success = true if pull succeeded and reconciliation completed (or was skipped)
- Success = false if pull failed or reconciliation failed
- Errors are logged but don't prevent processing remaining stacks (FR-012)

**Go Representation**:
```go
type StackUpdateResult struct {
    StackName          string
    ProjectName        string
    PullStartTime      time.Time
    PullEndTime        time.Time
    ImagesPulled       []ImagePullResult
    ReconcileTriggered bool
    ContainersUpdated  int
    Success            bool
    Error              error
}
```

**Relationships**:
- Child of UpdateCycle
- Contains multiple ImagePullResults
- Feeds into cycle summary counts

---

### 4. Image Pull Result

**Purpose**: Records the outcome of pulling a specific container image

**Attributes**:
- `ImageName` (string): Full image reference (e.g., "nginx:latest")
- `PreviousDigest` (string): Digest before pull (empty if new)
- `NewDigest` (string): Digest after pull
- `Changed` (bool): Whether digest changed
- `Success` (bool): Whether pull succeeded
- `Error` (error): Error if pull failed (nil if successful)
- `Duration` (time.Duration): How long pull took

**Change Detection**:
- Changed = true if PreviousDigest != NewDigest
- Changed = false if digests match (no update needed)
- Changed = N/A if pull failed

**Go Representation**:
```go
type ImagePullResult struct {
    ImageName      string
    PreviousDigest string
    NewDigest      string
    Changed        bool
    Success        bool
    Error          error
    Duration       time.Duration
}
```

**Relationships**:
- Child of StackUpdateResult
- Directly logged during pull operation (FR-009)
- Determines if reconciliation is needed

---

### 5. Update Operation Log Entry

**Purpose**: Structured log record for a specific operation during update cycle

**Attributes**:
- `Timestamp` (time.Time): When operation occurred
- `Level` (string): Log level (info, warn, error, debug)
- `Operation` (string): Type of operation (pull, update, prune, cycle_start, cycle_end)
- `Subject` (string): What was operated on (stack name, image name, or "all")
- `Status` (string): Result status (success, failed, skipped)
- `Message` (string): Human-readable description
- `Metadata` (map[string]any): Additional structured data
- `Error` (string): Error message if operation failed (empty if success)

**Operation Types**:
- `cycle_start`: Update cycle beginning
- `cycle_end`: Update cycle completion
- `pull`: Image pull for a stack
- `update`: Stack reconciliation/container update
- `prune`: System-wide image prune
- `skip`: Stack skipped (stopped or error state)

**Log Level Guidelines**:
- **info**: Normal operations, successes, summaries
- **warn**: Non-fatal errors, skipped stacks, network issues
- **error**: Fatal errors, scheduler failures
- **debug**: Detailed traces, intermediate states

**Go Representation** (mapped via slog):
```go
logger.Info("operation description",
    "operation", "pull",
    "subject", stackName,
    "status", "success",
    "metadata", map[string]any{
        "images": imageCount,
        "duration": duration,
    },
)
```

**Persistence**: Written to stdout (captured by container runtime/log aggregator)

**Relationships**:
- Multiple entries per UpdateCycle
- Multiple entries per StackUpdateResult
- Multiple entries per ImagePullResult
- Satisfies all logging requirements (FR-008 through FR-011, FR-019)

---

## Data Flow

```
SchedulerService
    ↓
[Cron Trigger]
    ↓
UpdateCycle (created)
    ↓
For each Stack:
    ↓
    StackUpdateResult (created)
        ↓
        For each Image:
            ↓
            ImagePullResult (created)
            Log: "pulling image X"
            Pull operation
            Log: "image X pulled"
        ↓
    If any images changed:
        Trigger Reconciliation
        Log: "reconciling stack"
    Log: "stack updated"
    ↓
[All stacks processed]
    ↓
Prune images
Log: "images pruned"
    ↓
UpdateCycle (completed)
Log: "cycle completed"
```

## State Management

**In-Memory State**:
- Current active UpdateCycle (if any)
- Scheduler running state
- Cron schedule parsed representation

**No Persistent State**:
- Update history is not stored (only logged)
- Cycles abandoned on restart (FR-18)
- State is reconstructible from logs if needed

**Concurrency Control**:
- `sync.Mutex` protects active cycle
- Only one cycle can run at a time
- New trigger terminates existing cycle (FR-20)

## Validation Rules

### Schedule Configuration
- Cron expression must parse successfully
- Must be 5-field format (minute hour day month weekday)
- Invalid expression → log error, use default

### Update Cycle
- StartTime must be set on creation
- CycleID must be unique UUID
- EndTime only set on completion
- Counts only increment, never decrement

### Stack Update Result
- StackName must not be empty
- PullStartTime must be before PullEndTime
- ContainersUpdated must be non-negative
- Success=false requires Error to be set

### Image Pull Result
- ImageName must not be empty
- NewDigest required if Success=true
- Changed can only be true if Success=true
- Duration must be non-negative

## Error Handling

**Stack-Level Errors**:
- Individual stack failures don't stop cycle
- Error captured in StackUpdateResult
- Logged as warning (not error)
- Next stack continues processing

**Cycle-Level Errors**:
- Critical errors stop cycle
- Examples: Docker daemon unreachable, out of disk space
- Logged as error
- Cycle marked as failed

**Network Errors**:
- Image pull failures due to network
- Logged as warning (FR-013)
- Stack marked as failed but cycle continues
- Retried on next cycle

## Performance Considerations

**Memory Usage**:
- UpdateCycle grows with stack count (linear)
- Typical: 100 stacks × 5 images = 500 ImagePullResults
- Estimate: ~50 bytes per result = 25KB per cycle
- Single cycle in memory at a time

**Computation**:
- No complex calculations
- Mostly I/O bound (Docker CLI commands)
- Sequential processing prevents resource spikes

## Schema Changes

**Version**: 1.0.0 (Initial)

**Future Extensions** (out of scope for this feature):
- Persistent update history
- Per-stack scheduling
- Configurable retry logic
- Selective stack updates
- Update windows/blackout periods

## Summary

All entities defined support the feature requirements:
- FR-001 through FR-020 (functional requirements)
- SC-001 through SC-008 (success criteria)
- Constitutional principles (single source of truth, safe operations)

Data model emphasizes:
- Simplicity (in-memory only)
- Observability (comprehensive logging)
- Resilience (error isolation)
- Performance (minimal overhead)
