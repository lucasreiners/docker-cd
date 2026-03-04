---

description: "Task list for 008-local-git-clone"

---

# Tasks: Local Repository Clone

**Input**: Design documents from `/specs/008-local-git-clone/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/
**Tests**: Required (integration tests requested, including docker-in-docker)

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Deployment scaffolding for persistent local clone

- [x] T001 Update docker-compose.yml to add repo volume mount at /repo in docker-compose.yml
- [x] T002 Update docker-compose.local.yaml to add repo volume mount at /repo in docker-compose.local.yaml

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core state and API surface shared across all user stories

- [x] T003 Extend refresh status model with updatesBlocked, blockedReason, localPath in backend/internal/desiredstate/state.go
- [x] T004 Update refresh status API response to include new fields in backend/internal/http/handler.go
- [x] T005 Update refresh status SSE payload to include new fields in backend/internal/desiredstate/broadcaster.go

**Checkpoint**: Refresh status can represent blocked state and local clone path

---

## Phase 3: User Story 1 - Startup Uses Local Clone (Priority: P1) 🎯 MVP

**Goal**: Create and use a persistent local clone on startup, and gate reconcile/update tasks on refresh completion.

**Independent Test**: Start the service with valid repo access and verify the local clone is created at /repo and reconcile/update only run after a successful startup refresh.

### Tests for User Story 1 (Required)

- [x] T006 [P] [US1] Unit tests for LocalClone open/clone and fetch/reset in backend/internal/git/local_clone_test.go
- [x] T007 [P] [US1] Integration test for startup refresh + reconcile gating in backend/tests/integration/dind_reconcile_test.go

### Implementation for User Story 1

- [x] T008 [US1] Implement LocalClone helper for on-disk clone management in backend/internal/git/local_clone.go
- [x] T009 [US1] Wire startup refresh to use LocalClone in backend/internal/refresh/refresh.go
- [x] T010 [US1] Gate reconcile on successful refresh completion in backend/internal/reconcile/reconcile.go
- [x] T011 [US1] Gate update cycles on successful refresh completion in backend/internal/scheduler/scheduler.go
- [x] T012 [US1] Use local clone path for stack file reads in backend/internal/refresh/refresh.go

**Checkpoint**: Startup refresh produces a local clone at /repo and reconcile/update are gated on refresh success

---

## Phase 4: User Story 2 - Manual and Webhook Refresh (Priority: P2)

**Goal**: Refresh the local clone on manual and webhook triggers, and notify UI when refresh fails or updates are blocked.

**Independent Test**: Trigger manual refresh and webhook refresh and confirm refresh status updates and tasks are blocked on failure.

### Tests for User Story 2 (Required)

- [x] T013 [P] [US2] Integration test for manual refresh updating local clone in backend/tests/integration/dind_reconcile_test.go
- [x] T014 [P] [US2] Integration test for refresh status blocked notification in backend/tests/integration/dind_reconcile_test.go

### Implementation for User Story 2

- [x] T015 [US2] Update refresh trigger handling to cancel running reconcile/update tasks in backend/internal/refresh/refresh.go
- [x] T016 [US2] Emit updatesBlocked and blockedReason on refresh failure in backend/internal/desiredstate/state.go
- [x] T017 [US2] Broadcast refresh.status SSE updates on refresh transitions in backend/internal/desiredstate/broadcaster.go
- [x] T018 [US2] Ensure webhook refresh uses LocalClone and respects gating in backend/internal/http/handler.go

**Checkpoint**: Manual/webhook refreshes update the local clone and UI receives blocked status on failure

---

## Phase 5: User Story 3 - Local Divergence Recovery (Priority: P3)

**Goal**: Replace diverged or corrupted local clones with remote contents using fetch/reset with re-clone fallback.

**Independent Test**: Corrupt the local clone and verify refresh replaces it with a clean clone from remote.

### Tests for User Story 3 (Required)

- [x] T019 [P] [US3] Unit test for re-clone fallback when LocalClone refresh fails in backend/internal/git/local_clone_test.go
- [x] T020 [P] [US3] Integration test for divergence recovery via refresh in backend/tests/integration/dind_reconcile_test.go

### Implementation for User Story 3

- [x] T021 [US3] Implement fetch + force reset with re-clone fallback in backend/internal/git/local_clone.go
- [x] T022 [US3] Record refresh failure details for blocked UI status in backend/internal/refresh/refresh.go

**Checkpoint**: Local divergence recovery works and blocked status is accurate

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation and comprehensive integration coverage

- [x] T023 [P] Expand docker-in-docker update cycle coverage in backend/tests/integration/dind_update_test.go
- [x] T024 [P] Update README.md with /repo volume requirement and blocked refresh behavior in README.md
- [x] T025 [P] Validate quickstart instructions with new volume requirement in specs/008-local-git-clone/quickstart.md

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies
- **Foundational (Phase 2)**: Depends on Setup completion
- **User Stories (Phase 3-5)**: Depend on Foundational completion
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: No dependencies after Foundational
- **User Story 2 (P2)**: Depends on US1 LocalClone usage
- **User Story 3 (P3)**: Depends on US1 LocalClone usage

### Parallel Opportunities

- Tests within each story marked [P] can be done in parallel
- Integration tests in US2/US3 can be written in parallel once LocalClone scaffolding exists
- Documentation updates in Phase 6 can be done in parallel

---

## Parallel Example: User Story 1

- T006 [US1] Unit tests for LocalClone open/clone and fetch/reset in backend/internal/git/local_clone_test.go
- T007 [US1] Integration test for startup refresh + reconcile gating in backend/tests/integration/dind_reconcile_test.go

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Setup (Phase 1)
2. Complete Foundational (Phase 2)
3. Complete User Story 1 (Phase 3)
4. Validate with unit + integration tests for US1

### Incremental Delivery

1. Add User Story 2 with refresh trigger coverage
2. Add User Story 3 divergence recovery
3. Finish polish tasks and documentation
