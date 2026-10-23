# Operational workspace — verification evidence

## Task 3: 250-stack real Compose observation measurement

Measured on 2026-10-23. This is performance evidence, not a migration/status
tracker or a substitute for running-container identity lifecycle verification.

### Reproduction

From the repository root:

```sh
(cd backend && go test -tags integration ./tests/integration/... \
  -run '^TestDinD_RuntimeObservation250Stacks$' -count=1 -v -timeout 4m)
```

Test: `backend/tests/integration/runtime_observation_test.go`.
The final measurement command exited 0; the test took 49.76 seconds, including
private-daemon startup, fixture discovery, the deliberate TTL wait, and cleanup.
No other integration tests were selected. There were no skipped tests.

### Isolation and workload

- Uses the existing `tests/integration/dind.StartT` harness. The final run used
  disposable DinD container `0db40beb5617`, which was stopped and terminated at
  the end. The outer Docker Desktop daemon was used only by the harness for its
  own test infrastructure; no operator application workload was inspected or
  mutated by this measurement.
- Docker client **29.3.1**, private Docker Engine **29.9.0**, Compose **5.1.1**;
  outer Docker Desktop reported 7,835 MB memory.
- Creates a local disposable Git repository with **250 unique nested stack
  paths**, each containing a valid `compose.yaml`, then discovers those files
  with the production `LocalComposeReader`.
- Uses the production `Reconciler.GetContainers`, `DockerComposeRunner.ComposePs`
  and `RuntimeReader`, with unchanged production budgets and cache settings.
  Commands are real host-side Docker/Compose subprocesses talking to the private
  daemon, not mocked command output or an in-container shortcut.
- The instrumented runner rejects anything except
  `docker -H <private-daemon> compose -p <project> ps -a --format json`.
  It records real command durations, failures, context timeouts and concurrency.
- **Actual application containers: 0 running, 0 stopped, 0 total**, verified by
  SDK listing before and after. The 250 valid service definitions were never
  pulled, created, or started. No observation initiated a registry lookup, image
  pull, prune, Compose up/down, or other application mutation.

### Measured results

| Read | Wall duration | Stack observations with successful listing | Compose calls | Command errors / timeouts | Observation errors / timeouts |
|---|---:|---:|---:|---:|---:|
| Cold runtime observation | 7.732779875 s | 250 / 250 | 250 | 0 / 0 | 0 / 0 |
| Immediate runtime cache hit | 540.708 µs | 250 / 250, original timestamps retained | 0 | 0 / 0 | 0 / 0 |
| Targeted detail cache hit | 188.875 µs | Existing cached empty container array | 0 | 0 / 0 | 0 / 0 |
| Runtime observation after production 15-second TTL | 7.712932125 s | 250 / 250, newer timestamps | 250 | 0 / 0 | 0 / 0 |

Maximum concurrent Compose subprocesses was **1** throughout. The two real
cycles issued **500 calls total**, all successful. Cumulative subprocess time was
15.426742080 seconds; the longest individual subprocess was 56.844 ms. Every
returned container array was empty, consistently with the independently verified
empty private daemon. Both cycles completed fully within the unchanged
**20-second cycle budget**; no timeout or stale/unknown result was counted as a
successful observation. No bounded-concurrency optimization was needed for this
measured empty-stack workload, and production execution scheduling was unchanged.

Cold listing timestamps spanned 7.695603 seconds; the oldest listing was
7.696725 seconds old when the cold response returned. After TTL expiry, the
listing window was 7.653921 seconds and the oldest listing age at response was
7.654332 seconds. Cache reads preserved the real listing timestamps rather than
presenting a new fetch time as observation time.

The test waits 15.1 seconds after cache publication before reading again. Measured
same-stack successful-observation intervals were **22.808969–22.864888 seconds**.
The backend's 15-second TTL begins at cycle publication: it is **not** a promise
of fixed 15-second start-to-start or same-stack observation cadence. Browser poll
alignment can add delay and was not measured here.

An earlier real run of the same measurement also observed all 250 stacks without
errors/timeouts: cold 7.943636708 seconds, immediate cache hit 309.208 µs, expired
cache cycle 7.832275250 seconds. The table above is the final, more fully
instrumented run.

### Limits of this evidence

- Establishes real subprocess/daemon cost for 250 discovered **empty** stacks on
  this machine. It does not establish capacity for 250 running stacks, image
  inspection costs, large child inventories, slower hosts, remote daemon latency,
  or concurrent deployment load.
