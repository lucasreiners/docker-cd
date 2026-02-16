# Research: Stack Sync and Reconciliation

## Decision 1: Sync metadata stored on containers

- Decision: Store sync metadata as labels on stack containers only.
- Rationale: Labels survive service restarts and are visible to the reconciler via `docker ps --format` without adding new storage.
- Alternatives considered: Local state only (lost on restart), labels on all resources (more complex and slower to inspect), single anchor resource (risk of missing per-service drift).

## Decision 2: Label schema for sync metadata

- Decision: Use the following labels on each container in a stack:
  - `com.docker-cd.stack.path`
  - `com.docker-cd.desired.revision`
  - `com.docker-cd.desired.commit_message`
  - `com.docker-cd.desired.compose_hash`
  - `com.docker-cd.synced.at` (RFC3339 timestamp)
  - `com.docker-cd.sync.at` (RFC3339 timestamp of last reconciliation outcome)
  - `com.docker-cd.sync.status` (synced|syncing|failed)
  - `com.docker-cd.sync.error` (truncated error message)
- Rationale: Labels allow drift detection and last-sync visibility after restarts.
- Alternatives considered: Store only revision/hash (missing timestamps and errors), or store a single composite label (harder to read and parse).

## Decision 3: Reconciliation apply strategy

- Decision: Use `docker compose -p <project> -f <composeFile> up -d` for create/update, and `docker compose -p <project> -f <composeFile> down --remove-orphans` for removal when removal is enabled.
- Rationale: `up -d` is idempotent for apply, while `down` is explicit for deletion and gated by a destructive opt-in.
- Alternatives considered: Always use `--remove-orphans` on apply (destructive without opt-in), or rely on `docker compose rm` (less consistent for project teardown).

## Decision 4: Project name derivation

- Decision: Derive a per-stack project name as `<projectName>-<stackPath>` with path separators replaced by `-`.
- Rationale: Ensures unique, stable compose project names across stacks while keeping names predictable.
- Alternatives considered: Use stack path only (possible collisions) or a hash-only name (hard to diagnose).

## Decision 5: Drift detection rules

- Decision: A stack is out of sync when container labels are missing or when labeled revision/hash differ from desired revision/hash.
- Rationale: Missing metadata is treated as drift per FR-011.
- Alternatives considered: Treat missing metadata as synced (risks hiding drift).
