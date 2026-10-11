# Admin console architecture study

Date: 2026-10-11

Status: Architecture study; implementation scope is documented in spec.md and plan.md.

Constraint: Vue.js remains the frontend framework.

## 1. Recommendation

Build a task-oriented administration console with a persistent sidebar, a compact
header, dedicated workspaces, and a global operation inspector. Use Vue 3, Vite,
TypeScript, Vue Router, Pinia, and **PrimeVue in styled mode with a custom preset**.

The product's organizing concept should be:

> Know what is running, understand why it changed, and resolve what needs attention.

The frontend is organized around stacks and their operations, with dedicated
workspaces for delivery, investigation and configuration. This study explains
the information architecture and the capabilities it supports.

The backend currently manages one configured repository and one Docker endpoint.
The console should show the instance identity prominently. Multiple hosts or
repositories would require a subsequent product/data-model expansion.

## 2. Study method and evidence

Reviewed the production router and handlers, configuration, desired-state store,
reconciliation/drift policy, scheduler, SSE broadcaster, current Vue branch, and
the existing feature specifications. Implementation takes precedence when a draft
specification and current behavior differ. UI-kit recommendations use current
official documentation; no migration benchmark has been performed.

Primary code references:

- `backend/internal/http/router.go` and `handler.go`: actual HTTP surface.
- `backend/internal/config/config.go`: instance configuration and feature switches.
- `backend/internal/desiredstate/state.go`: public stack/container fields.
- `backend/internal/desiredstate/events.go`: live events and reconnect snapshots.
- `backend/internal/reconcile/{reconcile,drift,ack,policy}.go`: deployment behavior.
- `backend/internal/scheduler/{scheduler,updater}.go`: image update cycles.
- `backend/internal/refresh/refresh.go`: refresh queue and interruption behavior.
- `backend/internal/docker/sdk.go`: available Docker client operations.
- `frontend/src/router/index.ts`, `services/api.ts`, and `store/stacks.ts`:
  current browser capabilities.
- `.specify/memory/constitution.md`: Git authority and configuration constraints.

## 3. Capability inventory

| Capability | Current implementation | Recommended UI home |
|---|---|---|
| Discover nested Compose stacks | Implemented; recursive paths and Compose filename precedence | Stacks; group/filter by path prefix |
| List deployments | `GET /api/stacks`; status, hashes, applied revision, last result | Stacks table and stack overview |
| Inspect containers | `GET /api/stacks/containers/*path`; service, image, state, health, ports | Stack Containers tab; container inspector |
| Repository refresh | `POST /api/refresh`; startup, webhook, and optional polling triggers | Repository page; global command search |
| Repository condition | `GET /api/refresh-status`; ref, commit, refresh time, blocked state | Repository page and compact overview signal |
| Continuous delivery | Successful refresh invokes reconciliation when configured | Operations; stack deployment state |
| Automatic removal | Policy-controlled removal of stacks absent from Git | System policy display; removal operation detail |
| Drift acknowledgement | `POST /api/reconcile/ack` exists; frontend does not expose it | Stack attention panel, after structured drift state is added |
| Image update cycle | `POST /api/update`; pulls images, compares digests, redeploys changes, prunes images | Operations / Image updates |
| Active update status | `GET /api/update/status` exists; frontend currently relies on SSE instead | Operations active-run summary |
| Scheduled image updates | Cron and enablement configured through environment | Operations schedule summary; System configuration |
| Per-image/layer progress | SSE carries download/extraction status and byte counts | Operation inspector, expandable per-image details |
| Live reconnect | SSE sends stack, refresh, and operation snapshots on connection | Persistent header connection state |
| Recent activity | Latest 50 received events in browser memory on current branch | Operations initially, labeled as session activity |
| Theme, search, layout preference | Light/dark and URL-backed list/grid/search/filter controls | Preferences and resource toolbar |
| Instance name and repository URL | Backend configuration; also present in backend text root response, not a dedicated frontend metadata API | Instance header; Repository/System through a new metadata endpoint |
| Durable run history | No historical query/persistence API; some last-sync metadata survives in container labels | New operation store and Operations history |
| Read Compose source or a diff | Content held internally, explicitly excluded from stack JSON | New read-only configuration/diff API and stack Configuration tab |
| Container logs | No application endpoint; current Docker interface does not include log streaming | New backend capability; container inspector/Logs tab |
| CPU, memory, restart counts | Not exposed by current container DTO | Future runtime telemetry, only when measured |
| Webhook delivery history | Webhook triggers refresh; no queryable delivery history | Repository / Webhook diagnostics, with new records |
| Per-stack retry/update action | Targeted reconciliation exists internally; no dedicated public endpoint | New policy-aware operation action |
| Application URLs | Current UI derives browser links from published ports | Explicit service URLs in a later metadata feature |