- The empty-daemon measurement cannot demonstrate ownership verification or
  running-image identity after tag movement/recreation. Those retain their unit
  coverage and require the separate real running-container lifecycle tests.
- This is one environment with two measured real runs, not a statistical latency
  benchmark or universal production performance guarantee.
- The three previously recorded unrelated real-Docker baseline failures were not
  rerun, fixed, or counted as passing by this narrowly selected command. The full
   integration/browser migration verification was not run here.

## Baseline integration repairs (before full lifecycle migration)

The initial real-Docker suite failed three tests before backend implementation.
These were isolated and repaired without changing production reconciliation or
updater algorithms. This evidence does not replace the final full-suite run.

- `TestSmokeRootEndpoint`: the fixture passed a nil Docker SDK dependency to the
  root handler. It now constructs and cleanup-closes the SDK client. The Tester
  independently passed the read-only root container-count request.
- `TestDinD_DriftDetectionRevisionChange`: the old assertion assumed a repository
  revision change alone redeploys identical Compose content. The repaired test
  first proves a no-op and retained applied revision, then changes actual Compose
  content and proves exactly one successful deployment of the new revision.
  The Tester independently passed both assertions against isolated DinD.
- `TestDinD_UpdateCycleRestartsContainers`: the old fixture expected forced
  recreation without arranging any image change. A new private in-DinD registry
  helper publishes two genuine images under one reference. The test proves
  unchanged image A retains container C1, then publishes remote B and restores
  the local tag to A without pushing it. The subsequent update pulls B and
  produces C2 running B, while source Compose bytes/hash and the configured image
  reference remain unchanged. The Tester passed this test in 42.73 seconds and
  `TestDinD_UpdateCycleUsesLocalClone` in 22.80 seconds; neither was skipped.

Reproduction of the targeted repairs:

```sh
(cd backend && go test -tags integration ./tests/integration/... \
  -run '^(TestSmokeRootEndpoint|TestDinD_DriftDetectionRevisionChange)$' \
  -count=1 -v -timeout 5m)
(cd backend && go test -tags integration ./tests/integration/... \
  -run '^TestDinD_UpdateCycle(UsesLocalClone|RestartsContainers)$' \
  -count=1 -v -timeout 5m)
```

All image publication, application containers, and update/prune operations in the
updater fixture target its disposable DinD daemon. Registry manifest digests and
Docker image IDs are independently read, not assumed equivalent. A containerd
image store may report equal values; that is not evidence that a classic Docker
configuration digest is interchangeable with a registry digest. Generated
effective Compose metadata can change during deployment; the unchanged-source
assertion deliberately does not claim effective configuration bytes are identical.

## Task 8: deterministic browser journeys, resilience and accessibility

Implemented against the task-7 resource-first frontend on 2026-10-23. These are
isolated fixture-browser results, **not real-Docker lifecycle acceptance** and
not user-validated usability research. The original five browser journeys remain
in `tests/e2e/specs/frontend.spec.ts`; additional coverage is isolated in
`tests/e2e/specs/frontend-workspace.spec.ts`.

### Fixture and journey coverage

- The opt-in `workspaceBrowserFixture()` advertises v2 capabilities and supplies
  an authoritative epoch/sequence snapshot. A completed refresh parent with an
  active deployment child and an independent updater share the exact image
  reference and layer ID, but have independent root/run/pull-attempt identities.
  The active ancestor is not in the retained latest-50 history fixture.
- **A — inspect/act at home:** real initial children, inline scope confirmation,
  cancel-without-POST, exact stack/revision POST, no prune on targeted action,
  returned operation identity, accepted/waiting rather than fabricated execution,
  duplicate blocking, and inline progress without a document reload.
- **B — maintenance:** inventory filters do not hide global work or change global
  action scope; explicit outside-view links, named interruption impact, mandatory
  host-wide-prune confirmation and dismiss-without-POST remain enforced. A real
  browser transport abort after the fixture accepted the POST produces an
  uncertain outcome and exactly one server-side action, not automatic replay.
- **C — Git/approval:** the original successful source-review/approval journey is
  retained. New cases verify both an observed revision change disabling the open
  review before submission and a server-side revision race returning HTTP 409
  after review; neither silently sends the newer revision or claims success.
- **D — identity:** full 64-hex repository-reference clipboard read-back,
  unknown digest kind/platform/provenance, fixed “Not checked” freshness,
  ambiguous and local-build/unavailable identities, explicit replacement
  selection, and a removed container retaining its selected dated identity.
