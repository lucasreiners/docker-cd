# Feature: Vue administration console

Status: implementation

Reference: [study](study.md)

## User stories

1. Navigate Overview, Stacks, Operations, Repository and System in a responsive
   administration shell, including keyboard search, drawers and saved table views.
2. Inspect deployment and runtime independently in tabbed stack details, including
   source configuration, container inspection, and operation history.
3. Follow server-owned operations through navigation/reconnect and retrieve their
   persisted results after browser and backend restarts.
4. Review a structured pending-drift state and explicitly apply the reviewed
   revision to that stack; retry a stack without triggering every stack.
5. Read effective policies, scheduler state, repository configuration and runtime
   capability/health information without exposing credentials.
6. Investigate failures through bounded container logs, desired/applied Compose
   comparison, and retained webhook delivery outcomes.

## Requirements

- Retain Vue 3, Vite, TypeScript, Pinia and the Go backend. Adopt PrimeVue.
- Preserve old stack URLs and real Docker lifecycle behavior.
- Persist bounded operation summaries and important steps, with restart recovery.
- Targeted actions must check current desired revision, refresh readiness and
  reconciliation policy; work must outlive the originating HTTP request.
- Keep environment-owned configuration read-only and Git authoritative.
- Show actual unavailable/disabled states and actionable API errors.
- Keep high-frequency image progress live; expose history through a query API.
- Provide headed Playwright fixtures and real-Docker regression tests.

## Acceptance

Desktop/mobile navigation, filtering, column preferences, theme, tab deep links,
drawer focus, live progress, reload persistence, pending acknowledgement, stale
revision rejection and persisted interrupted operations have automated coverage.

## Later product extensions

Multi-host/repository management, accounts, editable agent configuration,
notifications and historical metrics remain follow-on product features identified
by the study. This implementation delivers the single-instance admin console.
