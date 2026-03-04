# Quickstart: Local Repository Clone

**Feature**: 008-local-git-clone
**Date**: 2026-03-04

## Prerequisites

- Docker and Docker Compose installed
- GitHub access token with read-only permissions

## Configure Environment

```bash
export GIT_REPO_URL="https://github.com/your-org/your-repo.git"
export GIT_ACCESS_TOKEN="<token>"
export GIT_REVISION="main"
```

## Run (local)

1. Ensure the local compose file mounts the persistent volume at `/repo`.
2. Start the service:

```bash
docker compose up --build
```

## Run (production)

1. Ensure the production compose file mounts the persistent volume at `/repo`.
2. Deploy the service as usual (compose, swarm, or equivalent).

## Verify

- Check refresh status:

```bash
curl http://localhost:8080/api/refresh-status
```

- Confirm `updatesBlocked=false` after a successful refresh.
- Trigger manual refresh and confirm status updates in the UI.
- If a refresh fails, verify `updatesBlocked=true` and `blockedReason` are set until the next successful refresh.

## Integration Tests (Docker-in-Docker)

```bash
cd backend

go test -tags integration ./tests/integration/... -v
```