- **E — leave/reconnect:** Repository navigation and header work access,
  frozen/read-only reconnect state, disabled mutations, authoritative replacement
  on a new subscription, empty/new-epoch recovery and rejection of old-epoch
  bytes. Missing records remain “Outcome not confirmed,” not successful.
- Pull details are opened, not merely assumed present. Tests distinguish unknown
  totals from known downloading/extracting phase bytes, isolate identical layer
  identifiers across roots, retain open detail panels after terminal evidence,
  and keep pull completion separate from applying/pruning and partial results.
  Explicit cancelled/interrupted server outcomes are not inferred successes.
- Additional cases cover empty repository, no matches, failed deployment with
  healthy observed runtime, disabled update capability, stale dated observations,
  runtime HTTP 503 retaining children, history HTTP 503 while v2 work continues,
  expired operation HTTP 404, 23 workflows paginated 10/10/3, old nested `/stack`
  routes, legacy query normalization, full-digest search and stack/container-scoped
  log requests.
- Newly added cases collect uncaught errors, console errors, HTTP failures and
  transport failures. Intentional failures are declared per test and asserted in
  the UI; unrelated failures fail the case. Browser request-start/finish counters
  independently assert runtime requests never overlap, in addition to fixture
  server concurrency counters. The Vite fixture supplies the runtime config
  script that production injects at container startup instead of ignoring 404s.

### Minimal frontend defects found and fixed

1. A **completed** refresh root with a still-running deployment child incorrectly
   disabled Git checks as though the Git check itself were running.
   `frontend/src/composables/useOperationActions.ts` now requires a running
   refresh root (or running refresh operation). The interruption warning and
   actual duplicate-refresh guard remain intact. Regression coverage is in
   `frontend/tests/operation-actions.spec.ts` and the browser interruption case.
2. Keyboard Back context lacked a stable stack-link focus identity, and teardown
   could overwrite the route guard’s saved focus/scroll after navigation.
   `StackTreeRow.vue` now identifies its navigation link;
   `useInventoryView.ts` preserves the already-captured outgoing context when
   unmounting on a destination route. `frontend/tests/stack-tree.spec.ts` and the
   large-inventory browser case cover restoration of query/page/disclosure,
   keyboard focus, frozen order and scroll. No backend/action algorithm changed.

### Accessibility and visual scope / limitations

- Light and dark at 1440, 768, 390 and 320 CSS pixels, with long nested paths and
  full digest values, are exercised with reduced motion. Assertions cover body
  reflow, disclosure state/controls, keyboard Enter/Space, drawer Escape/focus
  recovery, Tab/Shift+Tab, command-search keyboard focus and actual computed
  no-animation/no-transition styles. Screenshots supplement these assertions.
- Chromium accessibility-tree exports are attached for all eight combinations.
  Inspection confirms named focusable disclosure buttons with expanded state and
  valid controls references, and status regions exposing `live=polite`,
  `atomic=true`, `relevant=additions text`. Twenty byte-only ticks leave all
  status-region text unchanged; phase changes remain separately announced.
- **No actual screen-reader pass was performed.** Superset CLI, Cua and Peekaboo
  executables were unavailable, and tool discovery exposed no screen-reader or
  native accessibility driver. No driver, browser or account was installed or
  attached. Chromium AX inspection and keyboard testing do not establish
  VoiceOver/NVDA speech order, verbosity or virtual-cursor quality. That manual
  assistive-technology acceptance remains a stated limitation, not a hidden pass.
- This is Chromium-only fixture coverage, not a cross-browser/mobile-device,
  contrast-certification or real registry/pull/recreation benchmark. No task-9
  test server, backend integration or GitOps spec was changed by this task.

### Final commands and measured results

All commands below exited 0; no browser cases were skipped or retried:

```sh
(cd frontend && bun run test && bun run build && bun run lint)
# 19 test files / 214 tests passed; vue-tsc + production build passed;
# Biome checked 70 files with no fixes/warnings.
(cd tests/e2e && bun run test:frontend)
# 28 passed (58.5s): original five plus 23 new cases.
(cd tests/e2e && bun run test:frontend --headed)
# 28 passed (57.3s), installed Chromium, same fixture suite.
git diff --check
```

The existing non-module `/config.js` production-build warning remains nonfatal;
the build completed. No dependencies, accounts, drivers or remote browsers were
installed. Final run logs are in the approved local tool directory
`/private/var/folders/fz/5vqzxnyj4p12pwnx72cl19c40000gn/T/opencode/`:
`task8-frontend-accepted.log`, `task8-complete-headless.log` and
`task8-complete-headed.log`.

