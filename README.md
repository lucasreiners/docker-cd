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
| `RECONCILE_ENABLED` | no | `true` | Enable/disable stack reconciliation |
| `RECONCILE_REMOVE_ENABLED` | no | `false` | Allow removal of stacks deleted from desired state |
| `DRIFT_POLICY` | no | `revert` | Drift handling: `revert` (auto-fix) or `flag` (require ack) |
| `UPDATER_ENABLED` | no | `false` | Enable/disable scheduled image updates |
| `UPDATER_CRON` | no | `0 3 * * *` | Cron expression for update schedule (default: 3 AM UTC daily) |

The service validates repository access on startup and exits immediately if any required variable is missing or credentials are invalid.

### Webhook Setup

Configure a GitHub webhook pointing to `https://your-host/api/webhook` with content type `application/json`. Set the same secret in both GitHub and the `WEBHOOK_SECRET` environment variable. If no secret is configured, all webhook requests are accepted without signature validation.

### Scheduled Updates

The updater feature automatically checks for new container images and updates running stacks. When enabled, it:

- Pulls latest images for all managed stacks
- Detects image changes by comparing digests
- Triggers reconciliation to recreate containers with new images
- Prunes unused images to reclaim disk space

To enable scheduled updates:

```bash
export UPDATER_ENABLED=true
export UPDATER_CRON="0 3 * * *"  # 3 AM UTC daily (default)
```

The updater is disabled by default. Update cycles run sequentially to avoid resource contention, and the service gracefully waits for active cycles to complete during shutdown.

Stack discovery recursively scans subdirectories of `GIT_DEPLOY_DIR`. Supported
filenames are `docker-compose.yml`, `docker-compose.yaml`, `compose.yaml`, and
`compose.yml`, in that precedence order (one stack per directory). Stack paths
remain relative to the deploy directory, including their nested parents.

## Run

```bash
export GIT_REPO_URL="https://github.com/your-org/your-repo.git"
export GIT_ACCESS_TOKEN="<token>"
export GIT_REVISION="main"

docker compose up --build
```

### Local Clone Volume

The service stores a local working copy of the repository at a fixed path inside
the container: `/repo`. Ensure your compose files mount a persistent volume at
that path so refresh, reconciliation, and updates can use the local clone.

If a refresh fails, the last successful clone is kept, but reconciliation and
updates are blocked until the next successful refresh. The UI surfaces this as
`updatesBlocked` in refresh status.

- Backend API: <http://localhost:8080>
- Frontend UI: <http://localhost:8081>

### Frontend development

```bash
cd frontend
bun install
bun run dev     # starts Vite dev server on :8081, proxies /api to :8080
```

## Tests

```bash
# Go unit tests
cd backend && go test ./... -v

# Frontend unit tests
cd frontend && bun run test

# Integration tests (requires a running Docker daemon)
cd backend && go test -tags integration ./tests/integration/... -v
```

### GitOps lifecycle integration tests

Prerequisites: Go matching `backend/go.mod`, Docker with privileged-container
support, Docker Compose v2+, and access to pull the public DinD/BusyBox images.
The test creates its own Git origin and clone in temporary directories and uses
a disposable Docker-in-Docker daemon. Compose, image pulls, containers, labels,
and volumes are real; GitHub push payloads and signatures are generated locally.
No GitHub token, application `.env`, or live deployment is used.

Run from `backend/`:

```bash
DOCKER_AUTH_CONFIG='{}' go test -race -tags integration ./tests/integration \
  -run 'Test(GitOps|SlowPull)Lifecycle' -count=1 -timeout=8m -v
```

Coverage includes nested discovery, initial sync, signed webhook verification,
changed-stack deployment, unchanged-stack preservation, duplicate deliveries,
runtime revision labels, failure/recovery, SSE transitions, and removal with
persistent-volume retention. `DOCKER_AUTH_CONFIG='{}'` tells Testcontainers to
use public-image authentication instead of invoking local credential helpers.

### Browser E2E tests

Install frontend dependencies in `frontend/` with `bun install --frozen-lockfile`.
Then run from `tests/e2e/`:

```bash
bun install --frozen-lockfile
bunx playwright install chromium
DOCKER_AUTH_CONFIG='{}' bun run test
```

Playwright starts Vite on `127.0.0.1:18081` and an integration-only Go test server
on `127.0.0.1:18080`, with its own DinD daemon. Both ports must be free. The test
uses real API/SSE responses and checks initial sync, live card container updates,
mid-sync reconnect, multi-tab updates, detail-page container replacement,
failure/recovery, manual image updates, and stack removal. A test-only gate pauses
before real Compose deployment so transient states can be asserted deterministically.
Test control routes are compiled only with the `integration` build tag and are
not exposed by the production application.

The lifecycle is one sequential browser scenario against a disposable fixture;
retries are disabled to avoid hiding failures behind already-mutated state.
Screenshots and traces are retained on failure. Inspect them with
`bunx playwright show-report` or `bunx playwright show-trace <trace.zip>`.
The shared Verify workflow runs both new suites on pull requests and releases.

#### Long image pulls and layer progress

The slow-pull fixture feeds controlled Docker Engine NDJSON through the real
SDK parser, reconciliation, SSE, and UI. It simulates two large layers (128/256
MiB), separate download/extraction phases, and an interrupted registry stream.
The layer bytes are simulated, not downloaded; successful fixtures then perform
an actual BusyBox pull and deployment in DinD. This tests slow-stream behavior
deterministically without depending on a public registry's speed.

Assertions cover independent layer byte counts, reconnect snapshots containing
both layers, whole-image versus layer completion, pull-error recovery, and
remaining `syncing` while periodic refreshes queue behind a long initial pull.
CI holds the stream quiet for 65 seconds. Local runs default to 5 seconds;
to watch the longer scenario in a visible browser, run from `tests/e2e/`:

```bash
DOCKER_AUTH_CONFIG='{}' E2E_PULL_HOLD_MS=65000 E2E_SLOW_MO=1000 \
  bun run test --headed
```
