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
| `DATA_DIR` | no | `/data` | Persistent operation-history directory; mount a writable volume here |

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

The frontend uses TypeScript 6.0.3 for compatibility with `vue-tsc` 3.3.13.
TypeScript 7 removes the `typescript/lib/tsc` entry point that this version of
`vue-tsc` requires; upgrade both together once the Vue checker supports it.

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
`bunx playwright show-report playwright-report/gitops` or
`bunx playwright show-trace <trace.zip>`.
The shared Verify workflow runs both new suites on pull requests and releases.

### Administration console

The Vue administration console opens on **Stacks**, with **Operations,
Repository, and System** available for investigation. Stacks shows nested runtime
containers, two previews per stack, bounded expansion, and filters/search across
stacks, containers and image identities. Instance-wide actions stay independent of
inventory filters. A compact work section and inline stack progress show running
checks, image pulls and deployments; the header Work control remains available
when navigating elsewhere. Parent and child operations share a workflow rather
than appearing as duplicate jobs.

Runtime observations refresh automatically while the page is visible, on return
and reconnect, and after relevant work. Polling is every 15 seconds, but backend
caching and observation cost affect the actual observation interval. The UI shows
observation age and retains dated data on failures; a live connection does not
prove current runtime health. Layer download/extraction progress is phase-local,
not a percentage of the whole deployment.

Container rows prefer repository digests over container IDs. Identity is inspected
from the exact running container's immutable image, not from its mutable tag.
Local builds and daemon metadata loss can make repository digests unavailable;
digest kind may be unknown. **Registry freshness is not checked** by this view.
Remote checks and outdated-image indicators are tracked separately in
[issue #44](https://github.com/lucasreiners/docker-cd/issues/44). Technical image and
container IDs remain in the inspector. An open inspector retains its dated image
selection and explains later identity changes or missing metadata.

The console retains command search (Cmd/Ctrl+K), nested stack deep links, tabbed
details, read-only Compose source and operation inspection. Theme and density
preferences are stored in the browser; Browser Back restores inventory context.

The Configuration tab compares the desired Compose file with the source captured
at the last successful deployment. Applied source snapshots are stored alongside
history in `${DATA_DIR}/applied-sources.json`; an existing stack gets its first
snapshot on its next successful deployment. The Logs tab follows stdout/stderr
for containers belonging to the selected stack, supports pause/filter/download,
and bounds each stream to 5 minutes or 1 MiB (200 KB retained in the browser).
Repository diagnostics retain webhook acceptance/signature failures and link
accepted deliveries to the repository checks they trigger.

Operation summaries are persisted in `${DATA_DIR}/operations.json`. Both Compose
files mount `operation-data:/data`. The agent retains up to 1000 completed
operations plus active work, with up to 100 significant steps per operation.
Unfinished operations become **Interrupted** when the agent restarts. Image-layer
byte progress remains live state. Keep the data volume to retain history across
container replacement. Storage failures are reported in the console.

Stack actions apply the reviewed Git revision, enforce refresh readiness and
reconciliation policy, and run independently of the HTTP connection. Pending drift
requires reviewing the source comparison before approval of that exact revision.
Per-stack image updates apply only that stack and do not prune images or force
recreation of unchanged containers. **Update all stacks** includes pruning unused
images across the Docker endpoint, including images outside managed stacks; the
confirmation explicitly discloses this scope. **Check Git for changes** can
interrupt active work and then apply configured reconciliation/removal policy—it
is not a read-only runtime refresh. No action is automatically replayed after an
uncertain network response.

Agent configuration is read-only in System; edit the deployment environment to
change it. Operations reports the effective scheduler timezone rather than
assuming browser time. Console APIs must remain behind an authenticated reverse
proxy or a restricted network; UI capability flags are not authorization.

The UI uses MIT-licensed **PrimeVue 4.5.5** and **@primeuix/themes 2.0.3**, pinned
because the newer major releases use a different licensing model.

### Headed admin-console walkthrough

For a long-lived environment you can explore manually, with its own Docker-in-Docker
daemon and a local Gitea repository/editor, run:

```bash
python3 demo/manage.py start
```

Open http://localhost:18881 for Docker-CD and http://localhost:13000/demo/homelab
for Git. Demo login, scenarios, and stop/reset commands are in [demo/README.md](demo/README.md).

To explore the frontend in a **headed Chromium browser**, run from `tests/e2e/`
after installing dependencies in both `frontend/` and `tests/e2e/`:

```bash
bunx playwright install chromium
bun run test:frontend:headed
```

This suite starts Vite on `127.0.0.1:18083` and a test-only API/SSE fixture on
`127.0.0.1:18082`. It needs no Docker daemon or credentials. Its realistic sample
data includes pending drift approval, a failed deployment, an unhealthy in-sync
stack, and retained operation records. The walkthrough exercises sidebar and
command navigation, filters/grouping, tabs, source, inspectors and focus recovery,
approval actions, live image progress, reconnect/reload, disabled features, themes,
mobile layouts, source diffs, logs, webhook diagnostics, and 250-stack pagination.
Screenshots are attached to its HTML report.

```bash
bun run test:frontend          # headless verification
bun run test:frontend:explore  # interactive Playwright UI with replayable steps
bunx playwright show-report playwright-report/frontend
```

The existing `bun run test` suite still verifies the full GitOps lifecycle
against real Docker-in-Docker containers.

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