Numbers below are from the final headed run, with real browser/HTTP timing (not
fake timers). They are single-sample, warm Vite fixture measurements, **not
production SLAs or Docker observation throughput**. Navigation-to-DOM and
event-to-DOM timings include Playwright assertion/control-request overhead.

| Measurement | Observed result |
|---|---|
| Stress dataset | 250 stacks, 748 observed containers overall; one stack has 250 children |
| Initial rendered inventory | 20 stack rows / 40 actual container rows; 1,327 total DOM elements |
| Navigation through initial row assertions | 516 ms |
| Explicit child expansion | 20, then 40 children; never an unbounded 250-child mount |
| Health update while a child control is focused | Visible second child becomes unhealthy; frozen order and focused first child remain |
| Browser Back | Restores `q=bench`, `deployment=synced`, directory grouping, page 2, collapsed service-030, stack-link focus and scroll Y=2,950 px |
| Stress journey runtime requests | 3 GETs; browser maximum in flight = 1; fixture server maximum in flight = 1 |
| Controlled byte upsert → asserted rendered bytes | 7 ms |
| Search during twenty additional progress updates | 322 ms, including 200 ms debounce; input value/focus and filtered row asserted |
| Leave/reconnect/restart journey runtime requests | 10 GETs across navigation/recovery; browser/server maximum in flight = 1 |
| Real visible polling/barrier case | 2 serialized bootstrap/recovery GETs, then one periodic GET; 3 total at capture |
| Runtime request start intervals in that case | 506 ms, then 14,499 ms; first-to-periodic start = 15,005 ms |
| Concurrent runtime calls during 500 ms responses and twelve stack events | Browser/server maximum in flight = 1 |

### Evidence locations and image review

The local HTML report is `tests/e2e/playwright-report/frontend/index.html`. It
contains assertions, screenshots and JSON attachments for all 28 cases. Generated
artifacts are intentionally not source snapshots; another run regenerates them.
Trace retention is configured for failures; the final passing run has no failure
trace to claim as evidence.

Under `tests/e2e/test-results/frontend/`, key screenshot paths are:

- `frontend-workspace-A-E-aut-37ecd-lying-reconnect-and-restart/v2-independent-applying.png`
- `frontend-workspace-B-globa-1a9fd-uncertain-POST-are-explicit/filtered-global-uncertain.png`
- `frontend-workspace-D-diges-872bc-dentity-survive-replacement/retained-selected-digest.png`
- `frontend-workspace-runtime-a5f59-does-not-suppress-live-work/history-unavailable-live-work.png`
- `frontend-workspace-pull-te-4ced8-eld-details-remain-separate/partial-result-held-pull-details.png`
- `frontend-workspace-250-sta-22f9d-ntext-coalesce-observations/250-resource-child-search.png`
- Eight matrix images: `frontend-workspace-accessi-*/light-{1440,768,390,320}.png`
  and `frontend-workspace-accessi-*/dark-{1440,768,390,320}.png`.

The matrix images were read directly for both themes at all four widths; journey,
retained-identity, history-failure and expanded-inventory images were also read.
Long paths/references wrap, focused child controls have a visible focus outline,
mobile controls remain reachable, and work remains visibly separate from runtime
and deployment outcomes. No external account/customer data appears in these
synthetic captures. The inventory screenshot shows **user-expanded/restored**
state; bounded initial rendering is proved by the earlier DOM assertions, not
inferred from that tall screenshot.

Raw measurement files in those same case directories are
`event-render-measurement.json`, `inventory-measurements.json` and
`real-polling-measurements.json`. Each matrix directory also contains
`accessibility-tree.json`; these are genuine Chromium AX exports, not a claimed
screen-reader transcript. Task 9 real Docker acceptance and the manual
assistive-technology limitation above remain separate.

## Task 9 independent verification — 2026-10-23

### Commands and results

Executed sequentially; the DinD-heavy commands did not overlap. Both use disposable
Testcontainers DinD instances; no operator workload was targeted.

1. `(cd backend && go test -tags integration ./tests/integration/... -timeout 30m)`
   exited **0**, but Go reported the primary integration package as **cached**. This
   result is recorded but is not treated as fresh real-Docker evidence.
