# Implementation Plan: Local Repository Clone

**Branch**: `008-local-git-clone` | **Date**: 2026-03-04 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/008-local-git-clone/spec.md`

## Summary

Introduce a persistent local working copy of the remote Git repository stored in a Docker volume at a fixed path (for example, `/repo`). Refresh the local clone on startup and on manual/webhook refresh, and enforce gating so reconcile/update tasks only run after the refresh for the same trigger succeeds. If a refresh fails, keep the last successful clone, block reconcile/update, and notify the UI via refresh status. Refresh sync uses fetch + force reset with a fallback to delete-and-reclone. The compose files are updated to mount the volume, and integration tests (including docker-in-docker) validate reconcile and update flows.

## Technical Context

**Language/Version**: Go 1.26.0
**Primary Dependencies**: gin (HTTP), go-git (Git access), Docker CLI via CommandRunner, testcontainers-go (integration)
**Storage**: Filesystem-backed Git clone in a Docker volume; in-memory desired-state store
**Testing**: `go test` unit tests; integration tests under `backend/tests/integration` (docker-in-docker)
**Target Platform**: Linux container runtime (Docker)
**Project Type**: Long-lived containerized service with web UI
**Performance Goals**: Refresh completes within 60 seconds for repositories up to 200 MB (SC-001)
**Constraints**: Remote repo is source of truth; no push from local; reconcile/update must not race refresh; fixed clone path
**Scale/Scope**: Single repository per instance; tens of stacks per repo

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Initial Check (Pre-Phase 0)

- **I. GitOps Source of Truth**: ✅ PASS - Local clone is read-only and always reset from the remote; no local pushes.
- **II. Continuous Reconciliation**: ✅ PASS - Refresh still triggers reconciliation; gating ensures consistency without removing webhook or polling behavior.
- **III. Container-First Runtime**: ✅ PASS - Uses a Docker volume and fixed in-container path; no host-only assumptions.
- **IV. Safe Docker Compose Apply**: ✅ PASS - Reconciliation continues to use `docker compose` and is gated by a successful refresh.
- **V. Automated Testing Baseline**: ✅ PASS - Plan includes new integration tests (docker-in-docker) and unit coverage.

### Post-Design Re-Evaluation (Phase 1 Complete)

- **I. GitOps Source of Truth**: ✅ PASS - Design uses fetch + force reset with re-clone fallback; remote remains authoritative.
- **II. Continuous Reconciliation**: ✅ PASS - Reconcile still triggers after refresh; cancellation handles race conditions safely.
- **III. Container-First Runtime**: ✅ PASS - Persistent volume and fixed path `/repo` are encoded in compose updates.
- **IV. Safe Docker Compose Apply**: ✅ PASS - No change to compose apply semantics, only input source path.
- **V. Automated Testing Baseline**: ✅ PASS - Integration tests specified for refresh + reconcile + update flows.

**Result**: All constitution principles satisfied.

## Project Structure

### Documentation (this feature)

```text
specs/008-local-git-clone/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── api.md
│   └── deployment.md
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
backend/
├── cmd/docker-cd/
│   └── main.go                  # Wire local clone refresh on startup
├── internal/
│   ├── config/
│   │   └── config.go             # No new config env vars; document fixed clone path
│   ├── git/
│   │   ├── reader.go             # Existing read-only in-memory path
│   │   ├── validator.go          # Existing repo validation
│   │   └── local_clone.go         # [NEW] On-disk clone + refresh helper
│   ├── refresh/
│   │   ├── refresh.go             # Ensure refresh uses local clone and gates reconcile
│   │   └── queue.go               # Enforce refresh ordering and cancellation
│   ├── reconcile/
│   │   └── reconcile.go           # Gate/cancel reconcile when refresh runs
│   ├── scheduler/
│   │   └── scheduler.go           # Gate/cancel update cycles during refresh
│   ├── http/
│   │   └── handler.go             # Surface refresh blocked status to UI
│   └── desiredstate/
│       └── state.go               # Store refresh metadata and blocking flags
└── tests/
    └── integration/
        ├── dind_reconcile_test.go # Extend for local clone refresh scenarios
        └── dind_update_test.go    # [NEW] Update flow integration coverage

docker-compose.yml

docker-compose.local.yaml
```

**Structure Decision**: Single Go service. Add a new helper under `internal/git` for on-disk clone management, wire refresh to use it, and surface status via existing HTTP/SSE pathways. Update root compose files to mount the volume at `/repo`.

## Complexity Tracking

No constitution violations identified. This section is not required.

## Phase 0: Research

**Status**: ✅ Complete

**Output**: [research.md](research.md)

**Key Decisions**:
1. Fetch + force reset with delete-and-reclone fallback
2. Full clone (no shallow depth) for long-lived working copies
3. Fixed clone path backed by a Docker volume
4. Refresh cancels reconcile/update and gates until success
5. UI notification via refresh status API/SSE

## Phase 1: Design

**Status**: ✅ Complete

**Outputs**:
- [data-model.md](data-model.md)
- [contracts/api.md](contracts/api.md)
- [contracts/deployment.md](contracts/deployment.md)
- [quickstart.md](quickstart.md)

**Entities Defined**:
1. LocalWorkingCopy
2. RefreshSnapshot (extended)
3. RefreshEvent
4. TaskGate

**Contracts Established**:
- Refresh status API/SSE extended with `updatesBlocked` and `blockedReason`
- Deployment requirement for `/repo` volume

**Agent Context Updated**: ✅ Completed (`.specify/scripts/bash/update-agent-context.sh copilot`)

## Next Steps

**Phase 2: Task Breakdown** - Run `/speckit.tasks` to generate implementation tasks.

**Plan Status**: ✅ Ready for task generation
