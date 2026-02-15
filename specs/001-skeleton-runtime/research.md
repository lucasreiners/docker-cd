# Research: Docker-CD Skeleton Runtime

## Decision 1: Language and HTTP framework

**Decision**: Use Go 1.22 with the Gin web framework.

**Rationale**:
- Small, static binary keeps container image size and startup time low.
- Gin provides ergonomic routing and middleware without heavy overhead.
- Built-in testing (`go test`, `httptest`) remains sufficient for the
  skeleton's HTTP and rendering needs.

**Alternatives considered**:
- Go standard library (`net/http`): Minimal dependencies but more manual
  routing and middleware wiring.
- Python (FastAPI/Flask): Faster iteration but larger images and runtime
  overhead.
- Node.js (Express): Familiar for JS teams but higher dependency
  footprint and larger base images.

## Decision 2: Docker CLI integration approach

**Decision**: Execute `docker ps --format` via `os/exec` using the
mounted Docker socket at `/var/run/docker.sock`.

**Rationale**:
- Aligns with the requirement to use the Docker CLI inside the container.
- Keeps the implementation straightforward without an extra Docker SDK.
- Works consistently when the socket is mounted by Compose.

**Alternatives considered**:
- Docker SDK API calls: More direct but conflicts with the explicit CLI
  requirement and adds dependency weight.

## Decision 3: Testing strategy

**Decision**: Unit tests use a command-runner interface to stub Docker
CLI output; integration smoke test runs only when a Docker socket is
available.

**Rationale**:
- Unit tests remain deterministic and fast.
- Integration test validates the mounted socket path and CLI invocation
  when Docker is present.

**Alternatives considered**:
- Full end-to-end Docker Compose tests in all environments: Valuable but
  too heavy for a skeleton and not always available in CI.
