# Quickstart: Desired State Cache and Refresh

## Configure

Set Git configuration and refresh settings:

- GIT_REPO_URL
- GIT_ACCESS_TOKEN
- GIT_REVISION
- GIT_DEPLOY_DIR (optional)
- WEBHOOK_SECRET (optional, GitHub webhook secret)
- REFRESH_POLL_INTERVAL (e.g., 60s)

## Run

```bash
export GIT_REPO_URL=https://github.com/org/repo.git
export GIT_ACCESS_TOKEN=your-token
export GIT_REVISION=main
export GIT_DEPLOY_DIR=deployments
export WEBHOOK_SECRET=optional-secret
export REFRESH_POLL_INTERVAL=60s

go run ./cmd/docker-cd
```

## Verify

- Root page: `GET /` returns the status page (plain text).
- Manual refresh: `POST /api/refresh` returns JSON status.
- Webhook refresh: `POST /api/webhook` with optional `X-Hub-Signature-256` header returns JSON status.
- Refresh status: `GET /api/refresh-status` returns the cached revision and refresh status.
- Stacks: `GET /api/stacks` returns all stacks with sync status.

## Example Requests

```bash
curl -s -X POST http://localhost:8080/api/refresh
```

```bash
curl -s -X POST http://localhost:8080/api/webhook \
  -H "X-Hub-Signature-256: sha256=<hmac>"
```

```bash
curl -s http://localhost:8080/api/refresh-status

curl -s http://localhost:8080/api/stacks
```
