# Implementation Plan: Docker-CD Skeleton Runtime

**Branch**: `001-skeleton-runtime` | **Date**: 2026-02-15 | **Spec**: [specs/001-skeleton-runtime/spec.md](specs/001-skeleton-runtime/spec.md)
**Input**: Feature specification from `/specs/001-skeleton-runtime/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Deliver a minimal, containerized Docker-CD service that exposes a root
HTTP endpoint returning ASCII art and the current running container
count from the host Docker Engine via a mounted socket. Use a small Go
web server built with Gin and a lightweight Dockerfile that includes the
Docker CLI and Compose plugin, plus a local Docker Compose file for
testing. Include unit tests for response rendering and Docker CLI
integration, and an integration smoke test when Docker is available.

## Technical Context

**Language/Version**: Go 1.25
**Primary Dependencies**: Gin (`github.com/gin-gonic/gin`), Go standard
library (`os/exec`)
**Storage**: N/A
**Testing**: `go test`, `net/http/httptest`, `httptest` with Gin
**Target Platform**: Linux container runtime (Docker Engine)
**Project Type**: single service
**Performance Goals**: Root endpoint responds in <1s p95 on a typical
developer laptop with Docker running
**Constraints**: Must run in a container with the host Docker socket
mounted; no long-lived secrets stored in the container
**Scale/Scope**: Single-node local testing and development usage

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- GitOps source of truth: `RuntimeConfig` documents repo/ref/path and a
  drift policy placeholder for future reconciliation features.
- Continuous reconciliation: approach outlined in research; skeleton
  provides the long-lived container runtime that later features extend.
- Container-first runtime: Dockerfile and compose harness included in
  the plan.
- Safe compose apply: compose usage and future plan/diff strategy
  documented; this feature does not perform destructive actions.
- Automated testing baseline: unit tests for response/rendering and
  Docker CLI execution; optional integration smoke test when Docker is
  available.

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)
```text
cmd/docker-cd/
└── main.go

internal/config/
internal/docker/
internal/http/
internal/render/

tests/integration/

docker/
├── Dockerfile
└── docker-compose.yml
```

**Structure Decision**: Single Go service with a conventional `cmd/` and
`internal/` layout. Integration tests live under `tests/` for Docker
socket-dependent checks. Container artifacts live under `docker/`.

## Constitution Check (Post-Design)

- GitOps source of truth: `RuntimeConfig` captures repo/ref/path and
  drift policy placeholders for future features.
- Continuous reconciliation: documented design in research; runtime
  container foundation established in this feature.
- Container-first runtime: Dockerfile + compose harness planned and
  documented in quickstart.
- Safe compose apply: compose usage documented; destructive actions not
  implemented in this feature.
- Automated testing baseline: unit tests and integration smoke test
  planned for the skeleton runtime.

## Complexity Tracking

No constitution violations identified.
