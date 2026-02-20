# Implementation Plan: Stack Sync and Reconciliation

**Branch**: `004-stack-sync` | **Date**: 2026-02-15 | **Spec**: [specs/004-stack-sync/spec.md](specs/004-stack-sync/spec.md)
**Input**: Feature specification from `/specs/004-stack-sync/spec.md`

## Summary

Implement reconciliation that compares desired state (from Git) to runtime state, applies docker compose updates when drift is detected, and records per-stack sync metadata as container labels. Update stack status reporting to include sync metadata and reconciliation outcomes.

## Technical Context

**Language/Version**: Go 1.26.0  
**Primary Dependencies**: gin (HTTP), go-git (Git access)  
**Storage**: Docker container labels for sync metadata; in-memory desired-state store  
**Testing**: go test (unit), integration tests under `tests/integration`  
**Target Platform**: Linux container runtime (Docker)  
**Project Type**: Single Go service  
**Performance Goals**: Single-stack reconcile completes within 3 minutes (SC-001)  
**Constraints**: Destructive actions require opt-in; reconcile one stack at a time  
**Scale/Scope**: Single host, tens of stacks per deploy directory

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- GitOps source of truth: desired state remains Git-backed and reconciliation uses desired snapshot as source
- Continuous reconciliation: refresh triggers reconciliation; periodic refresh continues to drive reconciliation
- Container-first runtime: service remains containerized; config via env vars
- Safe compose apply: apply uses `docker compose up -d`; destructive removals gated by config
- Automated testing baseline: unit tests for diff/reconcile logic; integration tests for compose apply

## Project Structure

### Documentation (this feature)

```text
specs/004-stack-sync/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
cmd/docker-cd/
internal/
├── config/
├── desiredstate/
├── docker/
├── git/
├── http/
├── refresh/
└── reconcile/          # new package for stack reconciliation
tests/
└── integration/
```

**Structure Decision**: Single Go service with a new `internal/reconcile` package.

## Phase 0: Research

Output: [specs/004-stack-sync/research.md](specs/004-stack-sync/research.md)

- Define label schema for sync metadata stored on containers.
- Decide reconciliation apply strategy (`docker compose up -d` for apply, `down --remove-orphans` for removal with opt-in).
- Define per-stack compose project name derivation and drift rules.

## Phase 1: Design

Output:
- [specs/004-stack-sync/data-model.md](specs/004-stack-sync/data-model.md)
- [specs/004-stack-sync/contracts/openapi.yaml](specs/004-stack-sync/contracts/openapi.yaml)
- [specs/004-stack-sync/quickstart.md](specs/004-stack-sync/quickstart.md)

Design decisions:
- Extend `StackRecord` to include sync metadata and add a `failed` sync status.
- Introduce a reconciliation policy config with enable/disable and removal flags.
- Map container labels to sync metadata in `StackRecord` for `/api/stacks`.

### Constitution Check (Post-Design)

- GitOps source of truth: labels used only as sync metadata; desired state remains Git-derived
- Continuous reconciliation: reconcile triggered after refresh events
- Container-first runtime: no new external storage; metadata labels on containers
- Safe compose apply: removals gated by `RECONCILE_REMOVE_ENABLED`
- Automated testing baseline: reconciliation logic + compose apply tests in plan

## Phase 2: Implementation Plan

1. **Configuration**
   - Add `RECONCILE_ENABLED`, `RECONCILE_REMOVE_ENABLED`, and drift policy config to config.
   - Update config tests and docker-compose/README documentation.

2. **Desired State Model Updates**
   - Extend `StackSyncStatus` to include `failed`.
   - Add sync metadata fields to `StackRecord` and update store copy helpers.
   - Update `/api/stacks` handler to include sync metadata fields.

3. **Runtime Inspection**
   - Add docker client helpers to list containers by labels and extract sync metadata.
   - Add helper to derive compose project name from stack path and `ProjectName`.

4. **Reconciler Service**
   - Create `internal/reconcile` package with a reconciler that:
     - Computes drift from desired snapshot vs container labels.
   - Applies drift policy (revert vs flag + acknowledgement) before reconciling.
   - Records acknowledgement state per stack path and exposes an acknowledgement endpoint.
     - Runs `docker compose up -d` with a generated override file that adds labels.
     - Runs `docker compose down --remove-orphans` for deleted stacks when removal is enabled.
   - Enforces deploy scope filtering and max concurrency of one stack at a time.
     - Updates `StackRecord` status to `syncing`, `synced`, or `failed` with timestamps.

5. **Refresh Integration**
   - Trigger reconciliation after successful refresh (startup, periodic, webhook, manual).
   - Ensure reconciliation uses the latest snapshot and avoids reapplying unchanged stacks.

6. **Testing**
   - Unit tests for drift detection, label parsing, and status transitions.
   - Integration tests for compose apply and removal with real Docker (tagged).
   - Regression test for no-op reconciliation when desired matches actual.

## Complexity Tracking

No constitution violations.