### Important behavior the UI must represent accurately

1. **Git checks and image updates are different operations.** A Git check refreshes
   the desired configuration and can lead to deployment. An image update pulls
   images and may change running containers without a new Git commit.
2. **A manual Git refresh can interrupt active work.** In `enqueueRefresh`,
   non-periodic triggers request cancellation of reconciliation/update work;
   periodic checks queue behind it. The action should describe that effect when
   work is active.
3. **Drift approval is currently encoded as failure text.** Under `flag` policy,
   the stack becomes `failed` with “drift detected, awaiting acknowledgement”.
   Add an explicit reason/action field before making approval a first-class UI
   state. The current acknowledgement handler invokes the reconciler and waits
   on the request; it is not yet an asynchronous, revision-bound operation API.
4. **The current drift detector compares recorded Compose hashes/metadata.** A
   hash difference is evidence of configuration divergence; it is not a
   field-by-field inspection of every Docker runtime setting.
5. **A connected SSE stream does not prove fresh runtime health.** Container
   details are fetched on inspection/stack updates; there is no independent
   health-event stream in the current frontend. Show observation time and add
   runtime sampling/events before promising immediate health-change detection.
6. **The image updater is optional.** The browser needs explicit capability and
   effective configuration information to distinguish disabled, blocked, busy,
   and available states. Endpoint presence or a generic 409 is insufficient.
7. **Update progress is not history.** SSE reconnect snapshots restore current
   state; they do not provide a durable, paginated event archive.
8. **Use the effective scheduler timezone.** Documentation describes 03:00 UTC,
   but the scheduler currently uses `cron.New()` without an explicit location.
   Expose the actual effective schedule/timezone and backend-computed next run.
9. **A published port is not necessarily an HTTP application.** Display port
   mappings as technical data; use explicitly configured application links when
   available instead of treating every database or UDP port as a website.

## 4. Operator workflows

| Question or task | Entry point | Useful outcome |
|---|---|---|
| Is the instance working? | Overview | See repository, deployment, runtime, and connectivity separately |
| Which service needs me? | Overview attention list or Stacks saved view | Open the relevant stack with its actual reason |
| What changed after that push? | Operations or stack Activity | Follow Git refresh to affected deployments and results |
| Why did deployment fail? | Stack Overview | Read operation error; inspect source/diff and container state |
| Why has a Git change not deployed? | Repository, then stack | Identify refresh failure, disabled reconciliation, or pending acknowledgement |
| Did last night's image update succeed? | Operations / Image updates | Read a persisted run summary and per-stack outcomes |
| Is an in-sync app healthy? | Stack Containers | Distinguish health checks, process state, and deployment state |
| How is this agent configured? | System | Read effective configuration, its source, and supported capabilities |

## 5. Navigation and screen ownership

```text
Docker / CD
home-server                         Instance identity

WORKSPACE
  Overview
  Stacks                   [2]      Attention count, not total objects
  Operations               [1]      Active operations

ADMINISTRATION
  Repository
  System

Appearance / density               Sidebar footer or System preferences
Version · connection state
```

Use five stable top-level destinations. Image updates, deployment history, and
activity share the Operations workspace and its filters. Container inspection
belongs to a stack; a global container inventory can be added when cross-stack
inspection is frequent enough to justify another destination.

### Global shell

- Desktop: approximately 232px collapsible sidebar, 56px header, full-width main
  workspace with 24px gutters. Collapsed navigation retains accessible labels.
- Header: breadcrumb, global search/command entry, live/connecting/stale state,
  and an active-operation indicator.
- Content: compact page title, purpose-specific toolbar, primary content. Put
  expensive actions next to the objects/workflows they affect.
- Global operation inspector: a right-side drawer, approximately 480px wide;
  expandable into a full operation page. Active work remains accessible during
  navigation. Closing the drawer does not cancel the job.
