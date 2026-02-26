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

### Webhook Setup

Configure a GitHub webhook pointing to `https://your-host/api/webhook` with content type `application/json`. Set the same secret in both GitHub and the `WEBHOOK_SECRET` environment variable. If no secret is configured, all webhook requests are accepted without signature validation.

## Run

```bash
export GIT_REPO_URL="https://github.com/your-org/your-repo.git"
export GIT_ACCESS_TOKEN="<token>"
export GIT_REVISION="main"

docker compose up --build
```

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
