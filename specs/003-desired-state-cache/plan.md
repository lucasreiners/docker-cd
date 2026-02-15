# Implementation Plan: Desired State Cache and Refresh

**Branch**: `003-desired-state-cache` | **Date**: 2026-02-15 | **Spec**: [/Users/lucasreiners/Documents/Code/docker-cd/specs/003-desired-state-cache/spec.md](specs/003-desired-state-cache/spec.md)
**Input**: Feature specification from `/Users/lucasreiners/Documents/Code/docker-cd/specs/003-desired-state-cache/spec.md`

## Summary

Implement a desired-state cache refreshed from Git on startup, via webhook, via manual API, and periodically. The cache stores compose file hashes, per-stack sync status, and the synced Git revision, with JSON-only endpoints (except the root page), webhook signature validation (GitHub HMAC), and a single-slot refresh queue.

## Technical Context

**Language/Version**: Go 1.25.7  
**Primary Dependencies**: Gin v1.11.0, go-git v5.16.5  
**Storage**: In-memory cache (no persistence)  
**Testing**: go test (unit + handler tests)  
**Target Platform**: Linux container runtime  
**Project Type**: Single service (Go module)  
**Performance Goals**: Refresh under 30 seconds for <=200 stack directories  
**Constraints**: Read-only Git access; JSON responses for all endpoints except `/`; GitHub webhook signature validation with configured secret  
**Scale/Scope**: Single repo, <=200 stacks per refresh

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- GitOps source of truth: repo/ref/path defined; drift policy documented (detect-only; no apply in this feature).
- Continuous reconciliation: webhook + periodic reconcile strategy defined (poll interval).
- Container-first runtime: container deployment + health checks remain unchanged.
- Safe compose apply: no compose apply in this feature; defer to future reconcile phase.
- Automated testing baseline: unit tests for refresh queue, cache updates, and endpoints; integration test plan for webhook/refresh.

## Project Structure

### Documentation (this feature)

```text
/Users/lucasreiners/Documents/Code/docker-cd/specs/003-desired-state-cache/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
└── tasks.md
```

### Source Code (repository root)

```text
/Users/lucasreiners/Documents/Code/docker-cd/
├── cmd/
│   └── docker-cd/
├── internal/
│   ├── config/
│   ├── docker/
│   ├── git/
│   ├── http/
│   ├── render/
│   ├── desiredstate/        # new cache + hash logic
│   └── refresh/             # new refresh scheduler + queue
└── tests/
    └── integration/
```

**Structure Decision**: Single Go service with new internal packages for desired-state storage and refresh orchestration.

## Phase 0: Research

- Capture decisions for Git tree reads, hashing scope, in-memory cache, and refresh queue strategy in `/Users/lucasreiners/Documents/Code/docker-cd/specs/003-desired-state-cache/research.md`.

## Phase 1: Design & Contracts

- Define data model for DesiredStateSnapshot, StackRecord, and refresh status in `/Users/lucasreiners/Documents/Code/docker-cd/specs/003-desired-state-cache/data-model.md`.
- Define API contracts for refresh endpoints and desired-state retrieval in `/Users/lucasreiners/Documents/Code/docker-cd/specs/003-desired-state-cache/contracts/openapi.yaml`.
- Provide quickstart instructions in `/Users/lucasreiners/Documents/Code/docker-cd/specs/003-desired-state-cache/quickstart.md`.

## Phase 2: Planning

- Identify tasks for config updates (poll interval, webhook secret/signature validation), cache storage, refresh queue, periodic scheduler, and JSON endpoints.
- Plan tests for refresh queue behavior, compose hashing, webhook auth, and desired-state response content.

## Constitution Check (Post-Design)

- GitOps source of truth: desired state derived only from Git, no local edits.
- Continuous reconciliation: periodic refresh configured and webhook/manual triggers supported.
- Container-first runtime: no changes required; health check remains.
- Safe compose apply: out of scope in this feature; no apply actions added.
- Automated testing baseline: tests planned for webhook handling and refresh logic.

## Complexity Tracking

No constitution violations requiring justification.
