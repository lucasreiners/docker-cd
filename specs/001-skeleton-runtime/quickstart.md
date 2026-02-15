# Quickstart: Docker-CD Skeleton Runtime

## Prerequisites

- Docker Engine installed and running
- Docker Compose v2 available (`docker compose`)

## Build and Run (Docker Compose)

```bash
docker compose -f docker/docker-compose.yml up --build
```

The service starts on port 8080 by default.

## Verify

```bash
curl http://localhost:8080/
```

Expected response includes ASCII art with "Docker-CD" and a line showing
"Running containers: <count>".

## Configuration

- `PORT`: HTTP port (default 8080)
- `PROJECT_NAME`: Display name (default "Docker-CD")
- `DOCKER_SOCKET`: Socket path (default `/var/run/docker.sock`)

## Notes

- The compose file mounts the host Docker socket into the container at
  `/var/run/docker.sock`.
- If the Docker socket is unavailable, the root endpoint returns an
  error response with a 500 status.
