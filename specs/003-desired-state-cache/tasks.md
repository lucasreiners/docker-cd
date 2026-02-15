---

description: "Task list for desired state cache and refresh"
---

# Tasks: Desired State Cache and Refresh

**Input**: Design documents from `/specs/003-desired-state-cache/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests are REQUIRED for core user story flows per the constitution.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure

- [X] T001 Add refresh config fields (WEBHOOK_SECRET, REFRESH_POLL_INTERVAL) in internal/config/config.go
- [X] T002 [P] Add config parsing tests for new env vars in internal/config/config_test.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

- [X] T003 Create desired state models and in-memory store in internal/desiredstate/state.go
- [X] T004 [P] Add compose hash helper in internal/desiredstate/hash.go
- [X] T005 [P] Implement Git compose reader (list stacks, read compose files) in internal/git/reader.go
- [X] T006 Implement refresh queue + single-slot replacement in internal/refresh/queue.go
- [X] T007 [P] Implement refresh service to update cache in internal/refresh/refresh.go
- [X] T008 [P] Preserve per-stack sync status during refresh updates in internal/refresh/refresh.go
- [X] T009 [P] Add desired state store unit tests in internal/desiredstate/state_test.go
- [X] T010 [P] Add refresh queue unit tests in internal/refresh/queue_test.go
- [X] T011 [P] Add Git compose reader tests with stubs in internal/git/reader_test.go

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Build Desired State on Startup (Priority: P1) 🎯 MVP

**Goal**: Refresh desired state on startup and via periodic polling.

**Independent Test**: Start service with a known repo and verify the cache is populated; validate periodic refresh runs on schedule.

### Tests for User Story 1 (REQUIRED) ⚠️

- [X] T012 [P] [US1] Add refresh service tests for startup + periodic refresh in internal/refresh/refresh_test.go

### Implementation for User Story 1

- [X] T013 [US1] Wire startup refresh and poll interval scheduler in cmd/docker-cd/main.go

**Checkpoint**: User Story 1 is fully functional and testable independently

---

## Phase 4: User Story 2 - Refresh Desired State via Webhook (Priority: P2)

**Goal**: Trigger refresh via webhook with GitHub HMAC signature validation.

**Independent Test**: Call webhook endpoint with valid signature and verify refresh runs; invalid signature returns 401.

### Tests for User Story 2 (REQUIRED) ⚠️

- [X] T014 [P] [US2] Add webhook handler tests for signature validation + queue responses in internal/http/handler_test.go

### Implementation for User Story 2

- [X] T015 [US2] Implement webhook handler with HMAC validation in internal/http/handler.go
- [X] T016 [US2] Register webhook route in internal/http/router.go

**Checkpoint**: User Stories 1 and 2 are functional and independently testable

---

## Phase 5: User Story 3 - Refresh on Demand and Track Stack Sync Status (Priority: P3)

**Goal**: Provide manual refresh, refresh-status, and stacks endpoints with JSON responses.

**Independent Test**: Call manual refresh, refresh-status, and stacks endpoints and verify JSON responses with expected fields.

### Tests for User Story 3 (REQUIRED) ⚠️

- [X] T017 [P] [US3] Add handler tests for manual refresh, refresh-status, and stacks endpoints in internal/http/handler_test.go

### Implementation for User Story 3

- [X] T018 [US3] Implement manual refresh handler in internal/http/handler.go
- [X] T019 [US3] Implement refresh-status handler in internal/http/handler.go
- [X] T020 [US3] Implement stacks handler in internal/http/handler.go
- [X] T021 [US3] Register refresh-status and stacks routes in internal/http/router.go

**Checkpoint**: All user stories are functional and independently testable

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [X] T022 [P] Update docker/docker-compose.yml with WEBHOOK_SECRET and REFRESH_POLL_INTERVAL env vars
- [X] T023 [P] Update README.md with new endpoints, webhook secret, and refresh polling
- [X] T024 [P] Add explicit log levels (info/warn/error) for refresh and webhook paths
- [X] T025 [P] Add compose apply regression test coverage in tests/integration/smoke_test.go

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
- Core services before handlers
- Handlers before router registration

---

## Parallel Execution Examples

### User Story 1

- T012 [US1] Refresh service tests in internal/refresh/refresh_test.go
- T013 [US1] Startup refresh + scheduler wiring in cmd/docker-cd/main.go

### User Story 2

- T014 [US2] Webhook handler tests in internal/http/handler_test.go
- T015 [US2] Webhook handler implementation in internal/http/handler.go
- T016 [US2] Route registration in internal/http/router.go

### User Story 3

- T017 [US3] Handler tests for refresh-status/stacks in internal/http/handler_test.go
- T018 [US3] Manual refresh handler in internal/http/handler.go
- T019 [US3] Refresh-status handler in internal/http/handler.go
- T020 [US3] Stacks handler in internal/http/handler.go
- T021 [US3] Route registration in internal/http/router.go

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 1
4. Validate startup refresh + periodic polling

### Incremental Delivery

1. Deliver User Story 1 (startup + periodic refresh)
2. Add User Story 2 (webhook refresh)
3. Add User Story 3 (manual refresh + status endpoints)
4. Finish polish tasks

### Parallel Team Strategy

- After Foundational phase, different developers can implement US1, US2, and US3 in parallel.
- Tests and implementations can run concurrently across different files.
