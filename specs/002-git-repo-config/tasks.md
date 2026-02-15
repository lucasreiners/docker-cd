---

description: "Task list for git repository configuration feature"
---

# Tasks: Git Repository Configuration

**Input**: Design documents from `/specs/002-git-repo-config/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests are REQUIRED for core user story flows per the constitution.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure

- [x] T001 Add go-git dependency to go.mod and go.sum
- [x] T002 [P] Add git env var placeholders to docker/docker-compose.yml

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

- [x] T003 Define repository config fields and env var constants in internal/config/config.go
- [x] T004 [P] Add config parsing/default tests in internal/config/config_test.go

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Configure repository access (Priority: P1) 🎯 MVP

**Goal**: Validate repository access and revision on startup, failing fast for missing or invalid configuration.

**Independent Test**: Run unit tests for validation and start the service with missing or invalid env values to confirm startup failure.

### Tests for User Story 1 (REQUIRED) ⚠️

- [x] T005 [P] [US1] Add git validator failure tests (missing ref, auth failure) in internal/git/validator_test.go
- [x] T006 [P] [US1] Add startup validation tests in cmd/docker-cd/main_test.go

### Implementation for User Story 1

- [x] T007 [US1] Create internal/git/validator.go with go-git read-only validation and HTTPS URL checks
- [x] T008 [US1] Refactor cmd/docker-cd/main.go to run startup validation and fail fast on error
- [x] T009 [US1] Add validation error types/messages in internal/git/errors.go

**Checkpoint**: User Story 1 is fully functional and testable independently

---

## Phase 4: User Story 2 - Confirm configuration in the UI (Priority: P2)

**Goal**: Display repository URL, revision, and deployment directory on the root page without exposing the token.

**Independent Test**: Start the service with valid config and verify the root page shows non-secret repo info only.

### Tests for User Story 2 (REQUIRED) ⚠️

- [x] T010 [P] [US2] Add renderer tests for repo info output in internal/render/render_test.go
- [x] T011 [P] [US2] Add handler tests for root page repo info in internal/http/handler_test.go

### Implementation for User Story 2

- [x] T012 [US2] Update internal/render/render.go to render repo URL, revision, and deploy dir
- [x] T013 [US2] Update internal/http/handler.go to pass repo config to the renderer (exclude token)

**Checkpoint**: User Stories 1 and 2 are functional and independently testable

---

## Phase 5: User Story 3 - Target a deployment subdirectory (Priority: P3)

**Goal**: Allow an optional deployment subdirectory and validate it exists at the configured revision.

**Independent Test**: Run validator tests with and without a deploy dir to confirm defaulting and path checks.

### Tests for User Story 3 (REQUIRED) ⚠️

- [x] T014 [P] [US3] Add deploy dir validation tests in internal/git/validator_test.go

### Implementation for User Story 3

- [x] T015 [US3] Extend internal/git/validator.go to verify deploy dir exists at revision

**Checkpoint**: All user stories are functional and independently testable

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [x] T016 [P] Update README.md with new git env vars and startup validation behavior

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3+)**: All depend on Foundational phase completion
- **Polish (Final Phase)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2)
- **User Story 2 (P2)**: Depends on User Story 1 for validated repo config
- **User Story 3 (P3)**: Depends on User Story 1 for validated repo config

### Parallel Opportunities

- Phase 1: T002 can run in parallel with T001
- Phase 2: T004 can run in parallel with T003
- Phase 3: T005 and T006 can run in parallel
- Phase 4: T010 and T011 can run in parallel

---

## Parallel Example: User Story 1

```bash
Task: "Add git validator failure tests in internal/git/validator_test.go"
Task: "Add startup validation tests in cmd/docker-cd/main_test.go"
```

---

## Parallel Example: User Story 2

```bash
Task: "Add renderer tests for repo info output in internal/render/render_test.go"
Task: "Add handler tests for root page repo info in internal/http/handler_test.go"
```

---

## Parallel Example: User Story 3

```bash
Task: "Add deploy dir validation tests in internal/git/validator_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (critical)
3. Complete Phase 3: User Story 1
4. Validate with unit tests and a startup failure check

### Incremental Delivery

1. Setup + Foundational
2. User Story 1 (MVP)
3. User Story 2
4. User Story 3
5. Polish

### Parallel Team Strategy

- Developer A: US1
- Developer B: US2
- Developer C: US3
