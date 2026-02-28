---
description: "Implementation tasks for Scheduled Stack Updates feature"
---

# Tasks: Scheduled Stack Updates

**Input**: Design documents from `/specs/006-scheduled-stack-updates/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: No explicit test requirements found in specification. Implementation includes unit and integration tests following existing patterns.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

This project uses `backend/` for Go code with structure:
- `backend/cmd/docker-cd/` - Main application
- `backend/internal/` - Internal packages
- `backend/tests/integration/` - Integration tests

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and dependencies

- [X] T001 Add github.com/robfig/cron/v3 dependency to backend/go.mod
- [X] T002 [P] Create internal/scheduler package directory structure
- [X] T003 [P] Create tests/integration/scheduler_test.go placeholder

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T004 Add UpdaterConfig struct to internal/config/config.go with Enabled and CronExpression fields
- [X] T005 [P] Create SchedulerService struct skeleton in internal/scheduler/scheduler.go
- [X] T006 [P] Add image operations interface to internal/docker/images.go (Pull, Prune, GetDigest methods)
- [X] T007 Wire up SchedulerService initialization in backend/cmd/docker-cd/main.go with lifecycle management
- [X] T008 Implement Start() and Stop() lifecycle methods in internal/scheduler/scheduler.go for graceful shutdown

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Automatic Nightly Stack Updates (Priority: P1) 🎯 MVP

**Goal**: Compose stacks are automatically updated every night at 3 AM UTC with image pulls, container recreation when images change, and cleanup of unused images

**Independent Test**: Deploy system with UPDATER_ENABLED=true, wait for 3 AM UTC (or trigger manually), verify stacks with outdated images are updated and logs show image pulls and container recreation

### Implementation for User Story 1

- [X] T009 [P] [US1] Create UpdateCycle struct in internal/scheduler/updater.go with cycle tracking fields (CycleID, timestamps, counters)
- [X] T010 [P] [US1] Create StackUpdateResult struct in internal/scheduler/updater.go for per-stack outcomes
- [X] T011 [P] [US1] Create ImagePullResult struct in internal/scheduler/updater.go for per-image tracking
- [X] T012 [US1] Implement default cron schedule initialization (0 3 * * *) in internal/scheduler/scheduler.go using robfig/cron
- [X] T013 [US1] Implement RunUpdateCycle() method in internal/scheduler/updater.go with cycle start/end logging
- [X] T014 [US1] Implement image pull logic using "docker compose pull" in internal/scheduler/updater.go per-stack sequential processing
- [X] T015 [US1] Implement change detection by comparing image digests in internal/scheduler/updater.go before/after pull
- [X] T016 [US1] Implement container recreation trigger via existing reconcile package when images changed
- [X] T017 [US1] Implement system-wide image pruning using "docker image prune -af" in internal/scheduler/updater.go
- [X] T018 [US1] Add basic structured logging for cycle start, completion, and error handling using slog
- [X] T019 [US1] Implement error recovery continuing with remaining stacks when one stack fails (FR-012)
- [X] T020 [US1] Add integration test in tests/integration/scheduler_test.go validating full update cycle with testcontainers

**Checkpoint**: At this point, User Story 1 should be fully functional - automatic updates run at 3 AM UTC with default configuration

---

## Phase 4: User Story 2 - Configure Update Schedule (Priority: P2)

**Goal**: System administrators can customize the update schedule using cron expressions instead of being limited to the default 3 AM UTC schedule

**Independent Test**: Set UPDATER_CRON to custom expression (e.g., "0 */6 * * *"), verify system accepts it and runs updates at configured times instead of default

### Implementation for User Story 2

- [X] T021 [P] [US2] Implement environment variable loading for UPDATER_ENABLED in internal/config/config.go using strconv.ParseBool
- [X] T022 [P] [US2] Implement environment variable loading for UPDATER_CRON in internal/config/config.go with default "0 3 * * *"
- [X] T023 [US2] Implement cron expression validation in internal/scheduler/scheduler.go using robfig/cron parser
- [X] T024 [US2] Add fallback to default schedule when invalid cron expression provided with error logging
- [X] T025 [US2] Implement scheduler enable/disable logic checking UPDATER_ENABLED flag in Start() method
- [X] T026 [US2] Add startup logging showing loaded configuration (enabled status and schedule) in internal/scheduler/scheduler.go
- [X] T027 [US2] Add unit tests in internal/scheduler/scheduler_test.go for cron parsing, validation, and fallback behavior
- [X] T028 [US2] Update integration test to verify custom cron schedules are respected in tests/integration/scheduler_test.go

**Checkpoint**: At this point, User Stories 1 AND 2 should both work independently - users can configure custom schedules

---

## Phase 5: User Story 3 - Update Operation Visibility (Priority: P3)

**Goal**: Detailed logs of all update operations enable troubleshooting, verification, and understanding of what changed during each cycle

**Independent Test**: Run update cycle and verify logs contain detailed information about each stack, image pull, success/failure status, and summary statistics

### Implementation for User Story 3

- [X] T029 [P] [US3] Create UpdateOperationLogEntry struct in internal/scheduler/updater.go with operation type, timestamp, subject, status fields
- [X] T030 [P] [US3] Implement structured logging for cycle start event with cycle ID and timestamp
- [X] T031 [P] [US3] Implement per-image pull logging with image name and result status
- [X] T032 [P] [US3] Implement per-stack update logging with stack name, containers affected, and change detection
- [X] T033 [P] [US3] Implement prune operation logging with images removed count and space reclaimed
- [X] T034 [US3] Implement cycle completion summary logging with all statistics (stacks processed, images pulled, containers updated, images pruned)
- [X] T035 [US3] Add error detail logging with sufficient context for diagnosis (stack name, operation, error message)
- [X] T036 [US3] Implement JSON log format support for structured log ingestion per contracts/log-format.md
- [X] T037 [US3] Add network failure handling with appropriate error logging (FR-013)
- [X] T038 [US3] Add stopped/error stack detection and skip logging (FR-014)
- [X] T039 [US3] Update integration tests to validate log output format and completeness in tests/integration/scheduler_test.go

**Checkpoint**: All user stories should now be independently functional with comprehensive logging

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [X] T040 [P] Implement overlapping cycle termination logic when new scheduled time arrives (FR-020)
- [X] T041 [P] Add restart recovery behavior abandoning in-progress cycles (FR-018)
- [X] T042 [P] Document UPDATER_ENABLED and UPDATER_CRON in README.md with examples
- [X] T043 Validate quickstart.md scenarios work end-to-end
- [X] T044 [P] Add unit tests for UpdateCycle, StackUpdateResult, and ImagePullResult structs in internal/scheduler/updater_test.go
- [X] T045 Code review and refactoring for clarity and maintainability
- [X] T046 Performance validation ensuring update cycles complete within 30 minutes (SC-001)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3-5)**: All depend on Foundational phase completion
  - User stories can then proceed in parallel (if staffed)
  - Or sequentially in priority order (P1 → P2 → P3)
- **Polish (Phase 6)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) - No dependencies on other stories
- **User Story 2 (P2)**: Can start after Foundational (Phase 2) - Extends US1 but independently testable (can configure scheduling)
- **User Story 3 (P3)**: Can start after Foundational (Phase 2) - Enhances US1 logging but independently testable (detailed logs vs basic logs)

### Within Each User Story

#### User Story 1 Dependencies:
- T009-T011 (structs) can run in parallel - must complete before T013-T020
- T012 (cron init) independent - can run anytime after T005
- T013 (cycle orchestration) depends on T009-T011
- T014-T017 (update operations) depend on T013
- T018-T019 (logging/error handling) depend on T013-T017
- T020 (integration test) depends on all implementation tasks

#### User Story 2 Dependencies:
- T021-T022 (env loading) can run in parallel
- T023-T024 (validation) depend on T022
- T025 (enable/disable) depends on T021
- T026 (startup logging) depends on T021-T024
- T027-T028 (tests) depend on all implementation tasks

#### User Story 3 Dependencies:
- T029 (log struct) independent
- T030-T035 (logging implementations) can run in parallel after T029
- T036 (JSON format) depends on T030-T035
- T037-T038 (error logging) can run in parallel
- T039 (test validation) depends on all implementation tasks

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel (T002, T003)
- Within Foundational phase: T005, T006 can run in parallel (different files)
- Within User Story 1: T009, T010, T011 can run in parallel (struct definitions)
- Within User Story 2: T021, T022 can run in parallel (different config fields)
- Within User Story 3: T030, T031, T032, T033 can run in parallel (different log events)
- Once Foundational phase completes, all three user stories can be worked on in parallel by different team members
- All Polish tasks marked [P] can run in parallel (T040, T041, T042, T044)

---

## Parallel Example: User Story 1

```bash
# Launch struct definitions in parallel:
Task T009: "Create UpdateCycle struct in internal/scheduler/updater.go"
Task T010: "Create StackUpdateResult struct in internal/scheduler/updater.go"
Task T011: "Create ImagePullResult struct in internal/scheduler/updater.go"

