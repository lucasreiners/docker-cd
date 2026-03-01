# Tasks: Stack UI Enhancements

**Feature**: 007-stack-ui-enhancements  
**Input**: Design documents from `/specs/007-stack-ui-enhancements/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Frontend**: `frontend/src/`, `frontend/tests/`
- **Component paths**: `frontend/src/components/`, `frontend/src/pages/`
- **Utility paths**: `frontend/src/utils/`

---

## Phase 1: Foundational (Blocking Prerequisites)

**Purpose**: Port parsing utilities required by both US1 (grid) and US3 (details)

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T001 Create PortMapping interface in frontend/src/utils/portUtils.ts
- [X] T002 Implement parsePortString() function to parse "8080:80/tcp, 8443:443/tcp" format in frontend/src/utils/portUtils.ts
- [X] T003 Implement getLowestExternalPort() function to extract minimum port number in frontend/src/utils/portUtils.ts
- [X] T004 Implement buildPortURL() function to construct URLs matching current protocol in frontend/src/utils/portUtils.ts
- [X] T005 Create unit test suite for parsePortString() with single port, multiple ports, no external mapping, empty string, and malformed input in frontend/tests/port-utils.spec.ts
- [X] T006 Create unit test suite for getLowestExternalPort() with multiple ports, single port, no external ports, and empty string in frontend/tests/port-utils.spec.ts
- [X] T007 Create unit test suite for buildPortURL() with HTTP/HTTPS protocol matching and hostname validation in frontend/tests/port-utils.spec.ts
- [X] T008 Run unit tests and verify >90% coverage for portUtils.ts

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 2: User Story 1 - Quick Port Access from Grid View (Priority: P1) 🎯 MVP

**Goal**: Display containers in grid cards with clickable links to open lowest port

**Independent Test**: View stacks grid, see container names with arrow/underline for ports, click link, verify browser opens correct port

### Implementation for User Story 1

- [X] T009 [P] [US1] Add containers state (ref<ContainerInfo[]>) to StackCard component in frontend/src/components/StackCard.vue
- [X] T010 [P] [US1] Add containersLoading and containersError state to StackCard component in frontend/src/components/StackCard.vue
- [X] T011 [US1] Add onMounted hook to fetch containers via fetchContainers(stack.path) API in frontend/src/components/StackCard.vue
- [X] T012 [US1] Create sortedContainers computed property with alphabetical sorting by name in frontend/src/components/StackCard.vue
- [X] T013 [US1] Enhance sortedContainers to add parsedPorts, lowestExternalPort, and hasExternalPorts using portUtils in frontend/src/components/StackCard.vue
- [X] T014 [US1] Add handleContainerClick() method to open port URL in new tab/window in frontend/src/components/StackCard.vue
- [X] T015 [US1] Update StackCard template to display container list with v-for over sortedContainers in frontend/src/components/StackCard.vue
- [X] T016 [US1] Add conditional rendering for containers with ports: show ↗ prefix and underline styling in frontend/src/components/StackCard.vue
- [X] T017 [US1] Add conditional rendering for containers without ports: show plain text without link in frontend/src/components/StackCard.vue
- [X] T018 [US1] Add CSS styles for .containers-list, .container-link, and .underlined classes in frontend/src/components/StackCard.vue
- [X] T019 [US1] Add loading state display (spinner/skeleton) for containers fetch in frontend/src/components/StackCard.vue
- [X] T020 [US1] Add error state display for containers fetch failures in frontend/src/components/StackCard.vue
- [X] T021 [P] [US1] Update StacksGrid CSS to add grid-auto-rows: 1fr for uniform row height in frontend/src/pages/StacksGrid.vue
- [X] T022 [P] [US1] Update StackCard wrapper CSS to add height: 100% and flex layout for card growth in frontend/src/components/StackCard.vue
- [X] T023 [P] [US1] Create component test suite for StackCard container fetching on mount in frontend/tests/stack-card-ports.spec.ts
- [X] T024 [P] [US1] Create component test for alphabetical container sorting in frontend/tests/stack-card-ports.spec.ts
- [X] T025 [P] [US1] Create component test for arrow and underline styling on containers with ports in frontend/tests/stack-card-ports.spec.ts
- [X] T026 [P] [US1] Create component test for plain text display on containers without ports in frontend/tests/stack-card-ports.spec.ts
- [X] T027 [P] [US1] Create component test for clicking container link opening correct port URL in frontend/tests/stack-card-ports.spec.ts
- [X] T028 [US1] Run component tests and verify all StackCard tests pass
- [X] T029 [US1] Manual test: View grid with multiple stacks, verify containers display, click links, verify uniform height

**Checkpoint**: User Story 1 complete - grid view provides one-click port access

---

## Phase 3: User Story 2 - Browser Tab Identification (Priority: P2)

**Goal**: Show "<hostname> - Docker-CD" in browser tab title on all pages

**Independent Test**: Open Docker-CD in browser, check tab title displays hostname correctly, navigate pages, verify title persists

### Implementation for User Story 2

- [X] T030 [US2] Import useRouter from vue-router in frontend/src/App.vue
- [X] T031 [US2] Create updateDocumentTitle() function to set document.title using window.location.hostname in frontend/src/App.vue
- [X] T032 [US2] Add router.afterEach() hook to call updateDocumentTitle() on route changes in frontend/src/App.vue
- [X] T033 [US2] Add onMounted() hook to call updateDocumentTitle() on initial page load in frontend/src/App.vue
- [X] T034 [US2] Manual test: Open Docker-CD, verify tab title shows hostname, navigate between grid and details, verify title persists

**Checkpoint**: User Story 2 complete - browser tabs show hostname for multi-instance identification

---

## Phase 4: User Story 3 - Enhanced Port Display on Details Page (Priority: P3)

**Goal**: Fix duplicate port display and add clickable port pills on stack details page

**Independent Test**: Open stack details, verify ports appear once as clickable pills, click pills, verify browser opens ports

### Implementation for User Story 3

- [X] T035 [P] [US3] Import parsePortString and buildPortURL from portUtils in frontend/src/pages/StackDetail.vue
- [X] T036 [US3] Audit existing port display v-for loops to identify duplication source in frontend/src/pages/StackDetail.vue
- [X] T037 [US3] Update v-for :key to use composite key `${container.id}-${port.external}-${port.internal}` to prevent duplicates in frontend/src/pages/StackDetail.vue
- [X] T038 [US3] Create formatPortPill() helper method to format PortMapping as "8080:80/tcp" string in frontend/src/pages/StackDetail.vue
- [X] T039 [US3] Create openPort() helper method to call buildPortURL() and window.open() in frontend/src/pages/StackDetail.vue
- [X] T040 [US3] Replace existing port display with n-tag components for visual pill styling in frontend/src/pages/StackDetail.vue
- [X] T041 [US3] Add @click handler to each n-tag pill to call openPort() with external port number in frontend/src/pages/StackDetail.vue
- [X] T042 [US3] Add v-for loop over parsePortString(container.ports) to iterate port mappings in frontend/src/pages/StackDetail.vue
- [X] T043 [US3] Add CSS styles for .port-pill class with cursor pointer and hover effects in frontend/src/pages/StackDetail.vue
- [X] T044 [P] [US3] Create component test for no duplicate port display in frontend/tests/stack-detail-pills.spec.ts
- [X] T045 [P] [US3] Create component test for correct pill format "8080:80/tcp" in frontend/tests/stack-detail-pills.spec.ts
- [X] T046 [P] [US3] Create component test for clicking pill opening port URL in frontend/tests/stack-detail-pills.spec.ts
- [X] T047 [US3] Run component tests and verify all StackDetail tests pass
- [X] T048 [US3] Manual test: View stack details, verify ports display once as pills, click pills, verify URLs open correctly

**Checkpoint**: User Story 3 complete - details page has clean clickable port pills without duplicates

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Integration testing, edge case validation, documentation

- [X] T049 [P] Run full test suite with coverage reporting: bun run test
- [X] T050 [P] Verify test coverage >90% for all new code in portUtils.ts, StackCard.vue, StackDetail.vue, App.vue
- [X] T051 [P] Run linter and fix any issues: bun run lint:fix
- [X] T052 Run build to verify no TypeScript errors: bun run build
- [X] T053 Manual test: Protocol matching - access via HTTP and HTTPS, verify port links match protocol
- [X] T054 Manual test: Edge case - stopped containers still show enabled links
- [X] T055 Manual test: Edge case - containers with many (10+) ports display correctly without overflow
- [X] T056 Manual test: Edge case - stack with 20+ containers causes card to grow properly
- [X] T057 Manual test: Edge case - very long hostname in browser tab truncates gracefully
- [X] T058 Manual test: Accessibility - verify keyboard navigation works for container links and pills
- [X] T059 Manual test: Performance - verify <100ms interaction response for clicking links and pills
- [X] T060 Update feature documentation to mark implementation complete in specs/007-stack-ui-enhancements/plan.md
- [X] T061 Commit all changes with message "feat: complete stack UI enhancements (US1, US2, US3)"

**Checkpoint**: Feature complete - all user stories implemented, tested, and validated

---

## Dependencies

### User Story Completion Order

```
Foundation (Phase 1: Port Utilities)
    ↓
