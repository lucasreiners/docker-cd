# docker-cd Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-02-15

## Active Technologies
- Go 1.22 + Gin (`github.com/gin-gonic/gin`), Go standard (001-skeleton-runtime)
- Go 1.25.x + Gin v1.11.0, go-git v5 (read-only validation) (002-git-repo-config)
- Go 1.25.7 + Gin v1.11.0, go-git v5.16.5 (003-desired-state-cache)
- In-memory cache (no persistence) (003-desired-state-cache)
- Go 1.25.7 + gin (HTTP), go-git (Git access) (004-stack-sync)
- Docker container labels for sync metadata; in-memory desired-state store (004-stack-sync)
- Go 1.25.7 (backend), Node.js 20 LTS (frontend build/runtime) + Gin (backend), Vue 3 + Vite + Naive UI + Pinia + Vue Router (frontend) (005-web-frontend)
- N/A (in-memory state in backend store and browser state) (005-web-frontend)

- Go 1.22 + Go standard library (`net/http`, `os/exec`) (001-skeleton-runtime)

## Project Structure

```text
src/
tests/
```

## Commands

# Add commands for Go 1.22

## Code Style

Go 1.22: Follow standard conventions

## Recent Changes
- 005-web-frontend: Added Go 1.25.7 (backend), Node.js 20 LTS (frontend build/runtime) + Gin (backend), Vue 3 + Vite + Naive UI + Pinia + Vue Router (frontend)
- 004-stack-sync: Added Go 1.25.7 + gin (HTTP), go-git (Git access)
- 003-desired-state-cache: Added Go 1.25.7 + Gin v1.11.0, go-git v5.16.5


<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
