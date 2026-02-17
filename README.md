# Docker-CD

ArgoCD, but for Docker. A GitOps continuous delivery agent for Docker Compose environments.

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GIT_REPO_URL` | yes | — | HTTPS Git repository URL |
| `GIT_ACCESS_TOKEN` | yes | — | Read-only access token |
| `GIT_REVISION` | yes | — | Branch, tag, or ref to deploy |
| `GIT_DEPLOY_DIR` | no | `/` (repo root) | Subdirectory within the repo |
| `PORT` | no | `8080` | HTTP listen port |
| `PROJECT_NAME` | no | `Docker-CD` | Name shown in status page |
| `WEBHOOK_SECRET` | no | — | HMAC-SHA256 secret for GitHub webhook verification |
| `REFRESH_POLL_INTERVAL` | no | — | Periodic refresh interval (e.g. `5m`, `30s`). Disabled if empty |
| `RECONCILE_ENABLED` | no | `true` | Enable/disable stack reconciliation |
| `RECONCILE_REMOVE_ENABLED` | no | `false` | Allow removal of stacks deleted from desired state |
| `DRIFT_POLICY` | no | `revert` | Drift handling: `revert` (auto-fix) or `flag` (require ack) |

The service validates repository access on startup and exits immediately if any required variable is missing or credentials are invalid.

## Web Frontend

A Vue 3 SPA provides a real-time dashboard for monitoring stacks. Updates are pushed via Server-Sent Events (SSE) — no polling required.

- **Grid view** — Argo CD-style card grid of all discovered stacks with live status badges
- **Detail view** — Full sync metadata, revision, commit message, and error details
- **Client-side filtering** — Filter by status or search by stack path
- **Connection banner** — Visual indicator when SSE disconnects or reconnects

The frontend runs in a separate container (Nginx) on port **8081** and proxies API calls to the backend.

| Variable | Default | Description |
|----------|---------|-------------|
| `DOCKER_CD_API_BASE_URL` | — | Backend URL for the frontend container (e.g. `http://docker-cd:8080`) |

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | HTML status page with container count and repo info |
| `POST` | `/api/webhook` | GitHub webhook endpoint (validates `X-Hub-Signature-256` if `WEBHOOK_SECRET` is set) |
| `POST` | `/api/refresh` | Trigger a manual desired-state refresh |
| `GET` | `/api/refresh-status` | Get current refresh status and cached Git revision |
| `GET` | `/api/stacks` | List all stacks with sync status and metadata |
| `GET` | `/api/stacks/containers/*path` | List containers for a specific stack (by compose project) |
| `GET` | `/api/events` | SSE stream of stack updates (`stack.snapshot`, `stack.upsert`, `stack.delete`, `refresh.status`) |
| `POST` | `/api/reconcile/ack` | Acknowledge drift for a flagged stack |

### Stack Sync Metadata

After reconciliation, each stack in `/api/stacks` includes:

| Field | Description |
|-------|-------------|
| `containersRunning` | Number of running containers in the stack |
| `containersTotal` | Total number of containers in the stack |
| `syncedRevision` | Git revision the stack was last synced to |
| `syncedCommitMessage` | Commit message for the synced revision |
| `syncedComposeHash` | Compose file hash at last sync |
| `syncedAt` | Timestamp of last successful sync (RFC3339) |
| `lastSyncAt` | Timestamp of last reconciliation outcome (RFC3339) |
| `lastSyncStatus` | Last reconciliation result: `syncing`, `synced`, or `failed` |
| `lastSyncError` | Error message if last sync failed |

Sync metadata is stored as Docker container labels and survives service restarts.

### Webhook Setup

Configure a GitHub webhook pointing to `https://your-host/api/webhook` with content type `application/json`. Set the same secret in both GitHub and the `WEBHOOK_SECRET` environment variable. If no secret is configured, all webhook requests are accepted without signature validation.

## Run

```bash
export GIT_REPO_URL="https://github.com/your-org/your-repo.git"
export GIT_ACCESS_TOKEN="<token>"
export GIT_REVISION="main"

docker compose -f docker/docker-compose.yml up --build
```

- Backend API: http://localhost:8080
- Frontend UI: http://localhost:8081

### Frontend development

```bash
cd frontend
bun install
bun run dev     # starts Vite dev server on :8081, proxies /api to :8080
```

## Tests

```bash
# Go unit tests
go test ./... -v

# Frontend unit tests
cd frontend && bun run test

# Integration tests (requires a running Docker daemon)
go test -tags integration ./tests/integration/... -v
```
