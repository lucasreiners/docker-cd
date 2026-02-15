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

The service validates repository access on startup and exits immediately if any required variable is missing or credentials are invalid.

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | HTML status page with container count and repo info |
| `POST` | `/api/webhook` | GitHub webhook endpoint (validates `X-Hub-Signature-256` if `WEBHOOK_SECRET` is set) |
| `POST` | `/api/refresh` | Trigger a manual desired-state refresh |
| `GET` | `/api/refresh-status` | Get current refresh status and cached Git revision |
| `GET` | `/api/stacks` | List all stacks with sync status |

### Webhook Setup

Configure a GitHub webhook pointing to `https://your-host/api/webhook` with content type `application/json`. Set the same secret in both GitHub and the `WEBHOOK_SECRET` environment variable. If no secret is configured, all webhook requests are accepted without signature validation.

## Run

```bash
export GIT_REPO_URL="https://github.com/your-org/your-repo.git"
export GIT_ACCESS_TOKEN="<token>"
export GIT_REVISION="main"

docker compose -f docker/docker-compose.yml up --build
```

## Tests

```bash
# Unit tests
go test ./... -v

# Integration tests (requires a running Docker daemon)
go test -tags integration ./tests/integration/... -v
```
