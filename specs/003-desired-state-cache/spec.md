# Feature Specification: Desired State Cache and Refresh

**Feature Branch**: `003-desired-state-cache`  
**Created**: 2026-02-15  
**Status**: Draft  
**Input**: User description: "Feature 003: cache desired git-defined state; refresh on app start, webhook URL, and refresh API endpoint; fetch latest files from git config on refresh; maintain datastore for desired/actual comparisons (hash docker-compose files) and track deployment status (missing/syncing/synced/deleting)."

## User Scenarios & Testing *(mandatory)*

Automated tests are REQUIRED for each user story per the constitution.

### User Story 1 - Build Desired State on Startup (Priority: P1)

As an operator, I want the application to load the desired state from Git at startup so I always begin with an accurate picture of what should be deployed.

**Why this priority**: Without a reliable desired state on startup, the system cannot make safe deployment decisions or status reporting.

**Independent Test**: Start the service with a known repo and verify the desired-state cache contains discovered stacks, hashes, and default statuses.

**Acceptance Scenarios**:

1. **Given** a valid Git configuration and a repository containing docker-compose files, **When** the service starts, **Then** it refreshes the desired state and stores a hash for each discovered stack.
2. **Given** a valid Git configuration but no docker-compose files in the deploy directory, **When** the service starts, **Then** the desired-state cache is empty and startup still succeeds.

---

### User Story 2 - Refresh Desired State via Webhook (Priority: P2)

As an operator, I want the system to accept a webhook call so that Git pushes can trigger a desired-state refresh without restarting the service.

**Why this priority**: Webhook refresh enables near-real-time updates after repository changes.

**Independent Test**: Call the webhook endpoint and verify the desired-state cache updates to the latest revision.

**Acceptance Scenarios**:

1. **Given** a running service with an existing cache, **When** a valid webhook request is received, **Then** the system refreshes the desired state from the latest Git revision.
2. **Given** a refresh is already in progress, **When** another webhook request arrives, **Then** the system does not start a second refresh and reports that a refresh is already running.

---

### User Story 3 - Refresh on Demand and Track Stack Sync Status (Priority: P3)

As an operator, I want a manual refresh endpoint and per-stack sync status so I can trigger updates and see if stacks are missing, syncing, synced, or deleting.

**Why this priority**: Manual refresh and visible sync status enable controlled operations and troubleshooting.

**Independent Test**: Trigger the manual refresh endpoint and verify sync status records are stored for each stack with the expected defaults.

**Acceptance Scenarios**:

1. **Given** a running service, **When** I call the manual refresh endpoint, **Then** the desired-state cache refreshes and each discovered stack has a stored sync status.
2. **Given** a refresh completes, **When** a stack is newly discovered, **Then** its initial status is set to "missing".

---

### Edge Cases

- What happens when Git is temporarily unreachable during a refresh?
- How does the system handle a webhook refresh request with an invalid signature?
- What happens when a stack directory is removed from Git between refreshes?
- How does the system behave if a refresh is triggered while another refresh is already running?

## Clarifications

### Session 2026-02-15

- Q: What authentication or verification method should be required for refresh triggers? → A: Webhook endpoint uses a configurable secret and validates GitHub HMAC signatures (X-Hub-Signature-256); all other user-facing endpoints (including manual refresh) have no auth for now.
- Q: Which files should be hashed to detect desired-state changes? → A: Only docker-compose.yml and docker-compose.yaml in each stack directory.
- Q: How should concurrent refresh requests be handled? → A: Allow one queued refresh while one is running; if another request arrives while one is queued, replace the queued refresh with the newest request.
- Q: Should deployment status be tracked per stack or globally? → A: Per stack only.
- Q: What should refresh endpoints return, and should we store the synced Git revision? → A: Refresh responses use status values refreshing, queued, completed, failed (no "already-queued"), and the cache stores the Git revision that was synced.
- Q: What response format should endpoints use? → A: All endpoints except the existing root endpoint must use JSON responses.
- Q: Should the system periodically refresh desired state? → A: Yes, add a configurable poll interval to refresh desired state even without webhook or manual triggers.
- Q: What does "refreshing" mean versus "syncing"? → A: Refreshing is a system-wide Git fetch/cache update; syncing is a per-stack operation that applies desired state to the runtime.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST refresh the desired state from the configured Git repository on every application start.
- **FR-002**: System MUST expose a webhook endpoint that triggers a desired-state refresh when called.
- **FR-003**: System MUST expose a manual refresh endpoint that triggers a desired-state refresh on demand.
- **FR-004**: System MUST store a desired-state cache that includes each stack's path and a deterministic hash of docker-compose.yml or docker-compose.yaml contents in that stack directory.
- **FR-005**: System MUST maintain a per-stack sync status (no global sync status) with the allowed values: missing, syncing, synced, deleting.
- **FR-006**: System MUST support a webhook secret configured via environment variable and validate GitHub HMAC signatures using the X-Hub-Signature-256 header; requests with missing or invalid signatures MUST be rejected.
- **FR-007**: Manual refresh requests and other user-facing endpoints MUST NOT require authentication for this feature.
- **FR-008**: System MUST allow only one refresh to run at a time and maintain a single-slot queue for one pending refresh; if a refresh is already queued, newer requests replace the queued refresh.
- **FR-009**: System MUST update the desired-state cache to reflect additions, removals, and changes in the repository at each refresh.
- **FR-010**: System MUST expose a refresh status endpoint that returns the latest refresh outcome (success/failure and timestamp) and the cached Git revision.
- **FR-011**: System MUST keep existing sync status values when a stack remains present across refreshes.
- **FR-012**: System MUST store the Git revision that the desired-state cache was synced to and expose it for user inspection.
- **FR-013**: System MUST return JSON responses for all endpoints except the existing root endpoint.
- **FR-016**: System MUST use explicit log severity levels (info/warn/error) for refresh operations and webhook validation outcomes.
- **FR-014**: System MUST support a configurable poll interval and perform periodic desired-state refreshes even without webhook or manual triggers.
- **FR-015**: System MUST expose a stacks endpoint that returns all stacks with their sync status.

### Key Entities *(include if feature involves data)*

- **DesiredStateSnapshot**: Represents a single refresh result with timestamp, revision, and collection of stack records (system-wide Git refresh state).
- **StackRecord**: Represents a stack's path, docker-compose hash, and sync status.
- **RefreshTrigger**: Represents a refresh request source (startup, webhook, manual) and timestamp.

### Assumptions

- Each stack is identified by the directory containing a docker-compose file.
- The desired-state cache can be rebuilt from Git on startup and does not need to survive application restarts.

### Dependencies

- Existing Git configuration and validation from Feature 002 remain the source of repository access details.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A startup refresh completes successfully for a valid repository in under 30 seconds for a repo containing up to 200 stack directories.
- **SC-002**: Webhook-triggered refreshes update the desired-state cache within 60 seconds of receipt for 95% of requests.
- **SC-003**: Manual refresh requests return a clear success or in-progress response in under 2 seconds.
- **SC-004**: 100% of discovered stacks have a stored hash and sync status after each successful refresh.
