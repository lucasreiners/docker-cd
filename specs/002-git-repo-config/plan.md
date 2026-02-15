# Implementation Plan: Git Repository Configuration

**Branch**: `002-git-repo-config` | **Date**: 2026-02-15 | **Spec**: [specs/002-git-repo-config/spec.md](specs/002-git-repo-config/spec.md)
**Input**: Feature specification from `/specs/002-git-repo-config/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Add environment-based Git repository configuration (HTTPS URL, token, revision, optional deployment directory) and validate access on startup using a read-only Git client. Reject missing or invalid configuration, and surface non-secret repository details on the root status page.

## Technical Context

**Language/Version**: Go 1.25.x  
**Primary Dependencies**: Gin v1.11.0, go-git v5 (read-only validation)  
**Storage**: N/A  
**Testing**: go test, httptest  
**Target Platform**: Linux container  
**Project Type**: single service  
**Performance Goals**: validate repository access within 10 seconds at startup  
**Constraints**: HTTPS-only repo URLs, read-only auth, no secret exposure in logs or UI  
**Scale/Scope**: single service instance, low request volume

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- GitOps source of truth: repo/ref/path will be defined via env vars for startup validation
- Continuous reconciliation: not changed in this feature (no reconciliation logic added)
- Container-first runtime: config remains environment-driven; container workflow unchanged
- Safe compose apply: not changed in this feature (no apply logic added)
- Automated testing baseline: new unit tests planned for config validation and handler output

## Project Structure

### Documentation (this feature)

```text
specs/002-git-repo-config/
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
internal/config/
internal/docker/
internal/http/
internal/render/
internal/git/            # new package for repo validation
tests/integration/
docker/
```

**Structure Decision**: Single Go service using cmd/ and internal/ packages, with a new internal/git package for read-only repository validation.

## Constitution Check (Post-Design)

- GitOps source of truth: env vars cover repo URL, revision, and deploy path; access validated at startup
- Continuous reconciliation: unchanged and not in scope for this feature
- Container-first runtime: no change; config remains env-based and container-safe
- Safe compose apply: unchanged and not in scope for this feature
- Automated testing baseline: unit tests for config parsing, validation failures, and handler rendering planned

## Complexity Tracking

No constitution violations.
