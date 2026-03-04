# API Contract: Refresh Status and Blocking

**Feature**: 008-local-git-clone
**Date**: 2026-03-04

## Overview

This feature extends the refresh status reporting to surface refresh failures and whether reconcile/update operations are blocked.

## REST: GET /api/refresh-status

**Response**: `RefreshSnapshot`

### New Fields

- `updatesBlocked` (boolean, required): Indicates reconcile/update tasks are blocked.
- `blockedReason` (string, optional): Human-readable reason for blocking.
- `localPath` (string, optional): Fixed local clone path for diagnostics (e.g., `/repo`).

### Example

```json
{
  "revision": "abc123",
  "commitMessage": "deploy v1",
  "ref": "main",
  "refType": "branch",
  "refreshedAt": "2026-03-04T08:00:00Z",
  "refreshStatus": "failed",
  "refreshError": "fetch failed: unauthorized",
  "updatesBlocked": true,
  "blockedReason": "refresh failed",
  "localPath": "/repo"
}
```

## SSE: refresh status events

**Event name**: `refresh.status`

**Payload**: Same shape as `RefreshSnapshot`.

### Example

```
event: refresh.status
id: 74
data: {"refreshStatus":"failed","refreshError":"fetch failed","updatesBlocked":true,"blockedReason":"refresh failed","localPath":"/repo"}
```
