# Data Model: Stack Sync and Reconciliation

## StackRecord (existing, extended)

Represents a desired stack and its runtime sync state.

Fields (existing + additions):
- `path` (string, required)
- `composeFile` (string, required)
- `composeHash` (string, required) — desired hash from Git for stack-level granularity (avoid reconciling unchanged stacks)
- `status` (enum, required) — `missing | syncing | synced | deleting | failed`
- `syncedRevision` (string, optional) — last synced Git revision
- `syncedCommitMessage` (string, optional) — commit message for the synced revision
- `syncedComposeHash` (string, optional) — last synced compose hash
- `syncedAt` (string, optional, RFC3339) — timestamp of last successful sync
- `lastSyncAt` (string, optional, RFC3339) — timestamp of last reconciliation outcome (success or failure), derived from `com.docker-cd.sync.at`
- `lastSyncStatus` (enum, optional) — `syncing | synced | failed`
- `lastSyncError` (string, optional) — last reconciliation error (truncated)

## StackSyncMetadata

Derived from container labels and mapped into `StackRecord` fields.

Fields:
- `stackPath`
- `desiredRevision`
- `desiredCommitMessage`
- `desiredComposeHash`
- `syncedAt`
- `lastSyncAt`
- `syncStatus`
- `syncError`

## ReconciliationRun

Tracks a single reconciliation attempt.

Fields:
- `stackPath`
- `desiredRevision`
- `desiredComposeHash`
- `startedAt`
- `finishedAt`
- `result` (success|failed|skipped)
- `error` (optional)

## ReconciliationPolicy

Configuration governing reconciliation behavior.

Fields:
- `enabled` (bool)
- `removeEnabled` (bool)
- `maxConcurrency` (int, fixed to 1)