# After structs complete, these can proceed sequentially:
Task T013: "Implement RunUpdateCycle() method"
Task T014: "Implement image pull logic using docker compose pull"
Task T015: "Implement change detection by comparing image digests"
# ... and so on
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: Test User Story 1 independently with default 3 AM schedule
5. Deploy/demo if ready

At this point you have: Automatic nightly updates at 3 AM UTC with basic logging - a complete, valuable feature

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 → Test independently → Deploy/Demo (MVP! Automatic updates work)
3. Add User Story 2 → Test independently → Deploy/Demo (Now configurable schedule)
4. Add User Story 3 → Test independently → Deploy/Demo (Now detailed logging)
5. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 1 (Core update cycle)
   - Developer B: User Story 2 (Configuration)
   - Developer C: User Story 3 (Logging)
3. Stories complete and integrate independently
4. Final integration testing validates all stories work together

---

## Notes

- **[P] tasks**: Different files or independent changes, no dependencies on incomplete tasks
- **[Story] label**: Maps task to specific user story for traceability and independent testing
- Each user story should be independently completable and testable
- Tests follow existing project patterns (unit tests with _test.go, integration tests with testcontainers)
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- **Avoid**: Vague tasks, same file conflicts, cross-story dependencies that break independence

---

## Task Summary

- **Total Tasks**: 46
- **Setup Phase**: 3 tasks
- **Foundational Phase**: 5 tasks (BLOCKING)
- **User Story 1 (P1 - MVP)**: 12 tasks
- **User Story 2 (P2)**: 8 tasks
- **User Story 3 (P3)**: 11 tasks
- **Polish Phase**: 7 tasks
- **Parallel Opportunities**: 18 tasks marked [P]
- **Independent Test Criteria**: Defined for each user story
- **Suggested MVP Scope**: Phases 1-3 only (20 tasks) delivers automatic nightly updates
