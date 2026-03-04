# Deployment Contract: Local Clone Volume

**Feature**: 008-local-git-clone
**Date**: 2026-03-04

## Overview

The service requires a persistent volume mounted at a fixed path inside the container (default `/repo`). Both local and production compose files must provide this volume.

## Docker Compose Requirements

### Volume

- Name: `repo-data` (or equivalent)
- Scope: persistent

### Mount

- Container path: `/repo`
- Access: read-write for the service

### Example (local)

```yaml
services:
  docker-cd:
    volumes:
      - repo-data:/repo

volumes:
  repo-data:
```

### Example (production)

```yaml
services:
  docker-cd:
    volumes:
      - repo-data:/repo

volumes:
  repo-data:
```
