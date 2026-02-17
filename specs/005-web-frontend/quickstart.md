# Quickstart: Minimal Web Frontend

## Prerequisites

- Docker and Docker Compose
- Bun (for local development)
- A Git repository URL containing stack definitions

## Local run (compose)

1. Set required environment variables:

```bash
export GIT_REPO_URL="https://github.com/your-org/your-repo"
export GIT_ACCESS_TOKEN="YOUR_TOKEN"
```

2. Build and start the stack:

```bash
docker compose -f docker/docker-compose.yml up --build
```

3. Open the UI:

- Backend API: http://localhost:8080/api/stacks
- SSE stream: http://localhost:8080/api/events
- Frontend UI: http://localhost:8081

## Local development (frontend only)

1. Install dependencies:

```bash
cd frontend && bun install
```

2. Start the dev server (proxies API to localhost:8080):

```bash
bun run dev
```

3. Open http://localhost:8081

## Run tests

### Go backend tests

```bash
go test ./internal/... -count=1 -short
```

### Frontend tests

```bash
cd frontend && bun run test
```

## Validation

| Step | Expected | Status |
|------|----------|--------|
| `go build ./...` | Clean compilation | PASS |
| `go test ./internal/... -short` | All 8 packages pass | PASS |
| `cd frontend && bun run test` | 29 tests, 4 files, all pass | PASS |
| Backend: GET /api/stacks | Returns JSON array of StackRecord | PASS |
| Backend: GET /api/events | SSE stream with stack.snapshot event | PASS |
| Frontend: Grid renders on / | Stack cards in grid layout | PASS (by test) |
| Frontend: Detail on /stack/:path+ | Stack details rendered | PASS (by test) |
| Frontend: ConnectionBanner | Warning on disconnect | PASS (by test) |

## Notes

- The frontend reads `DOCKER_CD_API_BASE_URL` at container start (via nginx envsubst + config.js injection).
- When deployed alongside the backend in the same compose project, the backend service name is used as the host (e.g., `http://docker-cd:8080`).
- In development mode, the Vite dev server proxies `/api` requests to `http://localhost:8080`.
- Frontend port: 8081 (nginx in production, Vite in dev)
- Backend port: 8080
