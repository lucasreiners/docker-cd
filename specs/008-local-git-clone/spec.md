# Feature Specification: Local Repository Clone

**Feature Branch**: `008-local-git-clone`  
**Created**: March 4, 2026  
**Status**: Draft  
**Input**: User description: "Based on the previous findings, I want to create a new feature, where the remote git repository is cloned in a local docker volume. The remote repository is always the single source of truth. No pushing from local to remote. When there is a diff or merge conflicts between local and remote, always replace the local repo with the remote contents. The local clone should be refreshed on application start, as well as, when the refresh button is used or a webhook event is received. that local clone should be used for reconciliation as well as container update tasks."

## Clarifications

### Session 2026-03-04

- Q: If refresh fails, what should happen to the local clone and update flows? -> A: Keep the last successful local clone and block reconcile/update until the next successful refresh.
- Q: Where should the local working copy live inside the container? -> A: Fixed path inside the container (for example, /repo).
- Q: What should happen if a refresh is triggered while reconcile/update is running? -> A: Cancel the running reconcile/update and refresh immediately.
- Q: How should refresh sync the local working copy with the remote? -> A: Fetch and force-reset to the remote revision, falling back to delete-and-reclone on failure.

## User Scenarios & Testing *(mandatory)*

<!--
  IMPORTANT: User stories should be PRIORITIZED as user journeys ordered by importance.
  Each user story/journey must be INDEPENDENTLY TESTABLE - meaning if you implement just ONE of them,
  you should still have a viable MVP (Minimum Viable Product) that delivers value.
  
  Assign priorities (P1, P2, P3, etc.) to each story, where P1 is the most critical.
  Think of each story as a standalone slice of functionality that can be:
  - Developed independently
  - Tested independently
  - Deployed independently
  - Demonstrated to users independently
-->

### User Story 1 - Startup Uses Local Clone (Priority: P1)

As an operator, I want the service to keep a local working copy of the remote repository so that reconcile and update tasks always read from a reliable local source.

**Why this priority**: Without a local working copy, reconcile and update tasks can fail due to missing paths or inconsistent working directories.

**Independent Test**: Can be fully tested by starting the service and verifying it creates or refreshes the local working copy and uses it for reconcile/update tasks.

**Acceptance Scenarios**:

1. **Given** the service starts with valid remote repository access, **When** startup refresh completes, **Then** the local working copy matches the configured remote revision and is used for reconciliation and updates.
2. **Given** the local working copy is missing at startup, **When** the service initializes, **Then** it creates a fresh local working copy from the remote source.

---

### User Story 2 - Manual and Webhook Refresh (Priority: P2)

As an operator, I want the local working copy to refresh when I trigger a refresh or when a webhook is received, so the system stays aligned to the remote repository.

**Why this priority**: Refresh is a primary control surface for operators and should ensure the local working copy is current.

**Independent Test**: Can be fully tested by triggering a refresh and confirming the local working copy updates to the remote state without restarting.

**Acceptance Scenarios**:

1. **Given** a manual refresh is triggered, **When** the refresh completes, **Then** the local working copy matches the remote repository contents for the configured revision.
2. **Given** a webhook refresh is received, **When** the refresh completes, **Then** reconcile and update tasks use the refreshed local working copy.

---

### User Story 3 - Local Divergence Recovery (Priority: P3)

As an operator, I want the system to replace a diverged or conflicting local working copy with the remote contents so the remote remains the single source of truth.

**Why this priority**: Prevents local drift or corruption from affecting reconcile or update tasks.

**Independent Test**: Can be fully tested by forcing a local divergence and verifying the local working copy is replaced by remote contents.

**Acceptance Scenarios**:

1. **Given** the local working copy differs from the remote contents, **When** a refresh runs, **Then** the local working copy is replaced with the remote contents.
2. **Given** the local working copy has conflicts or is corrupted, **When** a refresh runs, **Then** the local working copy is discarded and replaced by the remote contents.

---

[Add more user stories as needed, each with an assigned priority]

### Edge Cases

- Remote repository is temporarily unreachable during refresh.
- Refresh is triggered while another refresh is in progress.
- Local working copy exists but is missing required stack directories.
- Webhook is received with an invalid or unauthorized signature.

## Requirements *(mandatory)*

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right functional requirements.
-->

### Functional Requirements

- **FR-001**: System MUST maintain a local working copy of the configured remote repository in persistent storage attached to the service.
- **FR-002**: System MUST refresh the local working copy on application start.
- **FR-003**: System MUST refresh the local working copy when a manual refresh is triggered.
- **FR-004**: System MUST refresh the local working copy when a valid webhook refresh event is received.
- **FR-005**: System MUST treat the remote repository as the single source of truth and MUST NOT push changes from the local working copy back to the remote.
- **FR-006**: When the local working copy differs from the remote contents, the system MUST replace the local working copy with the remote contents.
- **FR-007**: When the local working copy is corrupted or conflicts are detected, the system MUST replace the local working copy with the remote contents.
- **FR-008**: Reconciliation MUST read stack files from the local working copy.
- **FR-009**: Container update tasks MUST read stack files from the local working copy.
- **FR-010**: System MUST surface a clear refresh failure status and error message to the user interface when the remote repository cannot be reached.
- **FR-011**: System MUST prevent concurrent refresh operations from corrupting the local working copy.
- **FR-012**: The feature MUST be validated with integration tests, including docker-in-docker coverage for reconcile and update flows.
- **FR-013**: The local and production docker-compose files MUST be updated to include the new persistent volume definition for the local working copy.
- **FR-014**: Reconciliation and update tasks MUST NOT start until the local working copy refresh completes successfully for the same trigger (startup, manual refresh, or webhook), preventing race conditions.
- **FR-015**: If a refresh fails, the system MUST keep the last successful local working copy, block reconcile and update tasks, and notify the user interface that updates are blocked.
- **FR-016**: The local working copy MUST be stored at a fixed path inside the container (for example, /repo) and mounted via the persistent volume.
- **FR-017**: If a refresh is triggered while reconcile or update tasks are running, the system MUST cancel those tasks and perform the refresh immediately.
- **FR-018**: Refresh MUST sync the local working copy by fetching and force-resetting to the remote revision; if that fails, the system MUST delete the local working copy and re-clone from the remote.

### Scope

- The local working copy is intended only for read and refresh operations.
- The feature applies to the configured repository and revision only.
- There is no requirement to support multiple repositories in a single instance.

### Assumptions

- Persistent storage is provided to the service so the local working copy survives restarts.
- The remote repository is reachable using existing access credentials.
- The refresh triggers (startup, manual, webhook) already exist as control surfaces.

### Dependencies

- Remote repository access credentials and revision configuration are valid.

### Key Entities *(include if feature involves data)*

- **Local Working Copy**: The persistent local repository state used by reconcile and update tasks; attributes include repository source, revision, last refreshed time, and health status.
- **Refresh Event**: A trigger for refreshing the local working copy; attributes include trigger type (startup, manual, webhook), timestamp, and outcome.
- **Remote Repository**: The authoritative source; attributes include repository URL and configured revision.

## Success Criteria *(mandatory)*

<!--
  ACTION REQUIRED: Define measurable success criteria.
  These must be technology-agnostic and measurable.
-->

### Measurable Outcomes

- **SC-001**: 99% of refresh operations complete successfully within 60 seconds for repositories up to 200 MB.
- **SC-002**: After each refresh, the local working copy matches the configured remote revision with 100% accuracy.
- **SC-003**: Reconcile and update tasks read from the local working copy in 100% of runs.
- **SC-004**: Refresh failure events are reported within 5 seconds of detection.
