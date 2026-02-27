# Implementation Plan: Scheduled Stack Updates

**Branch**: `006-scheduled-stack-updates` | **Date**: February 27, 2026 | **Spec**: [specs/006-scheduled-stack-updates/spec.md](specs/006-scheduled-stack-updates/spec.md)
**Input**: Feature specification from `/specs/006-scheduled-stack-updates/spec.md`

## Summary

Implement automatic periodic updates for all managed compose stacks on a configurable cron schedule. Each update cycle pulls latest images for all stacks, recreates containers when images change, and prunes unused images system-wide. Default schedule: 3 AM UTC daily. Comprehensive logging captures all operations with timestamps and outcomes.

## Technical Context

**Language/Version**: Go 1.26.0  
**Primary Dependencies**: gin (HTTP), go-git (Git access), Docker CLI via CommandRunner abstraction  
**Storage**: Docker container labels for stack metadata; in-memory desired-state store  
**Testing**: go test (unit), integration tests under `tests/integration` with testcontainers  
**Target Platform**: Linux container runtime (Docker)
**Project Type**: Long-lived containerized service  
**Performance Goals**: Update cycles complete within 30 minutes for all stacks (SC-001)  
**Constraints**: Zero service interruption during updates (SC-002); sequential stack processing to avoid resource contention (FR-016)  
**Scale/Scope**: Single host deployment managing tens of compose stacks, daily update cycles

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Initial Check (Pre-Phase 0)

- **I. GitOps Source of Truth**: ✅ PASS - Scheduled updates operate on stacks already managed by Git-backed desired state. Updates pull images and recreate containers but do not modify Git state.
- **II. Continuous Reconciliation**: ✅ PASS - Update scheduler complements existing webhook-based and poll-based reconciliation. Updates trigger reconciliation after image pulls.
- **III. Container-First Runtime**: ✅ PASS - New configuration options (cron expression, enable/disable flag) will be provided via environment variables following existing patterns.
- **IV. Safe Docker Compose Apply**: ✅ PASS - Uses existing `docker compose` reconciliation logic. Image pulls and container updates use standard Docker CLI commands. Image pruning is explicit and logged.
- **V. Automated Testing Baseline**: ✅ PASS - Will add unit tests for scheduler logic and integration tests for update cycle operations following existing test patterns.

### Post-Design Check (After Phase 1)

- **I. GitOps Source of Truth**: ✅ PASS - Confirmed: Design uses existing desired-state store, no Git modifications
- **II. Continuous Reconciliation**: ✅ PASS - Confirmed: Scheduler service reuses existing Reconciler, maintains idempotency
- **III. Container-First Runtime**: ✅ PASS - Confirmed: UpdaterConfig added to Config struct, environment-based, documented contracts
- **IV. Safe Docker Compose Apply**: ✅ PASS - Confirmed: Uses ComposeRunner interface, sequential processing avoids conflicts
- **V. Automated Testing Baseline**: ✅ PASS - Confirmed: Research identifies test strategy, integration tests planned with testcontainers

**Result**: All constitutional principles satisfied in both initial design and detailed design.

## Project Structure

### Documentation (this feature)

```text
specs/006-scheduled-stack-updates/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
backend/
├── cmd/docker-cd/
│   └── main.go          # Wire up scheduler service
├── internal/
│   ├── config/
│   │   └── config.go    # Add scheduler config fields
│   ├── scheduler/       # NEW: Cron-based update scheduler
│   │   ├── scheduler.go # Scheduler service and cron parsing
│   │   ├── updater.go   # Update cycle execution logic
│   │   └── scheduler_test.go
│   ├── docker/
│   │   ├── client.go    # Add image pull and prune methods
│   │   └── images.go    # NEW: Image operations
│   ├── reconcile/       # Reuse existing reconciliation
│   └── desiredstate/    # Use existing state store
└── tests/
    └── integration/
        └── scheduler_test.go  # NEW: Integration tests for update cycles
```

**Structure Decision**: Single Go service with new `internal/scheduler` package for cron scheduling and update cycle coordination. Reuses existing `reconcile` package for stack updates and `docker` package for image operations.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations identified. All constitution principles are satisfied by this design.

## Phase 0: Research

**Status**: ✅ Complete

**Output**: [research.md](research.md)

**Key Decisions**:
1. **Cron Library**: Selected `github.com/robfig/cron/v3` for parsing and scheduling
2. **Image Pull Strategy**: Sequential pulls via `docker compose pull` per stack
3. **Image Pruning**: System-wide prune via `docker image prune -af`
4. **Update Coordination**: Sequential state machine with explicit phases
5. **Scheduler Lifecycle**: Independent service with graceful shutdown
6. **Logging Strategy**: Structured logging with slog at multiple levels
7. **Configuration**: Environment variables following existing pattern (disabled by default)
8. **Change Detection**: Compare image digests via Docker inspect

**Dependencies Added**: `github.com/robfig/cron/v3`

## Phase 1: Design

**Status**: ✅ Complete

**Outputs**:
- [data-model.md](data-model.md) - Entities and data structures
- [contracts/config.md](contracts/config.md) - Configuration interface contract
- [contracts/log-format.md](contracts/log-format.md) - Structured log format contract
- [quickstart.md](quickstart.md) - User setup guide

**Entities Defined**:
1. **Update Schedule Configuration** - Cron expression and enabled state
2. **Update Cycle** - Single execution with timing and statistics
3. **Stack Update Result** - Per-stack outcome with image details
4. **Image Pull Result** - Per-image pull status and change detection
5. **Update Operation Log Entry** - Structured log records

**Contracts Established**:
- Configuration via `UPDATER_ENABLED` and `UPDATER_CRON` environment variables
- Structured log format (text and JSON) for all update operations
- Log-based observability with specific events for monitoring

**Agent Context Updated**: GitHub Copilot context file updated with Go 1.26.0 and technology stack

## Next Steps

**Phase 2: Task Breakdown** - Run `/speckit.tasks` to generate implementation tasks

The implementation plan is complete and ready for task generation. All design decisions have been documented, interfaces defined, and constitutional compliance verified.

## Summary

**Feature**: Scheduled Stack Updates  
**Branch**: 006-scheduled-stack-updates  
**Planning Status**: ✅ Complete (Phases 0-1)

**What Was Designed**:
- Cron-based scheduler service integrated into main application
- Sequential update cycle: pull images → update stacks → prune images
- Comprehensive structured logging for observability
- Environment-based configuration (disabled by default)
- Reuses existing reconciliation and Docker infrastructure

**Key Technical Choices**:
- `robfig/cron/v3` for cron scheduling
- In-memory state (no persistence)
- System-wide image pruning
- Sequential processing for safety
- Logging-only notifications

**Constitutional Compliance**: All 5 principles satisfied

**Artifacts Generated**:
- ✅ plan.md (this file)
- ✅ research.md (8 decisions)
- ✅ data-model.md (5 entities)
- ✅ contracts/config.md
- ✅ contracts/log-format.md
- ✅ quickstart.md
- ✅ .github/agents/copilot-instructions.md (updated)

**Ready For**: Implementation task breakdown (`/speckit.tasks`)
