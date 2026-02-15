# Tasks: Docker-CD Skeleton Runtime

**Input**: Design documents from `/specs/001-skeleton-runtime/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests are REQUIRED for core user story flows per the constitution.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Include exact file paths in descriptions

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure

- [x] T001 Initialize Go module and install Gin dependency in go.mod
- [x] T002 [P] Create project directory structure per plan (cmd/, internal/, tests/, docker/)
- [x] T003 [P] Create .gitignore for Go project

**Checkpoint**: Project compiles with empty main

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story

- [x] T004 Implement RuntimeConfig in internal/config/config.go (env-based config with defaults)
- [x] T005 Implement DockerStatus model in internal/docker/status.go
- [x] T006 [P] Implement CommandRunner interface and real implementation in internal/docker/runner.go
- [x] T007 Implement Docker container count function in internal/docker/client.go (uses CommandRunner)
- [x] T008 Implement ASCII art renderer in internal/render/render.go

**Checkpoint**: Foundation ready — user story implementation can now begin

---

## Phase 3: User Story 1 — See Runtime Status (Priority: P1) 🎯 MVP

**Goal**: Root endpoint returns ASCII art with project name and running container count

**Independent Test**: `curl http://localhost:8080/` returns ASCII art and count

### Tests for User Story 1 (REQUIRED for core flow) ⚠️

- [x] T009 [P] [US1] Unit test for config loading in internal/config/config_test.go
- [x] T010 [P] [US1] Unit test for Docker container count with stubbed runner in internal/docker/client_test.go
- [x] T011 [P] [US1] Unit test for ASCII art rendering in internal/render/render_test.go
- [x] T012 [US1] Unit test for root HTTP handler with Gin test context in internal/http/handler_test.go

### Implementation for User Story 1

- [x] T013 [US1] Implement root HTTP handler in internal/http/handler.go (uses config, docker, render)
- [x] T014 [US1] Implement Gin router setup in internal/http/router.go
- [x] T015 [US1] Implement main.go entry point in cmd/docker-cd/main.go
- [x] T016 [US1] Verify all unit tests pass

**Checkpoint**: User Story 1 is fully functional and testable independently

---

## Phase 4: User Story 2 — Local Compose Test Harness (Priority: P2)

**Goal**: Dockerfile + docker-compose.yml enabling local testing with mounted Docker socket

**Independent Test**: `docker compose -f docker/docker-compose.yml up --build` starts the service

### Tests for User Story 2 (REQUIRED for core flow) ⚠️

- [x] T017 [US2] Integration smoke test in tests/integration/smoke_test.go (build-tag guarded, requires Docker)

### Implementation for User Story 2

- [x] T018 [US2] Create multi-stage Dockerfile in docker/Dockerfile (build Go binary, install Docker CLI + Compose)
- [x] T019 [US2] Create docker-compose.yml in docker/docker-compose.yml (mount socket, expose port)
- [x] T020 [US2] Verify container builds and root endpoint works via compose

**Checkpoint**: Both user stories work independently

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Final cleanup

- [x] T021 [P] Create README.md with quickstart instructions
- [x] T022 [P] Verify .gitignore covers build artifacts
- [x] T023 Run full test suite and confirm pass

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational phase completion
- **User Story 2 (Phase 4)**: Depends on User Story 1 implementation (needs working binary)
- **Polish (Phase 5)**: Depends on all user stories being complete

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Models / interfaces before services
- Services before handlers
- Handlers before main entry point

### Parallel Opportunities

- T002 and T003 can run in parallel (different files)
- T006 can run in parallel with T004/T005 (different packages)
- T009, T010, T011 can all run in parallel (different test files)
