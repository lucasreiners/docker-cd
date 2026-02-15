# Docker-CD

ArgoCD, but for Docker. A GitOps continuous delivery agent for Docker Compose environments.

## Run

```bash
docker compose -f docker/docker-compose.yml up --build
```

## Tests

```bash
# Unit tests
go test ./... -v

# Integration tests (requires a running Docker daemon)
go test -tags integration ./tests/integration/... -v
```
