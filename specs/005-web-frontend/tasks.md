# Tasks: Minimal Web Frontend

**Input**: Design documents from `/specs/005-web-frontend/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests are REQUIRED for each user story per the constitution.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure

- [X] T001 Create Vue 3 + Vite frontend scaffold in frontend/ (package.json, vite.config.ts, index.html, src/main.ts, src/App.vue)
- [X] T002 [P] Add UI/theme and core deps in frontend/package.json (Naive UI, Pinia, Vue Router) and create frontend/src/styles/theme.css
- [X] T003 [P] Add Dockerfile for frontend build/serve in docker/frontend.Dockerfile
- [X] T004 Update compose stack to include frontend service and DOCKER_CD_API_BASE_URL in docker/docker-compose.yml
- [X] T005 [P] Add frontend scripts for dev/build/test in frontend/package.json

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

- [X] T006 Add SSE event definitions and broadcaster in internal/desiredstate/events.go
- [X] T007 Wire desired-state refresh updates to SSE broadcaster in internal/refresh/refresh.go
- [X] T008 Add SSE endpoint handler for /api/events in internal/http/handler.go
- [X] T009 Register /api/events route in internal/http/router.go

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Monitor stacks at a glance (Priority: P1) 🎯 MVP

**Goal**: Grid view of stacks with live status updates via SSE and client-side filtering/search.

**Independent Test**: Load the UI with sample data and SSE events to confirm grid rendering and live updates without polling.

### Tests for User Story 1 (REQUIRED) ⚠️

- [X] T010 [P] [US1] Contract test for /api/stacks response in internal/http/handler_test.go
- [X] T011 [P] [US1] Contract test for /api/events SSE stream in internal/http/events_test.go
- [X] T012 [P] [US1] Frontend store test for initial load + SSE upsert in frontend/tests/stacks.spec.ts
- [X] T013 [P] [US1] Frontend grid render test in frontend/tests/stacks-grid.spec.ts

### Implementation for User Story 1

- [X] T014 [P] [US1] Implement API client for /api/stacks and /api/refresh-status in frontend/src/services/api.ts
- [X] T015 [P] [US1] Implement SSE client wrapper (EventSource, retry, full-record upserts) in frontend/src/services/sse.ts
- [X] T016 [US1] Implement Pinia store with in-memory stack map, filtering, and search in frontend/src/store/stacks.ts
- [X] T017 [P] [US1] Build stacks grid page and card UI in frontend/src/pages/StacksGrid.vue and frontend/src/components/StackCard.vue
- [X] T018 [P] [US1] Add status badge component in frontend/src/components/StatusBadge.vue
- [X] T019 [US1] Wire router + app shell for grid as default route in frontend/src/main.ts and frontend/src/App.vue

**Checkpoint**: User Story 1 is functional and testable independently

---

## Phase 4: User Story 2 - Inspect stack details (Priority: P2)

**Goal**: Detail view for a selected stack showing sync metadata and errors.

**Independent Test**: Select a stack from the grid and verify details render with latest metadata.

### Tests for User Story 2 (REQUIRED) ⚠️

- [X] T020 [P] [US2] Frontend detail view test in frontend/tests/stack-detail.spec.ts

### Implementation for User Story 2

- [X] T021 [P] [US2] Implement stack detail page in frontend/src/pages/StackDetail.vue
- [X] T022 [US2] Add detail route and navigation from grid in frontend/src/router/index.ts and frontend/src/pages/StacksGrid.vue

**Checkpoint**: User Story 2 is independently functional and testable

---

## Phase 5: User Story 3 - Stay informed during connectivity issues (Priority: P3)

**Goal**: Clear UI indicator when SSE disconnects and when it recovers.

**Independent Test**: Simulate SSE disconnect and confirm stale indicator appears and clears on reconnect.

### Tests for User Story 3 (REQUIRED) ⚠️

- [X] T023 [P] [US3] Frontend connection state test in frontend/tests/connection-banner.spec.ts

### Implementation for User Story 3

- [X] T024 [P] [US3] Implement connection banner component in frontend/src/components/ConnectionBanner.vue
- [X] T025 [US3] Update store and grid page to show stale indicator on SSE disconnect in frontend/src/store/stacks.ts and frontend/src/pages/StacksGrid.vue

**Checkpoint**: User Story 3 is independently functional and testable

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation and final validation

- [X] T026 [P] Update quickstart and compose references if ports or env var usage changed in specs/005-web-frontend/quickstart.md
- [X] T027 [P] Run quickstart validation steps and note results in specs/005-web-frontend/quickstart.md

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies
- **Foundational (Phase 2)**: Depends on Setup completion
- **User Stories (Phase 3+)**: Depend on Foundational completion
- **Polish (Phase 6)**: Depends on all desired user stories

### User Story Dependencies

- **US1 (P1)**: No dependencies beyond Phase 2
- **US2 (P2)**: No dependencies beyond Phase 2
- **US3 (P3)**: No dependencies beyond Phase 2

### Within Each User Story

- Tests MUST be written and fail before implementation
- API/Store before UI wiring
- Core implementation before integration

---

## Parallel Execution Examples

### User Story 1

- T010, T011, T012, T013 can run in parallel (different files)
- T014, T015, T017, T018 can run in parallel after tests

### User Story 2

- T020 and T021 can run in parallel (tests + page scaffold)

### User Story 3

- T023 and T024 can run in parallel (tests + component scaffold)

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 1
4. Validate User Story 1 independently

### Incremental Delivery

1. Setup + Foundational
2. User Story 1 → validate
3. User Story 2 → validate
4. User Story 3 → validate
5. Polish & cross-cutting