┌────────────────────────────────────┐
│   User Story 1 (Grid View)        │ ← MVP Scope
│   Tasks T009-T029                  │
└────────────────────────────────────┘
    ↓ (can run in parallel) ↑
┌─────────────────────┐    ┌────────────────────────────┐
│ User Story 2        │    │ User Story 3               │
│ (Browser Title)     │    │ (Details Pills)            │
│ Tasks T030-T034     │    │ Tasks T035-T048            │
└─────────────────────┘    └────────────────────────────┘
    ↓                           ↓
    └───────────────┬───────────┘
                    ↓
        Polish & Testing (Phase 5)
        Tasks T049-T061
```

**Key Dependencies**:
- **Foundation (T001-T008)**: MUST complete before any user story work
- **US1 and US2**: Can run in parallel after Foundation (independent)
- **US1 and US3**: T009-T022 (US1 implementation) and T035-T043 (US3 implementation) can run in parallel after Foundation
- **US1 and US3 tests**: T023-T028 (US1 tests) and T044-T047 (US3 tests) depend on Foundation but can be written during implementation
- **Polish**: Requires all user stories complete

### Parallel Execution Opportunities

**Phase 1 - Foundational**:
- After T004 (utilities implemented), can parallelize:
  - T005 (parsePortString tests)
  - T006 (getLowestExternalPort tests)  
  - T007 (buildPortURL tests)

**Phase 2 - User Story 1**:
- After T020 (StackCard implementation complete), can parallelize:
  - T021-T022 (CSS updates)
  - T023-T027 (test suite creation)

**Phase 3/4 - User Stories 2 & 3** (MAXIMUM PARALLELIZATION):
- After T008 (Foundation checkpoint), can run ALL of these in parallel:
  - US2: T030-T033 (Browser title implementation)
  - US3: T035-T043 (Details pills implementation)
  - US3 Tests: T044-T046 (Details pills test creation)

**Phase 5 - Polish**:
- After all user stories complete, can parallelize:
  - T049 (test suite)
  - T050 (coverage check)
  - T051 (linting)

---

## Implementation Strategy

### MVP First (Minimum Viable Product)

**Recommended MVP Scope**: User Story 1 ONLY (Tasks T001-T029)

**Rationale**: 
- US1 provides highest immediate value (one-click port access from grid)
- US1 is independent and fully testable on its own
- Reduces risk by delivering core functionality first

**MVP Delivery**:
1. Complete Phase 1 (Foundation) - T001-T008
2. Complete Phase 2 (US1) - T009-T029  
3. Test, validate, and potentially deploy to production
4. Gather user feedback before implementing US2/US3

### Incremental Delivery

After MVP, prioritize based on user feedback:

**Option A - Follow Priority Order**:
1. MVP: US1 (Grid view port access) ← SHIP THIS FIRST
2. Increment 2: US2 (Browser title) ← Quick win
3. Increment 3: US3 (Details pills) ← Polish

**Option B - Bundle UI Polish**:
1. MVP: US1 (Grid view port access) ← SHIP THIS FIRST
2. Increment 2: US2 + US3 together ← Comprehensive UI upgrade

---

## Task Summary

| Phase | Task Range | Count | Story | Description |
|-------|------------|-------|-------|-------------|
| Phase 1 | T001-T008 | 8 | Foundation | Port utilities + unit tests |
| Phase 2 | T009-T029 | 21 | US1 | Grid view containers + tests + CSS |
| Phase 3 | T030-T034 | 5 | US2 | Browser title |
| Phase 4 | T035-T048 | 14 | US3 | Details pills + tests |
| Phase 5 | T049-T061 | 13 | Polish | Integration testing + docs |
| **Total** | **T001-T061** | **61** | | **All tasks** |

**Parallel Opportunities**: 18 tasks marked [P] can run in parallel with others in their phase

**Estimated Time**:
- Foundation: 3 hours
- US1 (MVP): 7 hours  
- US2: 0.5 hours
- US3: 3 hours
- Polish: 2 hours
- **Total: ~15.5 hours**

---

## Validation

### Format Validation ✅

- [x] All tasks follow checklist format: `- [ ] [TaskID] [P?] [Story?] Description`
- [x] Sequential task IDs (T001-T061)
- [x] [P] markers on parallelizable tasks only
- [x] [Story] labels ([US1], [US2], [US3]) on user story phases only
- [x] No [Story] labels on Foundation or Polish phases
- [x] File paths included in all task descriptions

### Coverage Validation ✅

- [x] All 17 functional requirements (FR-001 to FR-017) covered
- [x] All 3 user stories (US1, US2, US3) mapped to task phases
- [x] All entities from data-model.md addressed (PortMapping, ContainerWithPorts)
- [x] All contracts from contracts/ implemented (portUtils, StackCard, StackDetail, App)
- [x] All decisions from research.md incorporated (parsing, protocol, styling, title)
- [x] Independent test criteria defined for each user story

### Quality Validation ✅

- [x] Each user story can be tested independently
- [x] MVP scope identified (US1 only)
- [x] Dependencies clearly documented
- [x] Parallel opportunities identified
- [x] Estimated times realistic
- [x] Manual test scenarios included

---

**Ready to implement? Start with Phase 1 (Foundation): T001-T008** 🚀