- Global search initially searches stack paths and navigation/actions. It can
  include recorded operations once that API exists. Support Cmd/Ctrl+K.
- Small screens: sidebar becomes an accessible drawer; inspection drawers become
  full-screen sheets. Keep stack identity, deployment, and runtime in the compact
  mobile list, with secondary fields available in details.

```text
┌────────────────┬──────────────────────────────────────────────────────┐
│ Docker / CD    │ Stacks / media / jellyfin       Search ⌘K   ● Live  │
│ home-server    ├──────────────────────────────────────────────────────┤
│                │ jellyfin                    [Open source] [Actions] │
│ Overview       │ media/jellyfin · main · Deployment failed           │
│ Stacks      2  │                                                      │
│ Operations  1  │ Overview  Containers  Configuration  Activity       │
│                ├──────────────────────────────────────────────────────┤
│ Repository     │ Deployment problem       │ Runtime                  │
│ System         │ Cause and next action    │ Containers / health      │
│                │                          │                          │
│                │ Latest operation and the change it attempted       │
│                │                                                      │
│ Preferences    ├──────────────────────────────────────────────────────┤
│ v…             │ Image update running · 2/6 stacks   [Inspect]       │
└────────────────┴──────────────────────────────────────────────────────┘
```

### Overview: triage and orientation

- Compact health strip: repository, deployments needing attention, runtime
  problems, active operations. Click through to the corresponding workspace.
- Attention list ordered by actionable reason, with stack, reason, age, and next
  action. Awaiting acknowledgement is a distinct state from deployment failure.
- Recent operations and upcoming image update, backed by real recorded data.
- Instance context in a secondary panel: Git ref, last successful check, policy.
- Primary stack browsing moves to Stacks, giving Overview a focused triage role.

### Stacks: the main working screen

- Dense data table by default; optional cards and grouping by repository path.
- Columns: stack, deployment, runtime health, running/total, applied revision,
  last successful deployment. Image reference and last error are optional columns.
- Filters: deployment, runtime, attention reason, path prefix. Search and active
  filters in the URL; density/visible columns stored as local preferences.
- Saved views such as “Needs attention”, “Deploying”, and “No health checks”.
- Stable row identity and predictable ordering during live updates; offer an
  explicit refresh/re-sort cue if changing values affect the selected order.
- Name opens the full detail route. A separate inspect action can open a drawer.
- Selection supports bulk operations only once those operations have real backend
  support and defined results for each selected stack.

### Stack detail: a resource workspace

| Tab | Content | Readiness |
|---|---|---|
| Overview | Deployment vs runtime, current issue, applied revision, latest attempt, relevant actions | Mostly existing fields; structured drift is new |
| Containers | Service table; inspection drawer with image, ports, state and health | Existing inspection API |
| Configuration | Compose metadata initially; source and desired/applied comparison later | Source/diff requires new API and historical snapshots |
| Activity | Filtered operations for this stack; drill into a run | Session-only initially; durable operations required for history |
| Logs | Container selector, stream, timestamps, pause/follow, bounded download | Add when backend log streaming exists |

Keep technical hashes copyable and available without dominating the overview.
Use full errors with a readable summary and expandable raw output. Retry must
describe whether it reapplies the current desired revision or performs a new Git
refresh; those are separate actions.

Routes can retain path-based stack identity: `/stacks/:path+?tab=containers`.
Keeping the tab in the query avoids ambiguity with nested stack names. Preserve
existing `/stack/:path+` links through a redirect. Operation details use a stable
`/operations/:operationId` route once IDs exist for all operation types.

### Operations: one place for execution and results

- Views: Active, All, Deployments, Image updates, Repository checks.
- Table: operation type, trigger, scope, start, duration, result.
- Inspector: phase timeline, affected stacks, image/layer detail, errors, result.
- Image update settings summary lives here: enabled, effective cron/timezone,
  next scheduled run, and last run. Configuration source links to System.
- Separate “Completed”, “Completed with errors”, “Failed”, “Cancelled”, “Skipped”,
  and “Interrupted by restart”. A cycle ending is not proof all stacks succeeded.
- Persist operation/step summaries, rather than every byte-progress event.
- Relationship: a repository check can cause multiple deployment operations; an
  image-update cycle can cause per-stack deployments. Store parent/child links.

