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
| `UPDATER_ENABLED` | no | `false` | Enable/disable scheduled image updates |
| `UPDATER_CRON` | no | `0 3 * * *` | Cron expression for update schedule (default: 3 AM UTC daily) |

The service validates repository access on startup and exits immediately if any required variable is missing or credentials are invalid.

### Webhook Setup

Configure a GitHub webhook pointing to `https://your-host/api/webhook` with content type `application/json`. Set the same secret in both GitHub and the `WEBHOOK_SECRET` environment variable. If no secret is configured, all webhook requests are accepted without signature validation.

### Scheduled Updates

The updater feature automatically checks for new container images and updates running stacks. When enabled, it:

- Pulls latest images for all managed stacks
- Detects image changes by comparing digests
- Triggers reconciliation to recreate containers with new images
- Prunes unused images to reclaim disk space

To enable scheduled updates:

```bash
export UPDATER_ENABLED=true
export UPDATER_CRON="0 3 * * *"  # 3 AM UTC daily (default)
```

The updater is disabled by default. Update cycles run sequentially to avoid resource contention, and the service gracefully waits for active cycles to complete during shutdown.

## Run

```bash
export GIT_REPO_URL="https://github.com/your-org/your-repo.git"
export GIT_ACCESS_TOKEN="<token>"
export GIT_REVISION="main"

docker compose up --build
```

### Local Clone Volume

The service stores a local working copy of the repository at a fixed path inside
the container: `/repo`. Ensure your compose files mount a persistent volume at
that path so refresh, reconciliation, and updates can use the local clone.

If a refresh fails, the last successful clone is kept, but reconciliation and
updates are blocked until the next successful refresh. The UI surfaces this as
`updatesBlocked` in refresh status.

- Backend API: <http://localhost:8080>
- Frontend UI: <http://localhost:8081>

### Frontend development

```bash
cd frontend
bun install
bun run dev     # starts Vite dev server on :8081, proxies /api to :8080
```

## Tests

```bash
# Go unit tests
cd backend && go test ./... -v

# Frontend unit tests
cd frontend && bun run test

# Integration tests (requires a running Docker daemon)
cd backend && go test -tags integration ./tests/integration/... -v
```