2. `(cd tests/e2e && bun run test)` exited **0**: **1 passed (1.1m)**, with the
   GitOps browser case itself completing in **41.9s**. The testserver created and
   terminated disposable DinD `02c487282e81`; no browser tests were skipped or
   retried.
3. A subsequent sequential uncached confirmation,
   `(cd backend && go test -count=1 -tags integration ./tests/integration/... -timeout 30m)`,
   exited **1** after **608.655s**. It reached real disposable DinD execution, but
   Docker Hub returned unauthenticated pull-rate-limit errors (`429 Too Many
   Requests`) for `nginx:alpine`, `busybox:1.38.0`, and `registry:2`. This is an
   external registry-prerequisite failure, not a skipped test or a claim that the
   backend integration suite passed.

The integration tree contains 39 `Test*` functions. Source inspection found only
conditional prerequisite/short-mode skips (`Docker socket/CLI unavailable`,
`SKIP_DIND_TESTS=1`, and `testing.Short()`); none were activated in the recorded
runs. The real browser spec contains no Playwright skip/fixme controls.

### Real-browser evidence inspected

The passing case uses the production HTTP/SSE API and an actual private DinD
lifecycle. Its EventSource wrapper records emitted `work.snapshot`/`work.upsert`
events only; it does not mock HTTP/SSE responses or UI state. The isolated
slow-pull fixture injects controlled Engine NDJSON layer bytes before an actual
private-DinD pull, so byte values are parser/progress evidence, **not measured
registry-download bytes**.

Recorded attachments from this run contain **185** v2 events (**6** snapshots,
**179** upserts), **22** unique runs with final statuses **18 completed, 3 failed,
1 cancelled**, and browser mutation paths only `/api/operations`, `/api/refresh`,
and `/api/update`.

Assertions exercised: nested inline initial pull; same-document SPA navigation
away/back; reconnect snapshot replacement retaining the same run/attempt/layers;
pull completion remaining non-terminal while Compose is gated; actual full
repository-digest copy; signed-webhook rejection; same-count replacement in two
browser tabs; real invalid-Compose failure and recovery; cancelled targeted work;
host-wide-prune confirmation with no POST on cancel; failed-stack skip and
terminal result counts; and removal yielding scoped-container `404`. The backend
identity test also explicitly allows Docker 29/containerd disappearance of old
A `RepoDigests` only as honest `unavailable/not_recorded`, forbids substituting
B, and uses full immutable IDs in its daemon assertions.

### Limitations / blocker

Fresh full backend integration acceptance is currently blocked by Docker Hub
rate limiting, so task-9's required backend command is not independently passing
in this environment. The browser result does not remove that blocker. Real
multi-platform identity creation remains unproven by this DinD evidence; its kind
is fixture/unit-covered only. Manual screen-reader validation is outside this
run.

## Task 9 retry — authenticated-host / isolated-DinD SDK preflight (2026-10-23)

Start SHA remains `efb7b1b4e87887ae7852a371ae008e8962df350f`; no application source, plan, or test code was modified for this retry. The host Docker daemon was reachable (`desktop-linux`, Server `29.3.1`). A normal host Docker CLI pull of `hello-world:latest` exited 0; no credential/configuration values were read or emitted.

Before repeating the full fixture-heavy integration tree, an isolated disposable `docker:29.9.0-dind` daemon was started and removed. A one-off Go SDK client, matching the production call shape, invoked `ImagePull("nginx:alpine", image.PullOptions{})` against that private daemon. It exited 1 with:

```text
SDK_PULL_ERROR: Error response from daemon: unknown: failed to resolve reference "docker.io/library/nginx:alpine": unexpected status from HEAD request to https://registry-1.docker.io/v2/library/nginx/manifests/alpine: 429 Too Many Requests
```

The expected client-auth propagation is absent on this execution path: `backend/internal/docker/images.go:68` calls `c.API.ImagePull(ctx, img, image.PullOptions{})`; `backend/tests/integration/harness/harness.go:97-104` creates an SDK client directly to the disposable DinD daemon and warms it through that call. `backend/tests/integration/harness/slow_pull.go:47-51,94` forwards the same options. Docker CLI authentication to the outer host is therefore not forwarded to this isolated daemon/SKD request. No full uncached backend suite was started, avoiding another fixture-wide anonymous pull burst. Consequently `(cd tests/e2e && bun run test)` was also not rerun: the required sequential backend prerequisite did not pass. No test skips were activated or counted as passing; full integration: not run (0/39 tests); real-browser retry: not run (0 cases).

