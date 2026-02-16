---

description: "Task list for stack sync and reconciliation"
---

# Tasks: Stack Sync and Reconciliation

**Input**: Design documents from `/specs/004-stack-sync/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests are REQUIRED for core user story flows per the constitution.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and configuration scaffolding

- [X] T001 Add reconcile config fields (including drift policy) in internal/config/config.go
- [X] T002 [P] Add reconcile config parsing tests in internal/config/config_test.go
- [X] T003 [P] Update docker/docker-compose.yml with RECONCILE_ENABLED, RECONCILE_REMOVE_ENABLED, and drift policy env var
- [X] T004 [P] Update README.md with reconcile config (including drift policy) and stacks metadata fields

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

- [X] T005 Create label constants for sync metadata in internal/reconcile/labels.go
- [X] T006 Extend StackRecord and StackSyncStatus (add failed + metadata fields, including commit message and lastSyncAt) in internal/desiredstate/state.go
- [X] T007 [P] Update desiredstate store copy helpers and tests in internal/desiredstate/state_test.go
- [X] T008 Add docker client helpers to list containers/labels in internal/docker/client.go
- [X] T009 [P] Add docker label parsing tests in internal/docker/client_test.go
- [X] T010 Add reconciliation policy struct/defaults in internal/reconcile/policy.go

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Reconcile Desired vs Actual State (Priority: P1) 🎯 MVP

**Goal**: Detect drift and apply docker compose updates so stacks converge to desired state.

**Independent Test**: Given a drifted stack, reconcile updates it and marks status `synced` without modifying unchanged stacks.

### Tests for User Story 1 (REQUIRED) ⚠️

- [X] T011 [P] [US1] Add drift detection and plan tests in internal/reconcile/reconcile_test.go
- [X] T012 [P] [US1] Add integration test for compose apply in tests/integration/reconcile_test.go
- [X] T030 [P] [US1] Add drift policy and deploy scope tests in internal/reconcile/reconcile_test.go
- [X] T031 [P] [US1] Add concurrency and cache preservation tests in internal/reconcile/reconcile_test.go
- [X] T036 [P] [US1] Add acknowledgement endpoint tests in internal/http/handler_test.go

### Implementation for User Story 1

- [X] T013 [US1] Implement drift detection and reconcile planner in internal/reconcile/reconcile.go
- [X] T014 [US1] Implement compose apply runner with label override file in internal/reconcile/compose.go
- [X] T015 [US1] Update stack status transitions (syncing/synced/failed) in internal/reconcile/reconcile.go
- [X] T032 [US1] Implement drift policy handling and operator acknowledgement gating in internal/reconcile/reconcile.go
- [X] T037 [US1] Add acknowledgement store and helpers in internal/reconcile/ack.go
- [X] T033 [US1] Enforce deploy scope filtering in internal/reconcile/reconcile.go
- [X] T034 [US1] Enforce single-stack concurrency in internal/reconcile/reconcile.go
- [X] T035 [US1] Preserve desired-state cache on reconcile failure in internal/reconcile/reconcile.go
- [X] T016 [US1] Trigger reconciliation after refresh success in internal/refresh/refresh.go
- [X] T017 [US1] Wire reconciler in cmd/docker-cd/main.go
- [X] T038 [US1] Add acknowledgement handler and route in internal/http/handler.go and internal/http/router.go

**Checkpoint**: User Story 1 is fully functional and testable independently

---

## Phase 4: User Story 2 - Track Synced Metadata (Priority: P2)

**Goal**: Record per-stack synced revision/hash/commit message and expose metadata via `/api/stacks`.

**Independent Test**: After a reconcile, `/api/stacks` returns synced revision, compose hash, and commit message derived from container labels.

### Tests for User Story 2 (REQUIRED) ⚠️

- [X] T018 [P] [US2] Add label-to-metadata mapping tests in internal/reconcile/metadata_test.go
- [X] T019 [P] [US2] Add stacks handler metadata test in internal/http/handler_test.go

### Implementation for User Story 2

- [X] T020 [US2] Add commit message label and metadata mapping in internal/reconcile/labels.go and internal/reconcile/metadata.go
- [X] T021 [US2] Expose synced metadata fields in stacks handler in internal/http/handler.go

**Checkpoint**: User Stories 1 and 2 are functional and independently testable

---

## Phase 5: User Story 3 - Handle Stack Removal (Priority: P3)

**Goal**: Detect stacks removed from desired state and remove them when opt-in is enabled.

**Independent Test**: When a stack is removed from desired state and removal is enabled, reconcile removes it and marks status `missing`.

### Tests for User Story 3 (REQUIRED) ⚠️

- [X] T022 [P] [US3] Add removal decision tests in internal/reconcile/reconcile_test.go
- [X] T023 [P] [US3] Add integration test for removal path in tests/integration/reconcile_test.go

### Implementation for User Story 3

- [X] T024 [US3] Implement removal flow using `docker compose down --remove-orphans` in internal/reconcile/compose.go
- [X] T025 [US3] Respect RECONCILE_REMOVE_ENABLED in internal/reconcile/reconcile.go

**Checkpoint**: All user stories are functional and independently testable

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [X] T026 [P] Add no-op reconcile regression test in internal/reconcile/reconcile_test.go
- [X] T027 [P] Update tests/integration/smoke_test.go to validate stacks metadata response
- [X] T028 [P] Add reconcile logging for start/success/failure in internal/reconcile/reconcile.go
- [X] T029 [P] Validate quickstart steps and update specs/004-stack-sync/quickstart.md

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3+)**: All depend on Foundational phase completion
- **Polish (Phase 6)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Starts after Foundational phase
- **User Story 2 (P2)**: Starts after Foundational phase
- **User Story 3 (P3)**: Starts after Foundational phase

### Within Each User Story

- Tests must be written and failing before implementation
- Drift/decision logic before compose apply
- Handlers after metadata mapping
- Story complete before moving to next priority

---

## Parallel Execution Examples

### User Story 1

- T011 [US1] Drift detection tests in internal/reconcile/reconcile_test.go
- T012 [US1] Compose apply integration test in tests/integration/reconcile_test.go

### User Story 2

- T018 [US2] Metadata mapping tests in internal/reconcile/metadata_test.go
- T019 [US2] Stacks handler tests in internal/http/handler_test.go

### User Story 3

- T022 [US3] Removal decision tests in internal/reconcile/reconcile_test.go
- T023 [US3] Removal integration test in tests/integration/reconcile_test.go

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 1
4. Validate User Story 1 independently

### Incremental Delivery

1. Deliver User Story 1 (reconcile apply)
2. Add User Story 2 (sync metadata)
3. Add User Story 3 (stack removal)
4. Finish polish tasks

### Parallel Team Strategy

- After Foundational phase, different developers can implement US1, US2, and US3 in parallel.
- Tests and implementations can run concurrently across different files.
