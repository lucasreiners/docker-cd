# Data Model: Local Repository Clone

**Date**: 2026-03-04
**Feature**: 008-local-git-clone

## Entities

### LocalWorkingCopy

Represents the persistent local clone stored at a fixed path.

**Fields**:
- `path` (string): Fixed filesystem path, e.g., `/repo`.
- `revision` (string): The currently checked-out revision (commit hash or ref).
- `ref` (string): Configured branch/ref name.
- `lastRefreshedAt` (datetime): Timestamp of last successful refresh.
- `status` (enum): `ready`, `refreshing`, `failed`.
- `lastError` (string, optional): Error message for the last failed refresh.

**Relationships**:
- Used by `RefreshSnapshot` for reporting.
- Read by `ReconcileRun` and `UpdateCycle` to locate stack files.

---

### RefreshSnapshot (extends existing)

Represents the current refresh status and visibility to UI.

**Existing fields**: `revision`, `commitMessage`, `ref`, `refType`, `refreshedAt`, `refreshStatus`, `refreshError`.

**New fields**:
- `updatesBlocked` (bool): Indicates reconcile/update tasks are blocked.
- `blockedReason` (string, optional): Reason for blocking (e.g., last refresh failure).
- `localPath` (string, optional): Fixed local clone path for diagnostics.

---

### RefreshEvent

Represents a refresh trigger and outcome.

**Fields**:
- `trigger` (enum): `startup`, `manual`, `webhook`, `periodic`.
- `startedAt` (datetime)
- `finishedAt` (datetime, optional)
- `result` (enum): `success`, `failed`, `canceled`.
- `error` (string, optional)

---

### TaskGate

Gates reconcile/update tasks on refresh completion.

**Fields**:
- `activeRefreshTrigger` (enum, optional): Current refresh trigger.
- `refreshComplete` (bool): Whether the current refresh completed successfully.
- `blocked` (bool): Indicates tasks must not run.
- `blockedReason` (string, optional)

---

## State Transitions

### Refresh lifecycle

- `idle` -> `refreshing` (triggered by startup/manual/webhook/periodic)
- `refreshing` -> `completed` (success)
- `refreshing` -> `failed` (error)
- `refreshing` -> `canceled` (if higher-priority refresh is triggered)

### Task gating

- `blocked=false` when refresh succeeded for the trigger.
- `blocked=true` when refresh fails (until next successful refresh).
- On refresh trigger, reconcile/update tasks are canceled and blocked until refresh completes.