This is a reproducible registry-auth prerequisite gap, not evidence of a product lifecycle assertion failure. A small **test-harness credential-forwarding fix is needed** for authenticated public-fixture pulls (using the normal Docker credential-helper/client-auth mechanism without logging or persisting secrets). Because the production Docker client uses the same empty `image.PullOptions{}` path, the owner should also decide whether authenticated registry pulls are a supported production capability; the harness-only fix would not change that product-path behavior.

## Task 9: authenticated private-DinD SDK regression verification

Measured on 2026-10-23 after the Docker Hub CLI-login retry and authenticated
DinD SDK harness change. This entry is verification evidence only.

- Focused wrapper race run: `(cd backend && go test -race -count=1
  ./tests/integration/dind -run '^(TestAuthenticatedPull.*|TestSDKRequiresExplicitHost)$' -v)`
  passed in **2s**. It executed the six wrapper tests (13 including the eight
  registry-routing subcases): Docker Hub normalization/routing, per-registry
  credential isolation, anonymous loopback fixtures, explicit options and
  privilege-function bypass, helper-error sanitization, identity-token
  forwarding, and explicit private-host enforcement. No race report occurred.
- Fresh tagged integration run, in the requested order:
  `(cd backend && go test -count=1 -tags integration ./tests/integration/... -timeout 30m)`
  passed in **741s** (integration package **739.313s**, `dind` package
  **19.635s**). The run was not cached and had no skipped package/test result.
  It covered **39** named integration tests and **7** tagged `dind` tests,
  including the existing real assertions for mutable/local running-image
  identity, slow-pull lifecycle, all-stack HTTP work stages, baseline repairs,
  reconciliation, update/restart behavior, and the authenticated Docker Hub
  preflight/private-DinD image inspection.
- Fresh browser run executed only after the tagged integration command:
  `(cd tests/e2e && bun run test)` passed in **68s** — Playwright reported
  **1 passed (1.1m)**. The real-DinD operational-workspace scenario completed
  and cleaned up its disposable private daemon.

No Docker credential configuration, authentication values, or unsanitized
credential-helper output was emitted in this evidence. Verification used only
disposable test infrastructure; no host application workload was mutated.

## Task 10: transitional UI retirement and compact global work (2026-10-23)

### Scope and compatibility audit

Removed `frontend/src/pages/OverviewPage.vue` and
`frontend/src/components/StackCard.vue`. Production import searches find no
references to either component. The existing operational home, root redirect,
nested stack route, legacy `/stack` redirect and old grid/group/status query
normalization remain unchanged. Removed their unused overview metrics/attention
layout, fixed footer/work bar, old inventory table/grid/layout-picker selectors
and corresponding responsive rules from `admin.css`; retained shared panel,
definition-list, saved-view/table/cell, source, port, tag, schedule and current
inventory styles. The sidebar still uses `--shell-footer-height`.

`store/stacks.ts` no longer owns singleton pull/image/layer progress types, refs,
completion timer or accumulator. The now-unconsumed Overview activity list and
its presentation-only `ActivityEntry` type are also removed. `UpdateProgress`
wire fields and `services/sse.ts` legacy `update.*` parsing are retained, as are
legacy `isUpdating`, stack/refresh bootstrap race protection, operation forwarding
and runtime refresh lifecycle handling. Keyed work, legacy operation-stage
fallback (`store/work.ts` / `useWorkflowGroups`) and capability/readiness gates
are not removed or weakened. Source search finds no production `pullProgress`,
`updateProgress`, `progressTimer`, `ActivityEntry` or retired progress-type use.

### Compact behavior and directly inspected visual evidence

Global work now shows workflow kind/target once and a phase-bearing **Pull
details** disclosure for each run. The repeated workflow/run message is inside
that existing native disclosure instead of appearing above it. Phase-only live
status remains accessible (visually hidden in global compact mode); bytes are
not announced on every tick. Failure messages and processed/skipped/failed
counts stay outside the disclosure. `DeploymentProgress`'s optional `compact`
prop is used **only** by `ActiveWork`: per-stack inline progress is unchanged.
Idle work is one quiet line; a retained last result remains readable with result
access. Neither errors nor results are ellipsized or cleared by the compact
layout. Existing pinned details, terminal/reconnect/epoch retention, off-filter
links, two-desktop/one-mobile root previews and ten-root all-work pagination
remain covered.

