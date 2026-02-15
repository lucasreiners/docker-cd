# Research: Desired State Cache and Refresh

## Decision 1: Use go-git tree reads for compose content

**Decision**: Use go-git with in-memory storage to resolve the configured revision and read compose file blobs directly from commit trees.
**Rationale**: Keeps refresh read-only, avoids working-tree side effects, and matches existing git usage in the project.
**Alternatives considered**: Shelling out to `git` CLI to read files; rejected to avoid external process dependency and to keep logic in-process.

## Decision 2: Hash only docker-compose.yml or docker-compose.yaml

**Decision**: Compute a deterministic SHA-256 hash from the contents of docker-compose.yml or docker-compose.yaml in each stack directory (prefer docker-compose.yml when both exist, otherwise docker-compose.yaml).
**Rationale**: Matches the clarified scope and avoids false diffs from non-compose files.
**Alternatives considered**: Hash all files in the stack directory or include override files; rejected because the feature scope is compose-only.

## Decision 3: In-memory desired-state cache with single-slot refresh queue

**Decision**: Store desired state in memory behind a read/write lock and manage refresh requests with a single-slot queue (one in-flight refresh, one queued refresh that can be replaced).
**Rationale**: Keeps the initial implementation lightweight and aligns with the queue semantics in the spec.
**Alternatives considered**: Persistent storage (SQLite) or full queueing; rejected to keep scope and complexity small.

## Decision 4: Periodic refresh via configurable poll interval

**Decision**: Add a poll interval configuration and run periodic refreshes using a background ticker.
**Rationale**: Satisfies the continuous reconciliation principle and the clarified requirement for periodic refresh.
**Alternatives considered**: Webhook-only refresh; rejected to avoid missed updates.
