# Operational Workspace — Design Study

Date: 2026-10-23 · Status: proposed design, awaiting Tech Lead review

**Read this before the [phased implementation plan](operational-workspace-redesign-plan.md).** This is a code-grounded conceptual study, not user-validated usability research. No live application inspection, usability sessions, performance measurements, or application implementation were performed. Wireframe data is illustrative. Existing historical screenshots are not evidence of the current running UI.

## 1. Problem and design thesis

The overview provides too little **actionable benefit**, not too little information. Adding more metrics would make the problem worse. Operators need to answer, in order:

1. What needs my decision, and what is already being done?
2. What is actually running inside this stack?
3. Can I safely update it, and what will that action affect?
4. Is it still pulling, applying configuration, or finished?
5. Which image does this running container actually use?

**Recommendation: make the stack inventory the operational home.** Put compact active work above a nested stack/container list; put the same work inline with its stack. Put instance-wide update tasks in that home, not exclusively in historical Operations. Keep investigation one disclosure away without making every stack a dashboard.

The intended character is a **calm utility workbench**: clear nouns, restrained separators, readable state sentences, precise scope, and progressive detail. Its distinctive feature is that the unit of work and the affected resource stay together. This is an information-architecture change, not a new coat of styling.

### Success hypotheses, to validate after implementation

- On first landing, an operator can see real container names and running work without navigating away or opening a drawer.
- An operator can start the existing all-stack image update from home, knowing it also prunes unused images across the Docker host.
- An operator can distinguish deployment success from runtime health and image identity from remote freshness.
- A new or returning connection never presents old pull bytes as a new run or infers success from a vanished progress bar.
- At 250 stacks, search, pagination and disclosure remain predictable; no telemetry-driven reordering steals focus.

## 2. Evidence and constraints

Current source takes precedence over older design documents. These references were read for this study:

| Evidence | Finding | Design consequence |
|---|---|---|
| `frontend/src/pages/OverviewPage.vue:13–167` | Metrics, attention, environment and recent operations; navigation rather than update actions | Replace the landing detour; do not enlarge the dashboard |
| `frontend/src/pages/StacksGrid.vue:15–227` | Flat paginated table, optional cards/directory grouping, optional columns; runtime summary but no nested rows | Use one strong inventory presentation with actual children visible initially |
| `frontend/src/pages/StackDetail.vue:61–318` | Five tabs; containers show image tag and container ID; inspector is separate | Keep detail deep links and investigation, promote image identity and active work |
| `frontend/src/App.vue:102–129,282–309` | Fixed work bar plus footer; runtime polled every 15 seconds while visible and refreshed on stack events/reconnect | Remove duplicated chrome; reuse automatic observation, fix consistency rather than add a refresh ritual |
| `frontend/src/store/console.ts:29–84`; `composables/useStackContainers.ts` | Active count includes every running record, including parent/child; separate detail cache; runtime errors hide retained rows | Derive workflow groups, use one runtime cache, preserve last known data explicitly as stale |
| `frontend/src/store/stacks.ts:46–50,147–255`; `components/DeploymentProgress.vue`; `services/progress.ts` | Singleton update/pull state; image/layer events can overwrite another stack's display | Truthful inline multi-run progress requires a contract change before UI rollout |
| `backend/internal/desiredstate/events.go:71–108,147–166` | Reconnect snapshot exists, but pull cache keyed by source then image/layer | Add execution identity, ordered replacement snapshots and explicit cleanup; never correlate by source alone |
| `backend/internal/desiredstate/operations.go`; `http/console.go:108–148` | Operations have ancestry/versioning, direct lookup and filtered history; pages default to 50 | Active work must not depend on the latest history page; related records require server filters/ancestry retrieval |
| `backend/internal/desiredstate/state.go:29–38`; `reconcile/compose.go:69–139` | Runtime is actual `compose ps -a` containers; ID shortened; no image digest, no pre-creation service DTO | Label absence honestly; never invent desired containers from image pull events |
| `backend/internal/docker/images.go:154–162`; `docker/sdk.go:15–23` | `GetImageDigest` returns `ImageInspect.ID`; SDK supports inspecting a container and image | Do not relabel this as a repository digest; inspect the immutable image used by the container |
| `backend/internal/http/console.go:150–217`; `reconcile/actions.go`; `reconcile/reconcile.go:271–426` | Reviewed actions enforce policy/revision/duplicates; `retry` and `update` both pull then run Compose up; no force recreation or prune | Preserve checks; differentiate operator intent without claiming different machinery |
| `backend/internal/scheduler/scheduler.go` | All-stack updater is sequential, skips failed stacks, applies changed/unknown images, ends with system-wide unused/dangling image prune | Global action must disclose broader scope than stack-only update; not a selected-row command |
| `frontend/src/pages/RepositoryPage.vue:9–29`; `backend/internal/refresh/refresh.go`, `queue.go` | Manual Git check requests cancellation before refresh/reconciliation; periodic check queues instead | Never call Git check “Refresh”; interruption warning must be at invocation |
| `backend/internal/http/router.go` | No selected-stack batch, container-update, cancel, or remote freshness endpoint; no general console authentication middleware shown | Do not fabricate controls; capabilities are not authorization |
| `frontend/src/router/index.ts` | Nested `/stacks/:path+`, legacy `/stack` redirect, query-backed tabs | Preserve nested paths, query state, and navigation return context |
| `frontend/package.json`, `backend/go.mod` | Vue/Pinia/PrimeVue already present; DM Sans + IBM Plex Mono; Docker SDK v28.5.2 | Reuse stack and fonts; do not introduce a UI framework or require a Docker upgrade |