Before **and** after PNGs were opened and inspected directly at 1440 and 390
pixels, including expanded details. Persistent copies are in
`.lead/_adhoc/task10-visual/{before,after}/` (`compact-active-WIDTH.png`,
`compact-idle-WIDTH.png`, and after-only `compact-expanded-WIDTH.png`). JSON
measurements are `desktop.json` / `mobile.json` in each directory. The baseline
desktop fixture includes two independent roots and a last-result sentence; this
measured baseline is taller than the earlier approximate lead review.

| Closed global region | Before | After | Reduction | Regression ceiling |
|---|---:|---:|---:|---:|
| 1440px, two roots + last result | 515.13px | 228.38px | 56% | 260px |
| 390px, one-root preview of two + last result | 319.75px | 234.59px | 27% | 270px |
| 1440px idle, no result | 76.83px | 28.80px | 63% | 40px |
| 390px idle, no result | 68.83px | 28.80px | 58% | 40px |

Ceilings allow roughly 30px of font/rounding headroom over actual active
measurements rather than asserting exact pixels. Mobile deliberately keeps
44px disclosure/buttons and wraps the last-result sentence instead of clipping
it. Opening one global pull detail increases height to **385.53px desktop** /
**485.03px mobile**; expanded detail has no compact-height cap. Browser tests
assert this increase, keyboard disclosure, visible bytes, two/one visible roots,
and the mobile 44px disclosure minimum. These bounds apply to the default closed
fixture, not arbitrary-length errors, restored warnings, pinned/open panels or
all-work pages, which may grow rather than hide information.

### Retained and migrated regression coverage

- `stack-card-ports.spec.ts` is replaced by
  `frontend/tests/container-row-ports.spec.ts`, exercising the actual
  `StackTree`/`ContainerRow` against the shared console runtime cache: lowest
  published port, native external link, `_blank`, `noopener noreferrer`,
  internal-only port text, no-port container, and failed observation remaining
  unknown with its underlying cache error preserved.
- `stacks-grid.spec.ts` now renders the tree for paths, **actual runtime**
  counts versus stale deployment counts, and distinct truncated desired/applied
  revisions. Its StatusBadge and store-filter tests remain. Former card commit
  message/error coverage moved to real `StackDetail` tests, now asserting those
  values rather than only the path. The live detail port-pill tests remain.
- `sync-lifecycle.spec.ts` replaces singleton assertions with correlated
  reconcile applying, independent same-image runs, independent layer completion,
  and pull-complete-not-workflow-complete assertions through actual SSE. It
  retains HTTP/SSE bootstrap races, legacy snapshot missed-completion recovery,
  cancellation, stack-success-not-cycle-completion, reconcile completion not
  ending a scheduler cycle, source/operation forwarding and runtime invalidation.
- Added compact-mode/verbose-disclosure and persistent global failure-result
  unit regressions. Existing open-panel terminal/epoch tests remain. Added two
  geometry browser cases without removing any of the original 28 journeys.

### Commands and results

- `(cd frontend && bun run test && bun run build && bun run lint)`:
  **214/214 tests, 19 files; TypeScript/build passed; lint clean, no warnings**.
  Build retains the pre-existing Vite notice that runtime `/config.js` is not a
  module to bundle; build exits 0.
- Final `(cd tests/e2e && bun run test:frontend)`: **30/30 passed in 59.5s**,
  no retries/skips. Includes all original 28 fixture journeys, two geometry
  cases, both themes/320–1440 reflow, exact digest copy, action safety, paging,
  root counts, byte-phase semantics, stale runtime and focus/AX/live-region
  assertions. Every case retains the browser max-one-runtime-request gate.
- Then `(cd tests/e2e && bun run test)`: **1/1 real-DinD browser case passed in
  44.9s, suite 1.2m**, no retries/skips. Private daemon `8d5cde4db087` was stopped
  and removed. Original real lifecycle/identity/safe-action assertions required
  no changes for compact DOM. Existing evidence remains under
  `tests/e2e/test-results/gitops/gitops-operational-workspa-77d6b-servations-and-safe-actions/`.
- `git diff --check` passed. No backend or CI files were edited by task 10; the
  full tagged/backend race suites are intentionally left to final independent
  Tester rather than repeating task 9's 12-minute run in this cleanup session.

During development the new geometry test initially reloaded before the old
document's runtime request finished; waiting for completion before reload keeps
the unchanged concurrency assertion meaningful. A later complete fixture run
also exposed a pre-existing byte-announcement sampling race: runtime bootstrap
mounted empty image-copy live regions between samples. The test now waits for
all expected preview children before sampling; it still compares **every** live
region exactly, keeps focus assertions and runs the same byte burst. The final
full fixture and real-browser commands above passed after that correction.

