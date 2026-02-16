# Quickstart: Stack Sync and Reconciliation

## Configuration

Add the following environment variables to enable reconciliation:

- `RECONCILE_ENABLED` (default: `true`) — enable/disable the reconciliation loop
- `RECONCILE_REMOVE_ENABLED` (default: `false`) — allow removal of stacks deleted from desired state
- `DRIFT_POLICY` (default: `revert`) — drift handling strategy:
  - `revert` — automatically apply desired state when drift is detected
  - `flag` — mark drift but wait for operator acknowledgement before applying

Reconciliation runs one stack at a time and uses container labels to store sync metadata.

## Run

```bash
export GIT_REPO_URL="https://github.com/your-org/your-repo.git"
export GIT_ACCESS_TOKEN="<token>"
export GIT_REVISION="main"

export RECONCILE_ENABLED="true"
export RECONCILE_REMOVE_ENABLED="false"
export DRIFT_POLICY="revert"

docker compose -f docker/docker-compose.yml up --build
```

## Verify

1. Trigger a refresh (webhook or manual `POST /api/refresh`) to fetch desired state.
2. Check `GET /api/stacks` to see sync status and metadata:
   - `syncedRevision` — last synced Git revision
   - `syncedComposeHash` — hash of applied compose file
   - `syncedCommitMessage` — commit message of synced revision
   - `lastSyncAt` — timestamp of last successful sync
   - `status` — one of: `missing`, `syncing`, `synced`, `failed`
   - `syncError` — error details if status is `failed`

## Drift Policy: Flag Mode

When `DRIFT_POLICY=flag`, drifted stacks are flagged but not automatically reconciled. To acknowledge and apply:

```bash
curl -X POST http://localhost:8080/api/reconcile/ack \
  -H "Content-Type: application/json" \
  -d '{"stack_path": "path/to/stack"}'
```

This acknowledges the drift and triggers an immediate reconciliation for that stack.