`specs/009-admin-console/study.md` and `spec.md` explain previous intent, but the study's capability inventory predates current source/diff, logs, history and reviewed actions. `.lead/_adhoc/reconcile-pull-progress.md` is a completed predecessor: explicit reconcile pulls already exist; this proposal fixes correlation and placement, not their initial introduction.

Current Docker documentation consulted through Context7: the official Docker docs source for [multi-platform digests](https://github.com/docker/docs/blob/main/content/manuals/dhi/explore/security-concepts/digests.md) and [manifest/config distinction](https://github.com/docker/docs/blob/main/content/manuals/build/metadata/annotations.md). A manifest index, a platform manifest and a classic image-store config ID are different identities. Newer Engine descriptor APIs cannot be assumed available with this repository's negotiated SDK/daemon versions.

## 3. Three distinct information-architecture strategies

| Strategy | Home and navigation | Strength | Cost / failure mode | Verdict |
|---|---|---|---|---|
| A. **Resource-first workbench** | Stacks is home; active work above and inside nested inventory; Operations is deeper execution/history | Direct answer to “what is running and what can I do?”; preserves context during work; useful even when idle | Requires disciplined density and a trustworthy join between runs and resources | **Choose**: strongest fit to expressed needs, bounded migration |
| B. **Task inbox first** | Home is approvals, failures and update tasks; separate inventory browser; completed tasks move to archive | Excellent for many approvals and shift handoff; no passive metric cards | Healthy resources disappear from home; operators still leave home to inspect containers; image update becomes an inbox artifact even without a known pending change | Discard for MVP: over-optimizes intermittent decisions, under-serves daily runtime inspection |
| C. **Explorer + persistent execution console** | Left resource tree, center inspector, right/bottom work console; selection drives all panes | Very efficient for a power operator investigating one resource repeatedly; context always present on a wide display | Three competing scroll/selection regions; poor narrow-screen adaptation; tree navigation hides containers until selected; more state and keyboard complexity | Discard: risks replacing current clutter with an IDE; could suit a future specialist mode, not a default |

A differs from a dashboard with a table appended: the inventory is the page's primary content and owns both decisions and live execution. The upper work section disappears down to one quiet line when idle. There is no second set of health cards, recent-history cards or environment panels.

## 4. Chosen layout and hierarchy

### 4.1 Desktop home — useful before any interaction

```text
┌──────────────────┬─────────────────────────────────────────────────────────────────────────────┐
│ Docker / CD      │ production-eu                                  Connected · Runtime 8s ago [1]│
│                  ├─────────────────────────────────────────────────────────────────────────────┤
│ Stacks           │ Stacks  42                     [Check Git for changes…] [Update all stacks…] │
│ Operations       │ Git main · a49f2c1 · checked 2m ago              Scope: entire instance [2]  │
│                  │                                                                             │
│ Repository       │ WORK IN PROGRESS · 2 workflows                         [Show all work (2)]  │
│ System           │ Image update · 4 / 12 stacks processed · pulling apps/web   [Details]      │
│                  │ Deployment · tools/metrics · applying Compose               [Details] [3]  │
│                  ├─────────────────────────────────────────────────────────────────────────────┤
│                  │ [Search stacks, containers, images…             ] [All ▾] [Display ▾]       │
│                  │ All stacks (42)   Needs attention (3)   Active work (2)                       │
│                  │                                                                             │
│                  │ STACK / CONTAINER                 DEPLOYMENT          RUNTIME / IMAGE        │
│                  │ ▾ apps/web                        Applying a49f2c1    2 / 2 running          │
│                  │   Pulling nginx:stable · image 1/2 · 3 layers downloading   [Pull details ▾] │
│                  │   ├ web-1                         Running · healthy                         │
│                  │   │ nginx:stable                  repo digest sha256:2b91…8af0 [Copy]        │
│                  │   └ proxy-1                       Running · no health check                 │
│                  │     caddy:2                       repo digest sha256:851c…c108 [Copy]    [4]│
│                  │ ─────────────────────────────────────────────────────────────────────────── │
│                  │ ▾ data/postgres                   Approval required   1 / 1 running         │
│                  │   Desired 31be09a · applied 183bf42                     [Review change]     │
│                  │   └ db-1                          Running · healthy                         │
│                  │     postgres:17                   repo digest sha256:17ad…300c [Copy]    [5]│
│                  │ ─────────────────────────────────────────────────────────────────────────── │
│                  │ ▾ tools/metrics                   Applied a49f2c1     4 / 5 running         │
│                  │   Applying Compose · started 30s ago                   [Work details]      │
│                  │   ├ exporter-1                    Exited                                    │
│                  │   │ prom/node-exporter:v1          Digest unavailable [Why?]                 │
│                  │   └ prometheus-1                    Running · healthy                         │
│                  │     prom/prometheus:v3            repo digest sha256:8e03…19a0 [Copy]        │
│                  │     [Show all 5 containers]                                             [6]│
│                  │                                                                             │
│                  │ 1–20 of 42 stacks      [Previous] [1] [2] [3] [Next]                         │
│                  │ Observations refresh automatically every 15s · [Refresh runtime]        [7]│
└──────────────────┴─────────────────────────────────────────────────────────────────────────────┘
```

Annotations:

1. One compact shell line: instance identity, connection, independently dated runtime. “Connected” never promises current health. Workspace search remains available through Cmd/Ctrl+K; theme/density preferences move under Display/System rather than claiming prime space.
2. Global actions are outside the inventory toolbar. Their scope never changes with search, page or expanded rows. Update is the emphasized action; Git check is quieter but readily reachable. Do not offer both generic “Sync” and “Refresh.”
3. Show the first two **root workflows**, ordered by start time, oldest first. Parent action and child deployment occupy one group, not two jobs. The summary includes progress stage and affected stack, not just a spinner. No modal or drawer needed to learn what is happening.
4. Stack row owns deployment intent/status and running count. Indented child rows own actual runtime and image identity. A running container remains visible while its replacement image pulls. Pull targets are not relabeled as the image already running.
5. Approval and runtime are independent. One explicit “Review change” action replaces a red failure badge plus approval badge plus retry button. Applied/desired revisions are revealed when different, not repeated on every child.
6. Two real containers are visible by default per stack. Exceptions appear first in this preview, with stable ordering rules below. Show-all is explicit and bounded. This is useful progressive disclosure, not a closed tree masquerading as container visibility.
7. No fixed bottom active-work bar, ornamental footer, or second refresh command. Pagination counts stacks, never child rows.

### 4.2 Expanded pull detail — phase first, honest quantities second

```text
▾ apps/web       Deployment: pulling images       Runtime: 2/2 running (observed 8s ago)
  Image update / run 09b…   Started 14:23:01                      [Open operation]
  ▾ nginx:stable           Downloading · image 1 of 2
      layer 4fc…   Downloading  18 / 40 MB  [=========           ]
      layer 772…   Extracting   4 / 12 MB  [======              ]
      layer 90d…   Waiting     total unknown
    caddy:2                Waiting to pull
  Runtime still describes the current containers; deployment has not finished.
  ├ web-1   Running · healthy     nginx:stable  sha256:OLD… [Inspect image]
  └ proxy-1 Running · no check    caddy:2       sha256:OLD… [Inspect image]
```

Only enumerate waiting images if the server supplied the run's image inventory; otherwise show “Image 1 of 2,” not invented names. Layer bars are phase-local, not percentages of the whole deployment. Download plus extraction is not a meaningful single denominator. After pulls complete, replace the pull headline with “Applying Compose”; do not display “Deployment 100%.” Bytes remain in the open live inspector until terminal state, but are not promised as durable history.

### 4.3 Mobile home — same task model, not squeezed desktop columns

```text
┌──────────────────────────────────────┐
│ [Menu] production-eu      [Work 2]    │ [a]
│ Connected · Runtime observed 8s ago   │
├──────────────────────────────────────┤
│ Stacks 42                            │
│ [Update all stacks…] [Git check…]     │ [b]
│ Applies across this instance         │
│                                      │
│ WORK IN PROGRESS                      │
│ Image update · 4/12 stacks processed │
│ apps/web · Pulling nginx:stable       │
│ [Details]      [Show all work (2)]    │ [c]
│                                      │
│ [Search stacks, containers, images…] │
│ [All stacks ▾]     [Display ▾]        │
├──────────────────────────────────────┤
│ ▾ apps/web                           │
│ Deployment: pulling images           │
│ Runtime: 2/2 running                 │
│ Image 1/2 · downloading              │
│ [Pull details ▾]                     │
│ │ web-1  Running · healthy           │
│ │ nginx:stable                       │
│ │ sha256:2b91…8af0 [Copy digest]      │ [d]
│ │ proxy-1 Running · no health check  │
│ │ caddy:2                            │
│ │ sha256:851c…c108 [Copy digest]      │
├──────────────────────────────────────┤
│ ▾ data/postgres                      │
│ Deployment: approval required        │
│ Runtime: 1/1 running                 │
│ [Review change]                      │
│ │ db-1 Running · healthy             │
│ │ postgres:17 · sha256:17ad…300c      │
│                                      │
│ 1–20 of 42 stacks   [Prev] [Next]     │
└──────────────────────────────────────┘
```

- **a:** A single Work button opens the shared all-work inspector; it is not another bottom overlay. Normal page content scrolls. Header must not obscure focused controls.
- **b:** Compact visible “Git check…” has accessible name “Check Git for changes”; its impact text is identical to desktop. Buttons wrap, never truncate their operative verb.
- **c:** Mobile previews one workflow, desktop two; explicit total and Show all preserve access to every concurrent workflow, including filtered-out stacks.
- **d:** Identity wraps onto a second line; copy returns the full value, not the elided text. Tap container name to inspect. No hover-only affordances or page-wide horizontal scrolling at 320 CSS pixels.

### 4.4 Decision and image inspection panels

```text
INLINE STACK ACTION (after choosing Update images…)
  Update images in apps/web?                         [Close]
  Pull declared images, then apply Compose at a49f2c1.
  Containers may be replaced. No forced recreation; no image prune.
  [Update apps/web] [Keep running unchanged]

REVIEW PAGE / existing Configuration tab
  data/postgres · configuration change requires approval
  Applied 183bf42            Reviewing desired 31be09a
  [Applied source | Desired source | Diff]
  ... read-only source comparison ...
  [Approve & deploy 31be09a] [Back to stacks]
  If revision changes: “New desired revision available. Review again.”
  The approval button is disabled until that new comparison is loaded.

IMAGE INSPECTOR (side sheet on desktop, full-screen sheet on mobile)
  web-1 / apps/web                                      [Close]
  Running image reference     nginx:stable
  Repository digest           docker.io/library/nginx@sha256:… [Copy reference]
  Digest kind                 Unknown (reported by local daemon)
  Running image platform      linux/amd64
  Observed                    14:22:58
  Registry freshness          Not checked
  Compare against the same repository and digest kind.
  A platform manifest differs from its multi-platform index.
  [Show other reported digests (2)] [Technical identifiers ▾]
  Technical identifiers: exact image ID; full container ID (when available).
  [View logs] [Open stack]
```

No automatic status such as “outdated” appears in this MVP. Copyable identity lets an operator perform a manual comparison against a registry or another environment. A difference between repository digests is evidence of a different artifact, not by itself evidence that one is newer. Multiple repository names may refer to identical content; textual ref equality and digest equality are distinct.

## 5. Exact inventory interactions

### Defaults and disclosure

- Canonical home is `/stacks`, initially All stacks, path ascending, 20 stacks per page. No default hiding of healthy stacks. A directory grouping option adds nonselectable headings, not another expandable tree layer. Full stack paths stay discoverable.
- Every newly encountered stack starts in **preview mode**, showing up to two observed containers. Preview priority is: non-running containers, then unhealthy/starting running containers, then remaining containers; ties by service, name, ID. “No health check” is neutral, not unhealthy.
- Freeze preview order for the current page visit after the first successful observation. State text can change in place; newly observed containers append. Re-entering the page or an explicit sort can recalculate priority. Never move a focused row because health changed.
- Stack chevron means **Hide containers** when open; **Show containers** when closed. Reopening restores the last preview/full mode for that stack. Collapsed stack still shows runtime summary and inline work. “Show all N containers” changes preview to full mode, initially 20 children; “Show 20 more” reveals the next 20; “Show fewer” returns to preview. It never fetches a different selection of desired services.
- Display menu offers Comfortable/Compact, Group by directory, Show container previews on this page, Hide containers on this page. These page-scoped controls affect only visible stack records; they never expand 250 off-page stacks. No permanent list/cards toggle or column picker in the new default. Configuration remains read-only.
- Keep disclosure/page state in session history keyed by full stack path and normalized filters. Density persists as today. The URL owns search/filter/group/page; expansion need not produce a huge URL. Browser Back restores page, focus/scroll anchor, filters and disclosure without reordering.
- A stack with no observed containers shows one indented sentence: “No containers observed” and last observation time. If deployment is missing: “Not deployed yet.” If inventory failed: “Runtime unavailable,” not “0 containers.” Pre-creation desired services are out of scope.

### Search, filters and result scope

- Search case-insensitively across stack path, observed container/service name, image reference and known full repository digest. Literal substring search, not an undocumented query language. Search/filter changes reset to page 1; debounce typing approximately 200 ms.
- A stack-name match shows its normal preview. A child-only match includes its stack ancestor and reveals up to two **matching** children, regardless of saved collapse state; text says “2 of 7 matching containers.” “Show matching containers” pages the remaining matches; “Show all N containers” exits matching-only child display for that stack. Clearing search restores pre-search disclosure.
- Quick filters: All stacks, Needs attention, Active work. Needs attention includes approval, failed/missing deployment, non-running/unhealthy runtime, and unavailable/stale observation as **unknown**, with a textual reason. A running container without a health check alone is not attention. Filter reasons never imply an update is available.
- More filters contains Deployment and Runtime separately (including No health checks and Unknown), so failure and runtime health do not become a single overloaded status. Compatibility maps existing `status=syncing` and `status=unchecked` links appropriately.
- Runtime/image matches are based on the last observation, explicitly dated. Unknown runtime must not silently match “Healthy.” While observation is unavailable, search notes that container/image results may be incomplete.
- Active-work section is instance-wide and unaffected by inventory filters. A run for an off-page/filtered stack includes “Outside current view · Open stack.” Display “3 matching stacks; 2 workflows across instance,” not a count implying every job belongs to the result set.

### Selection and action scope

**No selection checkboxes in MVP.** Disclosure, search and navigation do not select action targets. Single-stack actions name the full path. “Update all stacks…” always targets the existing instance-wide updater, not visible results, expanded stacks or children. Container rows offer inspection, copy and logs only; no update/restart/delete controls. There is no client-side loop pretending to be an atomic selected-stack batch.

## 6. Primary operator journeys and safe controls

### Journey A — inspect and act without leaving home

Open Stacks → read work/attention → expand the relevant observed children if needed → choose a stack action → confirm in-place scope → follow inline progress → inspect exception only if necessary. Completed deployment and runtime confirmation arrive independently. A healthy runtime after a failed deployment is not contradictory.

### Journey B — routine image maintenance

Open Stacks → “Update all stacks…” → review host-wide impact → “Update all stacks and prune images” → return immediately to the inventory with server-owned active work. Sequential processing shows current stack and processed count, skipped/failed outcomes explicitly. Filtering does not hide global work. Completion summary points to durable operation detail; runtime stays dated until observed again.

### Journey C — Git change and approval

“Check Git for changes…” → inspect impact/cancellation warning → check completes → automatic policy deploys or a stack shows “Review change.” Reviewing navigates to the existing source comparison. Approval binds to the reviewed revision, not a reactive current revision variable. If desired revision changes during review or while queued, require review again; preserve the server's rejection and never silently resubmit with the latest hash.

### Journey D — identify an unexpected image

Search image/service → actual container child → copy repository digest or open image inspector → compare same kind/platform against external evidence → decide whether to update the **stack**. After pulling, old running identity remains until a new observation shows the replacement. No freshness promise from the mutable tag, local image ID, or successful pull alone.

### Journey E — reconnect during work / leave and return

Navigate to Repository/System while work runs → header Work button remains accessible → return to Stacks or open a direct operation link → ordered authoritative snapshot restores all live runs. Old bytes are retained only as stale while reconnecting; active rows must not disappear into success. History and live work can recover independently.

### Control contract

| Visible label | Actual scope and backend | Interaction / safety |
|---|---|---|
| **Refresh runtime** | Observational `GET /api/runtime` only; no Git, pulls or Compose | Direct click, no confirmation; automatic 15-second visible-page observation remains the default |
| **Check Git for changes…** | Instance `POST /api/refresh`; refresh clone/discovery and reconcile by policy; requests cancellation of active deployment/update work | When idle, compact inline confirmation: “May deploy/remove stacks according to policy”; confirm “Check Git and apply policy.” With active work, impact dialog names affected workflows and says “Requests interruption; work may already have made changes”; confirm “Interrupt work and check Git.” Never describe this as dry-run or harmless refresh |
| **Update all stacks…** | Instance `POST /api/update`; sequential eligible stacks, skip failed stacks; changed/unknown images can trigger Compose; host-wide unused/dangling image prune | Impact dialog every time: full scope, skips/blocks, possible replacement, system-wide prune including images outside managed stacks. Confirm **Update all stacks and prune images**. Cancel closes without request. Do not invent a no-prune checkbox |
| **Update images…** | One healthy/ordinary stack, `POST /api/operations` action `update`, exact stack and reviewed revision | Inline scope review shown in §4.4, not a full-screen everyday modal. Hide this choice when approval is required. Capability/policy/duplicate gates respected |
| **Retry deployment…** | One failed stack, action `retry`; same pull + Compose algorithm as `update`, no prune | Inline scope review; confirm “Retry {full stack path}.” Do not call this “Force redeploy” or promise restart of unchanged containers |
| **Review change** → **Approve & deploy {revision}** | One stack, action `acknowledge`; apply exact reviewed revision, not all pending changes | Source/diff is the confirmation surface; no extra redundant modal. Explicit approval click only after comparison loaded; failed/changed revision requires review again |
| **Details / Pull details / Open operation** | Read-only; identified operation/run, not whichever is first in a global array | Inline disclosure by default, inspector for depth, stable full-page URL for history |

Disabled controls remain explainable: reconciliation disabled, updater disabled, refresh incomplete, approval required, duplicate/busy, stale state. Show concise adjacent reason or keyboard-accessible explanation; do not rely solely on hover tooltip. Unsupported capability can be omitted from container menus, but central image updates should explain “Image updates disabled by instance configuration” with a System link.

While disconnected or before the initial authoritative state, mutation controls are disabled; read-only inspection/copy continue against clearly dated data. After request: “Request accepted; awaiting execution” is distinct from “Running.” A network error after POST is “Outcome unknown; checking operations,” not “Failed, retry now.” Do not automatically replay mutations. The single-stack endpoint returns an operation ID; global update/Git endpoints currently do not, so correlate those via their subsequently observed server workflow rather than inventing an ID or success.

No Cancel button: a manual Git check's interruption side effect is not a general cancellation API. No promise of rollback or transactionality. Progress can report backend cancellation without offering cancellation as a control.

## 7. State catalogue

| State | Home / nested row | Operator path |
|---|---|---|
| Loading first snapshot | Stable skeleton sized for stack+two-child blocks; “Connecting to agent” | No zero counts or enabled mutation actions |
| Empty repository | “No stacks discovered. Add Compose in the configured Git repository.” | Open Repository; existing Git-check control with policy warning |
| No filter matches | “No stacks match these filters”; keep instance-wide active work visible | Clear filters, preserving global scope |
| No active work | One quiet line “No work in progress”; no empty panel/spinner | Inventory and actions retain their place |
| Multiple active workflows | First two desktop/one mobile, explicit total, expand all-work list in-place (10 per page) | Every workflow reachable; parent/child counted once; off-view target links |
| Accepted / waiting | “Requested; awaiting execution” tied to returned operation, or local request status until server observed | Do not fabricate queue position; duplicate submit blocked |
| Pull totals unknown | Phase text / indeterminate activity; known layer bytes may still show | Layer expansion; no fabricated 0%/ETA |
| Pull complete, Compose active | “Images pulled · Applying Compose” | No completed deployment state yet |
| Backend terminal success | “Deployment completed · Runtime observation pending” | Refresh runtime automatically; runtime may still be unhealthy |
| Terminal error / partial global outcome | Red exception sentence with stack/phase; global summary “Completed with 2 failed stacks,” not all-green success | View errors / retry applicable stack; skipped does not mean updated |
| Cancelled / interrupted | “Cancelled by server” only on terminal evidence; retained partial outcomes | Inspect operation; no assumption of unchanged runtime |
| Approval required | Amber “Approval required”; desired/applied difference; runtime still visible | Review change; no direct update bypass |
| Runtime request fails | Retain last known children with “Observation failed; last success {time}”; per-stack failures stay local | Refresh runtime; not equivalent to no containers |
| Stale observation | Mark runtime stale after 45 seconds since successful observation while visible; hidden-tab return marks pending until refresh succeeds | Unknown filter/attention reason; threshold is a display rule, not a health failure |
| SSE reconnecting | One shell notice; live run text/bytes freeze and become “Last received {time}”; no animated fake advance | Auto reconnect; preserve filters/focus; disable mutations |
| Connected but snapshots not recovered | “Restoring work state…”; no premature Connected/healthy interpretation for work | Wait for snapshot barrier and runtime separately |
| History unavailable / persistence failed | Separate notice “Live work available; history unavailable” or “History not being saved” | Retry history; do not conflate with deployment failure |
| Operation expired / parent absent | “Operation not found or expired”; children identified where possible | Return to stack/history; never label loaded subset as complete ancestry |
| Container removed/replaced while inspecting | “This container is no longer in the current observation”; retain dated identity | Open replacement container explicitly; never silently switch log/copy target |
| Digest unavailable / ambiguous | “Digest unavailable” or “Multiple repository identities”; reason in inspector | No guessed fallback to container ID; full technical image ID separately labeled |

Terminal success remains inline for 10 seconds, then becomes a compact last-result sentence; failures remain until superseded by a new authoritative run/result. An open detail panel is not auto-closed. Timers never set success and never discard the only reference to an error. Completion announcements occur once per workflow, not once per image layer.

## 8. Design-level data contracts (MVP requirements, not implemented)

### 8.1 Correlated live work

Add a versioned live-work representation alongside legacy `update.progress` during migration, rather than attempting to infer identity from the existing singleton. Proposed SSE names: `work.snapshot` and `work.upsert`; advertise `capabilities.liveWorkVersion: 2`.

**Event envelopes:** `work.snapshot` contains schemaVersion, epoch, sequence watermark, runs (all active run records), and operations (all running operation records plus the ancestor records needed to group them). `work.upsert` contains schemaVersion, epoch, sequence, an optional complete run record and an operations array of complete changed operation records. Operation-only updates are valid, so a running Git check/action waiting on a lock is visible even before pull data exists. Existing `operation.upsert` continues for history compatibility; the new work store applies ordered work envelopes as its active-membership authority. Run fields below belong to the run record unless explicitly identified as envelope ordering fields.

| Field | Meaning / invariant |
|---|---|
| `schemaVersion`, `epoch`, `sequence` | Schema version; server-lifetime identity; monotonically increasing publication sequence. Compare ordering within epoch, never across restarts |
| `runId` | Unique execution attempt, never stack path, source, image or a reusable cycle label; a retry is a new run |
| `operationId`, `rootOperationId`, `parentOperationId?` | Durable operation owner and workflow ancestry. A stack pull within a global update can own the global operation while its later deployment owns a child; both group under one root |
| `source`, `stack?`, `revision?`, `cycleId?` | Reconcile/update/refresh origin and optional target context, not identity keys; global prune has no container target |
| `status` | `running`, `completed`, `failed`, `cancelled`; waiting stage is a phase when actually known. UI-only submission pending is not a backend run |
| `phase` | `waiting`, `checking_repository`, `pulling`, `applying`, `pruning`, `finished`; only truthful backend stages, with optional reason text |
| `startedAt`, `updatedAt`, `finishedAt?`, `message`, `errorCode?` | Server times and safe display outcome; raw credential-bearing errors must not be exposed |
| `images[]` | Each has image reference, pull-attempt identity, status, done, optional known position/total, and `layers[]`; layer ID is only unique within that image pull attempt |
| `layers[]` | ID, phase/status, optional current/total bytes, done, updatedAt. Null/absent total is unknown, never an invented zero-total percent |
| `stackResults?` | For global update: processed, skipped, failed counts and bounded outcome references; no claim that all discovered stacks are eligible |

Required semantics:

- Snapshot is atomic with event subscription and includes its sequence watermark, **all active runs**, active operation roots and necessary ancestor summaries, regardless of the latest-50 history page. Include standalone running operations with no pull data. Completed parent with active child still constitutes one active workflow.
- `work.upsert` is a full replacement of one run at a sequence, not a fragile partial layer patch. Coalesce byte ticks per run (up to roughly 2 deliveries/second), but flush phase changes and terminal outcomes immediately. Bound live-layer memory and snapshot size; a truncated detail set declares `detailsTruncated`, preserving truthful summary rather than silently inventing a denominator.
- Key state by epoch + run ID; image attempt + layer ID beneath it. Ignore older/duplicate sequence updates. Snapshot watermark rejects earlier events. Terminal run cannot return to running; new attempt requires new ID.
- Reconnect replaces active membership, not an append-only merge. Remove omitted old live layers; mark formerly known but absent runs “No longer active; retrieving outcome” until operation lookup establishes terminal state. Absence alone is not success. Restart invalidates live-layer cache and resolves persisted interrupted operations according to existing operation-store semantics.
- Completed pulls do not complete deployment. Root remains active through child execution/prune. Count unique roots with any active run/running descendant; show children inside the group, not as duplicate top-level jobs. If ancestry is genuinely unavailable, display “Grouping unavailable” rather than guessing from matching times/stack.
- Do not put layer bytes in durable history. Keep phase/outcome history and the current live snapshot. Reopening a completed run says “Live pull detail is no longer retained” if necessary.
- Preserve existing scheduler/reconcile execution ordering and backend locks; multi-workflow visibility is not permission to introduce parallel mutation execution.
- Legacy servers: show operation-based stages and “Detailed live progress unavailable on this agent”; never place uncorrelated legacy bytes on a stack row. New backend keeps old events during frontend migration, then removes their use in the new UI.

### 8.2 Running-image identity

Extend the existing `ContainerInfo` returned through runtime/detail with optional `imageIdentity`. Keep existing `image` and `id` for compatibility. Proposed fields:

| Field | Meaning |
|---|---|
| `status` | `available`, `unavailable`, `ambiguous`; absent identity means older backend, not error |
| `observedAt`, `source` | When inspected; `container_image_inspect` (local, not registry lookup) |
| `imageId` | Immutable image identifier taken from the exact inspected container, separately labeled “Local image ID” |
| `reference` | Container's configured image reference, not a newly resolved mutable tag |
| `platform` | `{os, architecture, variant?}` of that image, when known |
| `repositoryDigests[]` | `{repository, digest, kind}` where kind is `index`, `manifest`, or `unknown`; never default unknown to manifest |
| `preferredRepositoryDigest?` | Unique unambiguous digest matching the normalized repository of the container reference; omit if more than one plausible match |
| `reason?` | Safe enum such as `not_recorded`, `image_missing`, `container_replaced`, `inspection_denied`, `repository_ambiguous`, `unsupported` |
| `freshness` | MVP fixed value `not_checked`; no implicit remote comparison |

Resolution rules:

1. Start from a server-enumerated container in a known stack. Retain full Docker ID internally even if display uses 12 characters. Inspect **that container**, read its immutable image ID, then inspect that ID. Never inspect only `nginx:stable`: a pull can retarget it while the old container keeps running.
2. Return `RepoDigests` as repository identities reported by the local daemon; normalize repository names with a proper image-reference parser. If exactly one matches the configured repository, prefer it. Otherwise expose alternatives with no arbitrarily selected default. Local builds can legitimately have none.
3. Platform identity is distinct from digest kind. Older APIs may supply RepoDigests without proving index versus manifest; mark kind unknown. Do not require newer Docker descriptor APIs to ship MVP. Repository digests associated with a local image do not prove which mutable tag currently points there, nor exact historical pull provenance if multiple associations exist.
4. Missing metadata or an image removed between calls must degrade only image identity, not the whole runtime observation. A container disappearing during inspection is not permission to look up a replacement by name.
5. Memoize image inspection by immutable image ID for a bounded observation cycle/cache; do not cache tag identity. Coalesce simultaneous runtime requests, bound inspection concurrency/timeouts, and invalidate after terminal deployment/reconnect. Do not turn a 250-stack page into 250 overlapping browser polls.
6. This DTO is useful for manual comparison. **Remote freshness is optional later scope**, requiring remote manifest/index resolution, same-repository + same-platform/kind comparison, checkedAt/TTL, authentication, rate-limit/backoff and unsupported states. A check must be read-only, not an alias for image pull; it must not be implemented by “update then see if ID changed.”

### 8.3 Runtime and security boundaries

One frontend runtime cache serves nested inventory, detail, image inspector and logs selection. Visible pages share the existing 15-second poll, immediate observation on visibility return/reconnect, and debounced refresh after stack/terminal events. Coalesce in-flight requests; response sequencing must prevent an older poll overwriting a targeted newer observation. Preserve last-success timestamps per stack and keep transport error separate from data. SSE health and runtime freshness are separate facts.

The router currently does not implement a general authenticated-user/role system. Capability flags are feature availability, **not authorization**. This design does not silently add an IAM project or claim current endpoints are protected. Deployment must retain an authenticated/restricted network or reverse-proxy boundary for console APIs. Review that boundary before exposing additional inspection metadata.

New inspection must remain read-only and restricted to server-discovered stacks and their verified containers, using existing Compose/managed labels and exact stack-path association. Do not add an arbitrary Docker image/container inspection proxy, accept user-supplied Docker socket URLs, or expose the SDK's raw inspect JSON. Whitelist identity/platform fields only: no environment, mounts, labels containing secrets, credentials or registry auth in client payloads, logs or SSE. Unknown ownership degrades to unavailable identity rather than inspecting an arbitrary host resource. All action authorization/policy checks remain server-side and repeated under existing locks. A future remote lookup must use server-side credential resolution and egress/registry policy, not browser tokens or unconstrained user-provided URLs.

## 9. Visual density, keyboard and accessibility

- Reuse DM Sans for labels/body, IBM Plex Mono for paths/digests; no font download or UI-kit migration. One consistent small heading scale; 14–16 px body, 12–13 px technical secondary text, never microscopic status labels.
- Neutral light/dark surfaces, one subtle boundary between stacks, faint indent guides for children. Green is small and secondary; amber denotes decision/unknown, red denotes failure, one accent denotes action/progress. No colored pill for every fact, no gradient metric cards, no giant health numerals.
- Comfortable desktop stack header about 48 px plus optional contextual sentence; child about 48 px over two lines. Compact reduces spacing, not meaning; controls retain usable hit areas. Mobile interactive targets at least 44×44 CSS pixels even if their icon is smaller.
- Do not make a whole row both toggle and navigate. Chevron is a button with stack-specific accessible name, `aria-expanded` and `aria-controls`; stack/container name is a link/button for inspection. Use semantic nested lists/sections with headings, not an ARIA treegrid unless its entire keyboard model is implemented. Desktop column alignment is visual, not a reason to invent complex grid semantics.
- Tab visits visible controls in reading order; Enter/Space activates disclosure. Hidden children leave the tab order. Escape closes a sheet/inline review and restores its trigger; if removed, restore to owning stack heading, then main heading. Retain skip link and Cmd/Ctrl+K. No single-letter shortcuts in text inputs.
- Focus indicators and 4.5:1 body-text contrast in both themes; icons accompanied by state text. Never communicate health/difference through color alone. Reduced motion suppresses animated progress sheen/spinners without hiding state.
- `aria-live=polite` summarizes phase changes and terminal outcomes, not byte ticks or every clock update. Determinate layer bars expose meaningful current/max only when known; indeterminate bars omit bogus percent. Full digest available to screen readers and copy confirmation announced once.
- Pagination occurs at stack boundaries; expanded children stay with their parent. Initial 20×2 preview is bounded; child expansion pages by 20. Preserve scroll anchors on SSE additions/deletions. Move focus explicitly before removing a currently focused vanished resource.
- Start with bounded pagination, not a virtualization library. Test 250 stacks and a single stack with 250 containers; profile requests/rendering. Server runtime loops currently invoke Compose per stack, so frontend DOM limits alone are insufficient: coalescing, bounded caching and measured backend latency are rollout requirements. Server-side inventory paging/virtualization is a later response to measured limits, not speculative MVP infrastructure.

## 10. Navigation migration and scope boundaries

| Current entry | Proposed destination / behavior |
|---|---|
| `/` Overview | Redirect to `/stacks`, preserving applicable query. Brand now names operational home. Remove redundant Overview nav entry after rollout gates pass |
| `/stacks` table/cards | Nested operational inventory; existing `q`, `status`, `group` mapped; legacy `view=grid` accepted/ignored without error and normalized to the single new layout |
| `/stacks/:path+`, `/stack/:path+` | Keep routes and nested path encoding/redirect. Detail becomes supporting investigation; active work appears above existing tabs; containers use shared observations/identity |
| Detail `tab=overview|containers|configuration|activity|logs` | Preserve links; do not rename tab values in MVP. Return link restores inventory filters/page/disclosure and removes detail-only query parameters |
| `/operations`, `/operations/:id` | Keep names and URLs. Active work and full retained history live here; reuse shared components/actions, not a divergent “other updater” |
| `/repository` | Keep source config, checks and webhook diagnostics; reuse same safe Git-check control as home |
| `/system` | Keep effective environment configuration and capabilities, read-only |
| Fixed global active bar/footer | Replace with inline home/detail work and compact header Work entry on all pages. Full inspector/direct URLs preserved |

### MVP required

Nested actual containers; useful default previews; central existing update/Git actions with scope; keyed live work; reliable reconnect/terminal handling; unified automatically refreshed runtime; local running-image repository identity with explicit uncertainty; retained approvals/deep links; accessibility and 250-resource verification.

### Explicitly later / not part of this plan

Remote registry freshness and “outdated” filtering; selected-stack batch update; per-container mutation; cancellation/rollback; no-prune updater variant; desired-service inventory before creation; multiple repositories/hosts; telemetry dashboard; custom saved views; durable byte/layer history; new auth/role system; wholesale tab or UI-kit rewrite. None is necessary to make the daily workspace substantially more useful.

### Review gate

Tech Lead should review this study and the supporting plan together, especially global prune visibility, Git-check interruption, default two-child preview, removal of Overview, and the cost of honest progress/digest contracts. Approval here is an architectural/design decision, not a claim of usability validation. Implementation begins only after an explicit user `/implement`; this study supplies no application code or runnable prototype.
