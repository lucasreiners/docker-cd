# Implementation Plan: Minimal Web Frontend

**Branch**: `005-web-frontend` | **Date**: 2026-02-16 | **Spec**: [specs/005-web-frontend/spec.md](specs/005-web-frontend/spec.md)
**Input**: Feature specification from `/specs/005-web-frontend/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Deliver a minimal web UI that lists stacks, shows sync status, and updates via SSE without polling. The frontend is a Vue 3 SPA built with Vite and a UI component library to minimize custom styling, served in a separate container and configured with an API base URL environment variable.

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: Go 1.26.0 (backend), Bun 1.3.9 (frontend build/runtime)  
**Primary Dependencies**: Gin (backend), Vue 3 + Vite + Naive UI + Pinia + Vue Router (frontend)  
**Storage**: N/A (in-memory state in backend store and browser state)  
**Testing**: `go test` + Testcontainers (backend), Vitest + @testing-library/vue (frontend), Playwright smoke test (optional)  
**Target Platform**: Linux containers + modern browsers (Chrome/Edge/Firefox/Safari)  
**Project Type**: web (backend + frontend)  
**Performance Goals**: UI load < 5s; SSE update reflected < 2s  
**Constraints**: SSE push only (no polling), separate frontend container, API base URL via env var  
**Scale/Scope**: 500+ stacks on a single page; low concurrency (ops users)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- GitOps source of truth: repo/ref/path defined; drift policy documented (pass - unchanged)
- Continuous reconciliation: webhook + periodic reconcile strategy defined (pass - unchanged)
- Container-first runtime: container deployment + health checks planned (pass - frontend container added)
- Safe compose apply: plan/diff + destructive opt-in handling documented (pass - unchanged)
- Automated testing baseline: unit and integration test coverage planned (pass - add frontend tests)

## Project Structure

### Documentation (this feature)

```text
specs/005-web-frontend/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)
<!--
  ACTION REQUIRED: Replace the placeholder tree below with the concrete layout
  for this feature. Delete unused options and expand the chosen structure with
  real paths (e.g., apps/admin, packages/something). The delivered plan must
  not include Option labels.
-->

```text
cmd/
└── docker-cd/
  └── main.go

internal/
├── config/
├── desiredstate/
├── docker/
├── git/
├── http/
├── reconcile/
├── refresh/
└── render/

frontend/
├── package.json
├── vite.config.ts
├── index.html
├── src/
│   ├── main.ts
│   ├── App.vue
│   ├── pages/
│   │   ├── StacksGrid.vue
│   │   └── StackDetail.vue
│   ├── components/
│   │   ├── StackCard.vue
│   │   ├── StatusBadge.vue
│   │   └── ConnectionBanner.vue
│   ├── services/
│   │   ├── api.ts
│   │   └── sse.ts
│   ├── store/
│   │   └── stacks.ts
│   └── styles/
│       └── theme.css
└── tests/
  └── stacks.spec.ts

docker/
├── docker-compose.yml
└── frontend.Dockerfile
```

**Structure Decision**: Add a top-level `frontend/` SPA alongside the existing Go backend, plus a dedicated frontend Dockerfile in `docker/` for the static build.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |

## Constitution Check (Post-Design)

- GitOps source of truth: unchanged, still driven by repo snapshot
- Continuous reconciliation: unchanged, still webhook + periodic refresh
- Container-first runtime: frontend delivered as container with env-configurable API base URL
- Safe compose apply: no change to reconcile behavior
- Automated testing baseline: add frontend unit tests for SSE update logic
