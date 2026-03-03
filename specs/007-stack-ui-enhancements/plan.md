# Implementation Plan: Stack UI Enhancements

**Branch**: `007-stack-ui-enhancements` | **Date**: 2026-03-01 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/007-stack-ui-enhancements/spec.md`

## Summary

This feature enhances the Docker-CD web UI with three priority improvements: (P1) Quick port access from grid view by displaying container names with clickable links for containers with port mappings, opening the lowest port number; (P2) Browser tab identification by showing hostname in title as "<hostname> - Docker-CD"; (P3) Enhanced port display on details page by fixing duplicate port listings and converting them to clickable pill elements. All changes are frontend-only, using existing backend API data (ContainerInfo with ports as comma-separated strings like "8080:80/tcp"). Technical approach uses Vue 3 composables for port parsing, Naive UI pills/tags for visual styling, and CSS Grid for uniform card height within rows.

## Technical Context

**Language/Version**: TypeScript 5.9, Vue 3.5 (Composition API)  
**Primary Dependencies**: Naive UI 2.43 (component library), Pinia 2.2 (state management), Vue Router 4.5  
**Storage**: N/A (frontend only, uses existing backend API)  
**Testing**: Vitest 4.0.18 with Vue Test Utils, @testing-library/vue, happy-dom  
**Target Platform**: Modern browsers (Chrome, Firefox, Safari, Edge) via Vite 7.3 build  
**Project Type**: Web application frontend (SPA - Single Page Application)  
**Performance Goals**: <100ms interaction response, smooth CSS Grid layout reflows  
**Constraints**: Must use existing ContainerInfo API format (ports as comma-separated string), no backend changes allowed  
**Scale/Scope**: 3 pages/components modified, ~15 new test cases, estimated 200-300 LOC

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Rationale |
|-----------|--------|-----------|
| **I. GitOps Source of Truth** | ✅ PASS | Not applicable - this is a frontend UI enhancement that displays existing data without modifying any GitOps reconciliation logic or source of truth |
| **II. Continuous Reconciliation** | ✅ PASS | Not applicable - no changes to reconciliation, webhook handling, or polling logic; only affects how existing data is displayed in the UI |
| **III. Container-First Runtime** | ✅ PASS | Not applicable - frontend changes only; existing containerized deployment remains unchanged |
| **IV. Safe Docker Compose Apply** | ✅ PASS | Not applicable - no changes to reconciliation, compose apply, or destructive actions; purely presentational changes |
| **V. Automated Testing Baseline** | ✅ PASS | Will be satisfied - plan includes comprehensive test coverage for all new UI interactions (port link parsing, click handlers, card layout, title updates) using existing Vitest infrastructure |

**Operational Constraints**: ✅ All constraints satisfied - no changes to webhooks, logging, configuration, or secrets management. Frontend respects existing API contract.

**Development Workflow**: ✅ Will be satisfied - this plan document exists before implementation, existing CI will run tests on PR, changes are purely additive UI enhancements with no reconciliation impact.

**Overall Status**: ✅ **APPROVED** - No constitution violations. All principles either not applicable (frontend-only change) or will be satisfied through planned test coverage.

### Post-Design Re-Evaluation (Phase 1 Complete)

**Date**: 2026-03-01

After completing Phase 0 (Research) and Phase 1 (Design), re-evaluating all principles:

| Principle | Post-Design Status | Notes |
|-----------|-------------------|-------|
| **I. GitOps Source of Truth** | ✅ PASS | Confirmed: No changes to reconciliation or Git operations. Research confirms frontend-only port display enhancements. |
| **II. Continuous Reconciliation** | ✅ PASS | Confirmed: No changes to webhook or polling logic. Design uses existing SSE events for real-time updates. |
| **III. Container-First Runtime** | ✅ PASS | Confirmed: No deployment changes. Frontend continues to run in existing container. |
| **IV. Safe Docker Compose Apply** | ✅ PASS | Confirmed: No changes to compose operations. Port display is read-only presentation of existing container data. |
| **V. Automated Testing Baseline** | ✅ PASS | Confirmed: Test plan includes 15+ test cases across unit, component, and integration levels. Coverage target >90%. |

**Operational Constraints**: ✅ No changes identified during design phase.

**Development Workflow**: ✅ Satisfied - All Phase 0 and Phase 1 artifacts created before implementation begins.

**Final Approval**: ✅ **CONSTITUTION COMPLIANT** - Design does not introduce any violations. Safe to proceed to implementation.

## Project Structure

### Documentation (this feature)

```text
specs/007-stack-ui-enhancements/
├── spec.md              # Feature specification (completed)
├── plan.md              # This file
├── research.md          # Phase 0 output (generated below)
├── data-model.md        # Phase 1 output (generated below)
├── quickstart.md        # Phase 1 output (generated below)
├── contracts/           # Phase 1 output (generated below)
│   └── component-apis.md
└── checklists/
    └── requirements.md  # Quality validation (completed)
```

### Source Code (repository root)

```text
frontend/
├── src/
│   ├── components/
│   │   ├── StackCard.vue          # [MODIFY] Add container list display
│   │   ├── StatusBadge.vue        # [NO CHANGE]
│   │   └── ThemeToggle.vue        # [NO CHANGE]
│   ├── pages/
│   │   ├── StacksGrid.vue         # [MODIFY] Update card layout CSS for uniform height
│   │   └── StackDetail.vue        # [MODIFY] Fix duplicate ports, add clickable pills
│   ├── services/
│   │   └── api.ts                 # [NO CHANGE] Uses existing ContainerInfo
│   ├── store/
│   │   └── stacks.ts              # [NO CHANGE]
│   ├── composables/               # [CREATE NEW]
│   │   └── usePortParsing.ts     # [NEW] Parse port strings, find lowest port
│   ├── utils/                     # [CREATE NEW]
│   │   └── portUtils.ts          # [NEW] Port parsing and URL construction utilities
│   ├── App.vue                    # [MODIFY] Add dynamic document title
│   └── main.ts                    # [NO CHANGE]
└── tests/
    ├── port-utils.spec.ts         # [NEW] Unit tests for port parsing
    ├── stack-card-ports.spec.ts   # [NEW] Component tests for card container display
    └── stack-detail-pills.spec.ts # [NEW] Component tests for clickable pills
```

**Structure Decision**: Web application structure (Option 2 pattern) - frontend-only changes. All modifications are in the `frontend/` directory. New utilities/composables follow existing Vue 3 Composition API patterns. No backend changes required as existing API provides all necessary data.

## Complexity Tracking

*No constitution violations identified - this section is not needed for this feature.*
