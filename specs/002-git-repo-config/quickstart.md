# Quickstart: Git Repository Configuration

## Prerequisites

- Docker Engine running
- Docker Compose v2 (`docker compose`)

## Configure

```bash
export GIT_REPO_URL="https://github.com/your-org/your-repo.git"
export GIT_ACCESS_TOKEN="<read-only-token>"
export GIT_REVISION="main"
# Optional: defaults to repository root
export GIT_DEPLOY_DIR="deployments/host-a"
```

## Run

```bash
docker compose -f docker/docker-compose.yml up --build
```

## Verify

```bash
curl http://localhost:8080/
```

You should see the repository URL, revision, and deployment directory in the output. The access token is never shown.

## Tests

```bash
go test ./... -v
```