### Repository: source and delivery triggers

- Configured repository link, ref, deploy scope, latest observed revision.
- Last successful refresh, latest failure, local clone condition where measured.
- “Check Git for changes”, with an explanation of active-work interruption.
- Discovered directory/stack tree with links back to stack details.
- Webhook setup instructions and polling configuration; delivery diagnostics
  become available after structured webhook records are added.
- Source actions open Git. Desired state remains authoritative in Git, consistent
  with the existing project constitution and read-only repository credentials.

### System: effective configuration and instance condition

- Instance: project name, version/build information, configured Docker endpoint,
  backend reachability and Docker probe result when available.
- Policies: reconciliation enabled, removals enabled, drift behavior.
- Scheduling: effective update configuration and polling interval.
- Configuration fields explicitly indicate environment/mounted-file ownership.
  Editable runtime settings require a separate persistence/precedence design.
- Preferences: theme, density, date format/timezone display, default stack view.
- Include endpoint health as measured state with an observation time. A browser
  connection indicator alone is not an engine-health check.

## 6. Visual and interaction direction

**A compact infrastructure admin console.** Neutral workspace, differentiated
navigation rail, one accent color, fine separators, and clear status typography.

- Give data and actions the screen area: 22–24px page headings, 13–14px table/body
  text, roughly 44px normal rows and an optional denser mode.
- Use status icon + text; semantic color supplements meaning. Reserve red for
  failures and unhealthy runtime, amber for attention/approval, neutral for unknown.
- Make health coverage explicit: “No health checks” and “Some checks passing” must
  remain distinguishable from a fully observed healthy service.
- One token source for shell and kit surfaces, typography, focus, spacing, and
  semantic states. Load fonts locally and use a consistent icon set.
- Use drawers for quick inspection; full routes for extended investigation and
  shareable history. Preserve focus and scroll when drawers close.
- Show loading/empty/unavailable/stale states per section. An unavailable Docker
  inspection should not erase repository information or claim zero containers.
- Long-running tasks are server-owned. Requests return promptly; navigation,
  refresh, and reconnect recover progress. Terminal outcomes remain discoverable.
- Charts follow measured history. Deployment duration and failure trends could be
  useful after history exists; current snapshots cannot support trend charts.

## 7. UI-kit study and decision

| Option | Fit for this console | Cost/tradeoff | Decision |
|---|---|---|---|
| **PrimeVue, styled mode** | DataTable filtering/sorting/grouping/state, rich selectors, drawers, tabs, trees, token-based themes | Migration, custom cell composition, theme integration and app-level accessibility testing | **Recommended for the expanded admin scope** |
| **Naive UI** | Already integrated; layout/menu, data table, drawer and theming cover an admin shell | Lowest migration cost; product-specific shell/table behavior still needs composition | Strong alternative if minimizing change becomes the priority |
| **Reka UI with a custom component layer** | Accessible headless primitives and maximal styling control | More ownership of styling, tables, forms and long-term interaction consistency | Best for investing heavily in a bespoke design system |

PrimeVue's advantage here is the amount of data-table and administrative interaction
behavior available to compose. Naive UI is also capable of building the proposed
layout; a library change is a scope/productivity choice, not a technical necessity.

Recommended implementation choices:

- Vue 3 SPA + Vite, TypeScript, Router and Pinia stay.
- PrimeVue styled mode with a custom Aura-derived preset. Current docs use
  `@primeuix/themes` and `definePreset`; resolve compatible releases during setup.
- Use the kit's DataTable, Drawer, Tabs, Menu, Tree, Select/MultiSelect, Toast and
  Dialog where useful; implement a focused application shell around them.
- Plain scoped CSS and shared tokens are sufficient. Utility CSS is optional,
  not a requirement for PrimeVue styled mode.
- Import only used components; keep inspectors and future log/diff tools lazy.
- Build a small set of product components: `AdminShell`, `ResourceHeader`,
  `StackStatus`, `StackTable`, `OperationInspector`, `ErrorPanel`, `EmptyState`.
- Keep one kit after migration. Port the few current Naive UI controls in the
  shell slice, then remove the dependency rather than maintain competing themes.
- Sakai is a useful published admin-layout reference. A custom shell avoids
  carrying unrelated template demo pages and assumptions into this repository.

