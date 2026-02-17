# Phase 0 Research: Minimal Web Frontend

## Decision 1: Frontend framework + UI library

**Decision**: Vue 3 + Vite + Naive UI

**Rationale**:
- Matches stated preference for Vue while keeping boilerplate low.
- Naive UI provides a consistent, enterprise-style theme and rich components (cards, tables, status badges) for an Argo CD-like UI.
- Vite offers fast dev/build cycles with a simple configuration surface.

**Alternatives considered**:
- React + MUI: fastest access to admin templates, but higher glue-code overhead and less aligned with stated preference.
- Vue 3 + Element Plus: mature and popular, but heavier and less flexible theming for a minimal UI.
- Svelte + Skeleton: excellent DX and bundle size, but fewer admin templates and less mature component coverage.

## Decision 2: Frontend state management

**Decision**: Pinia for in-memory state + EventSource for SSE

**Rationale**:
- Pinia integrates cleanly with Vue 3, supports a simple normalized map of stacks, and fits the in-memory requirement.
- Native `EventSource` handles SSE reconnects and minimizes dependencies.

**Alternatives considered**:
- Vuex: heavier and no longer the preferred Vue 3 store.
- RxJS: powerful, but unnecessary for the minimal state updates required.

## Decision 3: SSE event strategy

**Decision**: Initial full fetch from `/api/stacks`, then SSE `stack.upsert` events carrying full stack records

**Rationale**:
- Full record updates avoid merge/patch complexity and align with the spec.
- SSE auto-reconnect with `Last-Event-ID` can be leveraged later if a replay buffer is added.

**Alternatives considered**:
- SSE snapshots only: simpler server logic but larger payloads on every update.
- WebSockets: bidirectional overhead not required for read-only UI.

## Decision 4: Frontend container serving

**Decision**: Separate frontend container serving static assets (Nginx)

**Rationale**:
- Keeps backend unchanged and aligns with the spec requirement for a separate container.
- Nginx is lightweight and common for static asset delivery.

**Alternatives considered**:
- Serve assets from Gin: increases backend surface and couples release cadence.
- Caddy: flexible but less common in existing project patterns.

## Decision 5: Backend API base URL configuration

**Decision**: Use a single environment variable for frontend API base URL (proposed: `DOCKER_CD_API_BASE_URL`)

**Rationale**:
- Explicit configuration works in local compose and production deployments.
- Allows referencing the backend container name when deployed in the same stack.

**Alternatives considered**:
- Derive from window location: fails when UI is served from a different host/port.
- Multiple env vars per endpoint: increases configuration surface without benefit.
