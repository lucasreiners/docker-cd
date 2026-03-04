# Phase 0 Research: Local Repository Clone

**Date**: 2026-03-04
**Feature**: 008-local-git-clone

## Decision 1: Local clone sync strategy

**Decision**: Use `fetch` + force-reset to the configured remote revision; if fetch/reset fails, delete the local clone and re-clone.

**Rationale**: Preserves the remote as the single source of truth while avoiding unnecessary full re-clones. The fallback path guarantees recovery from corruption or inconsistent state.

**Alternatives considered**:
- Always delete and re-clone on each refresh (too expensive for larger repos).
- `git pull --rebase` with conflict resolution (violates “remote is source of truth” and adds manual conflict handling).

## Decision 2: Clone depth

**Decision**: Use a full clone (no shallow depth) for the on-disk working copy.

**Rationale**: Long-lived shallow clones can fail during reset if the target commit is outside the shallow history. Full clones reduce edge cases and simplify recovery logic.

**Alternatives considered**:
- Shallow clone with periodic deepen (complexity and potential failure modes when revision changes).

## Decision 3: Storage location and lifecycle

**Decision**: Store the local working copy at a fixed in-container path (e.g., `/repo`) backed by a Docker volume. If the local clone is invalid, replace it in-place at the same path.

**Rationale**: A fixed path simplifies compose configuration and ensures reconcile/update tasks have a consistent filesystem location.

**Alternatives considered**:
- Configurable path via environment variable (adds configuration surface without a clear need).
- Ephemeral temp directories (breaks reconciliation needing a stable path).

## Decision 4: Refresh gating and task cancellation

**Decision**: When a refresh is triggered, cancel any running reconcile/update tasks and perform refresh immediately. Reconcile/update tasks must wait for the refresh triggered by the same event to complete successfully.

**Rationale**: Prevents race conditions between local clone updates and tasks that read from the clone. Ensures all operations use a consistent repository state.

**Alternatives considered**:
- Let running tasks finish before refresh (risk of stale data).
- Allow refresh concurrently (introduces race conditions and inconsistent reads).

## Decision 5: User notification on refresh failures

**Decision**: Expose refresh failures and “updates blocked” status through the existing refresh status API/SSE channels.

**Rationale**: Operators need immediate feedback when refresh fails, and UI needs a clear signal to communicate that updates are blocked.

**Alternatives considered**:
- Logging only (insufficient for UI feedback).
- Separate alerting endpoint (adds surface area without clear benefit).