Before committing the entire migration, build one real stack table and one
operation drawer with nested paths, long errors, light/dark themes, keyboard
navigation, and SSE updates. Verify focus stability and measured bundle output.

Official references consulted:

- [PrimeVue DataTable](https://primevue.org/datatable)
- [PrimeVue styled theming](https://primevue.org/theming/styled)
- [PrimeVue configuration](https://primevue.org/configuration)
- [PrimeVue Sakai](https://primevue.org/templates/sakai)
- [Naive UI theme customization](https://github.com/tusen-ai/naive-ui/blob/main/demo/pages/docs/customize-theme/enUS/index.md)
- [Naive UI DataTable API](https://github.com/tusen-ai/naive-ui/blob/main/src/data-table/demos/enUS/index.demo-entry.md)
- [Reka UI introduction](https://reka-ui.com/docs/overview/introduction)

## 8. Feature additions, ranked

Effort is relative (small/medium/large), not a delivery-time estimate.

| Priority | Addition | Value | Work and dependencies |
|---|---|---|---|
| P0 | Admin shell and dedicated workspaces | Organizes every current workflow | Medium frontend; route migration and E2E coverage |
| P0 | Instance/capabilities/effective configuration API | Explains disabled/blocked actions; powers Repository/System | Small–medium backend; explicit DTO, omit credentials |
| P0 | Rich table, path grouping, column visibility, saved views | Makes larger installations manageable | Medium frontend; reuse existing stack data |
| P0 | Global operation inspector | Progress remains visible across pages | Medium frontend; consume existing SSE initially |
| P1 | Structured drift reason + acknowledgement workflow | Exposes a real backend capability operators cannot currently use in the UI | Medium backend/frontend; bind action to reviewed desired state and report its scope |
| P1 | Durable operation history | Answers what happened while nobody watched | Large backend/frontend; persistence, IDs, outcomes, retention and query API |
| P1 | Per-stack retry and image update | Resolves a specific problem without a whole-instance action | Medium–large backend/frontend; asynchronous jobs and explicit policy semantics |
| P1 | Backend runtime summary and observation timestamp | Accurate health filtering without one request per table row | Medium backend; sampling/event ownership and partial-failure states |
| P2 | Read-only Compose source/diff | Makes deployment causes understandable | Medium–large; current desired source plus retained applied source |
| P2 | Container logs | Shortens diagnosis of in-sync but unhealthy apps | Medium–large; Docker streaming, backpressure, bounded retention and UI follow controls |
| P2 | Webhook diagnostics | Explains why pushes did/did not trigger deployment | Medium; delivery records and linkage to operations |
| P2 | Explicit application links and labels | Makes service access useful beyond guessed ports | Medium; metadata convention and clear source ownership |
| P2 | Notifications on failed operations | Useful when operators are away | Medium; build on durable terminal events |
| P3 | Global image/runtime inventory and metrics | Helps capacity and maintenance work | Large; inventory/telemetry APIs and retention |
| P3 | Multiple hosts/repos, team accounts and permissions | Expands the product beyond a single-instance agent | Large; ownership, identities, routing and persistence changes |

Some additions require explicit product decisions:

- Manual image updates when scheduled updates are disabled: today these are coupled.
- Configuration editing in-browser: today configuration belongs to deployment
  environment/files. Choose ownership and restart semantics before adding Save.
- Historical rollback: a Git revert fits current source-of-truth semantics;
  reproducible image rollback additionally requires recorded digests.
- Multi-host management: choose whether a central control plane manages agents or
  the browser connects to separate instances before designing a host selector.

## 9. Backend contracts needed by the design

These are proposed contracts, not existing endpoints.

1. `GET /api/instance`: display name, effective repository/ref/scope, policies,
   update enablement/effective schedule/timezone/next run, version and capabilities.
   Expose configuration ownership and credential-presence booleans where useful.
2. Enrich stack status with machine-readable reason, acknowledgement requirement,
   allowed actions, observation timestamps, and runtime summary. Retain separate
   deployment and runtime fields. Do not derive approved actions from error text.
3. `GET /api/operations` and `GET /api/operations/:id`: cursor-paginated summaries
   and detail, filtered by stack/type/result/time. Every operation has an ID,
   optional parent ID, trigger, scope, desired revision, start/end and outcome.
4. SSE operation events carry operation IDs and sequence/version information so
   bootstrap plus live updates can be reconciled. Reconnect restores active work;
   history comes from the durable query API.
5. `POST /api/operations`: policy-aware targeted actions with an immediate accepted
   response containing operation ID. Define idempotency and cancellation results.
   Existing refresh/update routes can adapt to the same operation model.
6. Read-only source/diff and container-log APIs follow when their tabs ship. Use
   existing nested path conventions or explicit path parameters consistently;
   avoid mixing resource suffixes into an unbounded wildcard stack path.

A small embedded operation store is a reasonable persistence direction. Persist
summaries, important step transitions, error text and necessary revision/digest
references. Keep high-frequency layer progress as live state. Specify retention
and represent jobs left active by a process crash as interrupted on restart.

The active-update endpoint currently returns a Go struct rather than a stable
frontend-oriented DTO. Normalize names, timestamps, errors and measured counters
as part of the operation contract; avoid building the new UI directly on raw
internal structs.

## 10. Frontend organization

```text
src/
  app/                 router, providers, initialization
  layouts/             AdminLayout
  components/ui/       shared product-level primitives
  features/
    overview/
    stacks/            table, detail tabs, runtime inspector
    operations/        list, drawer, progress, history
    repository/
    system/
  services/            typed HTTP and SSE transport
  stores/              instance, stacks/runtime, operations, preferences
  styles/              tokens, PrimeVue preset, global utilities
```

Separate transport state from page filtering and formatting. One SSE connection
feeds normalized resource/operation stores. URL state owns shareable filters and
tabs; preference storage owns theme/density/columns. Backend history owns retained
events. Reuse runtime inspection between table and detail views and move global
health summaries to the backend as installation size grows.

## 11. Delivery slices and validation

### Slice 1 — Admin foundation and existing functionality

Implement the shell, PrimeVue proof/migration, Stacks table, tabbed stack details,
Repository status, a session-based Operations view and global progress inspector.
Add the instance/capabilities DTO so System and disabled actions have real data.
Preserve current links and the existing GitOps browser coverage.

Acceptance: every current feature remains reachable; active work survives route
changes; nested paths, filters and themes survive navigation/reload; optional
updater states are explained correctly; mobile drawer and keyboard flows work.

### Slice 2 — Complete operational workflows

Add structured drift/acknowledgement, durable operations, and targeted actions.
Promote Operations from a session view to a historical workspace. Add restart
recovery for recorded jobs and explicit mixed-success results.

Acceptance: an operator can acknowledge a flagged change, trace a failed stack to
its operation, retry the intended scope, and inspect yesterday's results after
both a browser reload and a backend restart.

### Slice 3 — Investigation tools

Add source/diff, container logs, improved runtime sampling and webhook diagnostics.
Then consider notifications, explicit service links, and measured historical charts.

Acceptance: an operator can diagnose a deployment/configuration issue and an
independent runtime-health problem using clearly attributed evidence.

### Browser test plan

Keep the headed fixture walkthrough and real-Docker lifecycle suites. Extend them
with navigation/sidebar collapse, drawer focus restoration, browser back/forward,
mobile navigation, filters/columns, two-tab updates, mid-operation reconnect,
disabled updater, structured acknowledgement, long pulls and mixed-result cycles.
Use real backend integration tests for new operation persistence and action scope.
Keep screenshots for light/dark and desktop/mobile, with representative long paths,
large errors and empty/loading/stale conditions. Performance checks should include
a larger synthetic installation, not only the six-stack presentation fixture.

## 12. Relationship to project principles

- Desired state stays in Git; repository inspection and external source links fit
  the existing read-only repository model.
- Runtime actions continue through the reconciliation/Compose engine. New targeted
  actions must explicitly follow the intended policy and serialization semantics.
- Effective configuration remains environment/mounted-file-owned unless that
  model is deliberately revised in a later feature specification.
- Existing removal opt-in remains represented in the UI and enforced by the backend.
- New behavior gets a feature spec/implementation plan and appropriate automated
  regression/integration tests before implementation.

## Decision summary

Proceed with a **Vue + PrimeVue administration console**, organized around
**Overview, Stacks, Operations, Repository, and System**. Build the shell and the
existing workflows first, then prioritize **structured drift handling and durable
operations**. Those two capabilities provide the largest functional improvement
over today's status-oriented interface.
