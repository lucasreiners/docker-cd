# Feature Specification: Stack Sync and Reconciliation

**Feature Branch**: `001-stack-sync`  
**Created**: 2026-02-15  
**Status**: Draft  
**Input**: User description: "Implement actual syncing/reconciliation to apply desired state from Git, tracking synced git revision and compose hash (container labeling optional)."

## User Scenarios & Testing *(mandatory)*

Automated tests are REQUIRED for each user story per the constitution.

### User Story 1 - Reconcile Desired vs Actual State (Priority: P1)

As an operator, I want the system to detect differences between the desired state (from Git) and the actual runtime state, so the running stacks are updated to match the desired configuration.

**Why this priority**: This is the core value of syncing; without reconciliation, the desired state cache is informational only.

**Independent Test**: Provide a desired state that differs from the current runtime state and verify that a reconciliation run updates the runtime to match and marks the stack as synced.

**Acceptance Scenarios**:

1. **Given** a stack with desired configuration that differs from the running state, **When** a reconciliation run occurs, **Then** the stack is updated to the desired configuration and its sync status becomes `synced`.
2. **Given** a stack already matching the desired configuration, **When** a reconciliation run occurs, **Then** the stack is not modified and its sync status remains `synced`.

---

### User Story 2 - Track Synced Metadata (Priority: P2)

As an operator, I want the system to record which desired revision and compose hash each running stack was synced to, so drift can be detected accurately after restarts.

**Why this priority**: Without durable sync metadata, drift detection after restarts is unreliable and can trigger unnecessary reconciliations.

**Independent Test**: After a reconciliation, restart the service and verify it can still determine the last synced revision and compose hash for each stack.

**Acceptance Scenarios**:

1. **Given** a stack was previously synced to a specific revision and compose hash, **When** the service restarts, **Then** the system can still determine the last synced revision and compose hash.
2. **Given** a stack has no recorded sync metadata, **When** drift is evaluated, **Then** the system treats the stack as out of sync.

---

### User Story 3 - Handle Stack Removal (Priority: P3)

As an operator, I want stacks that are removed from the desired state to be detected and cleaned up, so the runtime does not accumulate stale stacks.

**Why this priority**: Keeping the runtime aligned with desired state requires removing stacks that are no longer declared.

**Independent Test**: Remove a stack from desired state and verify a reconciliation run removes the corresponding runtime stack and marks it as `missing`.

**Acceptance Scenarios**:

1. **Given** a stack exists in runtime but not in the desired state, **When** reconciliation runs, **Then** the stack is removed and its status is set to `missing`.

---

### Edge Cases

- What happens when the runtime environment is unavailable during reconciliation?
- How does the system handle a reconciliation that fails partway through a multi-stack update?
- What happens when the desired state references a stack with invalid configuration?
- How does the system handle a stack that was partially removed outside the system?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST compare desired state against the current runtime state for each stack and determine whether reconciliation is required.
- **FR-002**: System MUST execute a reconciliation for stacks that are out of sync and update their status to `syncing` during the operation.
- **FR-003**: System MUST mark a stack as `synced` only after the runtime matches the desired configuration.
- **FR-004**: System MUST mark a stack as `failed` with an error message when reconciliation fails.
- **FR-005**: System MUST detect stacks present in runtime but not in desired state and remove them when reconciliation is enabled for removals.
- **FR-006**: System MUST record, per stack, the last synced desired revision and compose hash in a durable way across restarts.
- **FR-007**: System MUST expose updated sync status for each stack via the existing stacks endpoint.
- **FR-008**: System MUST provide a way to enable/disable reconciliation without disabling desired-state refresh.
- **FR-009**: System MUST log reconciliation start, success, and failure with stack identifiers.
- **FR-010**: System MUST support a safe no-op reconciliation when desired and actual states already match.
- **FR-011**: System MUST treat missing or invalid sync metadata as out-of-sync and reconcile.
- **FR-012**: System MUST preserve the desired-state cache even if reconciliation is disabled or fails.
- **FR-013**: System MUST store the reconciliation outcome (success/failure) with a timestamp for each stack.
- **FR-014**: System MUST only reconcile stacks within the configured deploy scope.
- **FR-015**: System MUST respect a maximum concurrent reconciliation limit of one stack at a time.
- **FR-016**: System MUST include a configuration option that controls whether stack removals are performed.
- **FR-017**: System MUST support reconciliation triggered by refresh events and manual refresh events.
- **FR-018**: System MUST ensure reconciliation decisions are based on the latest desired state snapshot.
- **FR-019**: System MUST avoid reconciling the same stack repeatedly when no desired-state change has occurred.
- **FR-020**: System MUST provide reconciliation progress visibility through stack status updates.

- **FR-021**: System MUST store sync metadata attached to running stack resources as metadata that survives service restarts.

### Key Entities *(include if feature involves data)*

- **Reconciliation Run**: A single attempt to align runtime state with desired state, including outcome, timestamps, and affected stacks.
- **Stack Sync Metadata**: Per-stack record of last synced desired revision, compose hash, sync timestamp, and last reconciliation result.
- **Runtime Stack**: The currently running stack instance associated with a desired stack path.
- **Reconciliation Policy**: Configuration that controls whether reconciliation runs, removal behavior, and concurrency.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 95% of reconciliation runs complete successfully within 3 minutes for a single stack.
- **SC-002**: Drift between desired and runtime state is detected within one refresh cycle.
- **SC-003**: After restart, 100% of stacks report the last synced revision and compose hash within 30 seconds.
- **SC-004**: In no-change scenarios, reconciliation performs no runtime modifications in at least 99% of runs.
- **SC-005**: Failed reconciliations are surfaced in stack status within 10 seconds of failure.

## Assumptions

- Desired state refresh continues to populate the cache from Git as implemented in feature 003.
- Reconciliation operates on a single stack at a time to reduce risk.
- Manual refresh and webhook refresh can trigger reconciliation when enabled.