This is developer visual inspection plus automated browser/AX/keyboard evidence,
not user usability validation or native screen-reader speech validation. Native
screen-reader validation remains unavailable. Existing real-Docker simulated
byte timing/multiarchitecture limitations remain as documented in task 9.
Remote registry freshness remains the separate GitHub #44 follow-up. No
production mutation semantics, credential behavior, deployment access boundary,
CI gates, plan Progress/checklists, or commits were changed by task 10. Final
independent Tester/Reviewer and Tech Lead release acceptance remain required.

## Final independent Tester gate — Task 10 cleanup (2026-10-23)

Executed fresh after the Task-10 retirement/compact-work cleanup. The full tagged
Docker integration command completed before the real-DinD browser command; the two
heavy DinD commands did not overlap. No test command reported a skipped or cached
PASS result.

| Gate | Actual command | Result |
|---|---|---|
| Frontend unit/type/build/lint | `(cd frontend && bun run test && bun run build && bun run lint)` | **PASS** — 19 files / **214 tests** in 1.32s; `vue-tsc --noEmit`, production build, and Biome (68 files) exited 0. The known nonfatal Vite `/config.js` non-module notice remained. |
| Backend unit/build/scoped race | `(cd backend && go test -count=1 ./... && go build ./... && go test -count=1 -race ./internal/desiredstate/... ./internal/operations/... ./internal/http/... ./internal/reconcile/... ./internal/scheduler/... ./internal/refresh/...)` | **PASS** — uncached unit packages passed; build exited 0; all six requested race-package groups passed without a race report. |
| Fixture browser | `(cd tests/e2e && bun run test:frontend)` | **PASS** — **30/30** Chromium cases in **1.0m**, one worker, no retries/skips. This includes the two compact-work geometry cases, eight theme/viewport keyboard/AX cases (320–1440), focus/Back recovery, live-region byte-tick behavior, and max-one-runtime-request assertions. |
| Tagged real-Docker integration | `(cd backend && go test -count=1 -tags integration ./tests/integration/... -timeout 30m)` | **PASS** — primary integration package **736.688s** and tagged `dind` package **20.473s**; harness/testserver have no test files. The command was uncached and returned 0, including the authenticated private-DinD registry path. |
| Real GitOps browser | `(cd tests/e2e && bun run test)` | **PASS** — **1/1** in **1.1m**; scenario **43.3s**, no retries/skips. It used an isolated disposable DinD daemon and completed cleanup. Controlled slow-pull NDJSON bytes remain parser/progress fixture values, not claimed registry-download measurements. |

Cleanup/coverage checks: `git diff --check` passed. Source/import inspection found no
production imports of the retired `OverviewPage` or `StackCard`; their port coverage
is present on the live `StackTree`/`ContainerRow`, and the migrated detail, source,
revision, operation/history, error and legacy-SSE fallback coverage ran in the
frontend suite. Legacy `isUpdating`/`UpdateProgress` parsing remains as a lifecycle
compatibility path; no singleton pull/layer accumulator remains in `stacks.ts`.
The existing focus/reload flake did not recur in this 30-case complete fixture run.

Visual artifact inspection: after screenshots exist at
`.lead/_adhoc/task10-visual/after/compact-{active,idle}-1440.png` (the idle file is
`compact-idle-1440.png`), corresponding 390px captures, and expanded captures at
both widths. Measured closed global-work heights are **228.375px** active / **28.797px**
idle at 1440 and **234.594px** active / **28.797px** idle at 390; expanded details
are **385.531px** and **485.031px** respectively. These satisfy the recorded closed
region ceilings (260px desktop; 270px mobile; 40px idle). Native screen-reader speech
was not available; the PASS is limited to the fixture keyboard and Chromium AX/live-
region checks already described above.

README review: the Administration-console section accurately documents the
Stacks-first workspace, compact work, mutable-tag/immutable-image identity boundary,
no registry-freshness claim, action scope, and reverse-proxy/restricted-network access
boundary. The integration commands retain `DOCKER_AUTH_CONFIG='{}'` for
Testcontainers public-image setup; the test-only authenticated private-DinD pull
client separately resolves normal Docker CLI configuration through `dockercfg`.
Source inspection found that resolver routes credentials only to the normalized
non-loopback registry, skips helpers for loopback fixtures, and returns sanitized
errors. No credential values were read, logged, or written by this verification.
