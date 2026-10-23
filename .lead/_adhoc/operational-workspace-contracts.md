# Operational workspace contracts — v2 live work / v1 image identity

Status: task 1 contract freeze, not a claim that the backend implements these APIs.
Authority: design study §8 and implementation plan task 1. The user activated
implementation; the lead owns plan Progress. This change adds types and opt-in
fixtures only: no route, response, mutation, polling or rendering changes.

## 1. Rollout and wire conventions

Canonical client declarations are `frontend/src/services/work.ts` and the additive
types in `services/api.ts`. Backend tasks 2–3 must use **these exact JSON names,
enum values, optionality and semantics**; Go fields need matching JSON tags. Do
not independently rename `sequence` to `watermark`, `run` to `record`, or
`pullAttemptId` to image/layer/source identifiers. Snapshot `sequence` *is* the
watermark. No generic wrapper surrounds the SSE JSON data.

`GET /api/instance` may add these members to its existing `capabilities` object:

```json
{"liveWorkVersion":2,"imageIdentity":{"version":1,"source":"container_image_inspect"}}
```

Each is independently optional. Advertise only after the corresponding server
contract is implemented, not merely because a type exists. Missing/unsupported
version means operation-stage fallback and “Detailed live progress unavailable
on this agent”; missing image identity means older/unavailable agent metadata,
not a failed container. Flags express feature availability, **not authorization**.
The current deployment requires its existing restricted network/reverse-proxy
access boundary; this contract introduces no IAM guarantee.

Wire rules for every table below:

- All fields without `?` are required. Optional fields are **omitted**, not empty
  strings or `null`, unless a table explicitly permits null. Never emit JSON null
  for an array; emit `[]`. Unknown totals/current bytes accept absence or null;
  both mean unknown, never zero. Known zero remains zero, not a percentage.
- Strings used as IDs/epoch are nonempty opaque server-owned values. Paths are
  exact discovered stack paths, not basenames. Times are UTC RFC3339 timestamps;
  timestamps are presentation facts, never ordering/identity keys.
- Counts/bytes/sequence/version/position are nonnegative safe integers
  (`<= Number.MAX_SAFE_INTEGER`); position and version start at 1. No NaN,
  infinity, negative values or numeric strings. Rotate epoch/resnapshot before
  sequence exhaustion; never wrap a sequence within an epoch.
- Messages/status text are safe display text, not raw SDK/Compose/registry errors.
  Messages are bounded to 1,024 UTF-8 bytes with safe text truncation; codes remain
  machine-readable. Never truncate an ID/reference/digest into another identity.
- Unknown future fields are ignored. Unknown schema/enum values, malformed
  ordering/identity fields or inconsistent counts do not authorize mutations or
  create success; the future consumer must reject that live update and restore
  from a supported snapshot. Keep last-known information visibly stale.

Legacy `update.started`, `update.progress`, `update.snapshot`,
`update.completed` and `operation.upsert` payloads stay unchanged. In particular,
legacy `cycle_id` is not renamed to `cycleId`. New work envelopes are additive,
on the existing `/api/events` SSE stream. Legacy bytes never get attributed to a
stack/run by guessing from source/image/layer names. Durable operation history
continues using the existing `Operation` shape, including its `steps`.

## 2. SSE envelopes and barrier

| Field | Owner and meaning | Absence/null/ordering |
|---|---|---|
| `schemaVersion` | Work publisher; literal `2` | Required on both events |
| `epoch` | Publisher lifetime identity, new on server restart/live-state reset | Required, compare only equality, never lexical order |
| `sequence` | Publisher-wide work publication counter; snapshot barrier watermark | Required safe integer, snapshot may begin at 0 |
| `operations` | Publisher; complete changed live summaries on upsert, authoritative active/ancestor set on snapshot | Required array, never a latest-history page |
| `runs` | Snapshot only; all and only currently running attempts | Required array; no retained terminal runs |
| `detailsTruncated` | Snapshot only; any run/image/outcome detail omitted by limits | Required boolean; does not mean summaries are missing |
| `run?` | Upsert only; full replacement of one running **or terminal** attempt | Omitted for operation-only upsert; never null |

SSE event names are `work.snapshot` and `work.upsert`; JSON is the `data` value
of the typed `WorkEvent`. SSE transport `id` remains opaque and is not the work
sequence, operation version or run ID. Reconnect always receives a fresh atomic
snapshot; no durable byte replay or reliance on Last-Event-ID is promised.

The server captures active runs, running operations and required ancestor
summaries, establishes watermark W, and registers delivery while publication is
paused under one coordinated barrier. Deliver snapshot before any queued work
event on that subscription. Publications after the barrier have sequence > W.
No operation-start/end race may leave a snapshot missing an active operation or
its run. Implement this without inverted operation-store/broadcaster lock order.
Overflow closes the subscription to force resnapshot; silently dropping an event
is not allowed.

Within a connection/epoch, accept only `sequence > lastAcceptedSequence` for
upserts. Duplicates, lower sequences and events <= snapshot W are ignored.
Gaps are allowed (this is ordering, not a contiguous delivery-count API).
Apply the entire envelope atomically. Operation summaries have their own durable
`version`; stale versions must not overwrite a newer history record. Legacy
`operation.upsert` may enrich history but cannot establish/remove v2 active
membership. Every live operation change must also publish a work envelope.

A reconnect snapshot replaces active membership, **not an append-only merge**.
An epoch is adopted only from the subscription's initial snapshot, never an
unsolicited upsert. Before that barrier, do not apply work upserts. A new epoch
discards old layer caches; a delayed old-connection event must not switch it back.
Request/connection generations must distinguish overlapping reconnects. Keep
mutations unavailable while disconnected/restoring authoritative state.

## 3. Run, image and layer records

| Run field | Owner / meaning | Absence rule |
|---|---|---|
| `runId` | Execution boundary allocates unique attempt ID; key `(epoch, runId)` | Required, never reused, even for retry on same stack |
| `operationId` | Durable operation that owns this attempt | Required, operation summary accompanies first publication |
| `rootOperationId` | Server-resolved workflow root, can be a completed ancestor | Required; never infer from matching stacks/times |
| `parentOperationId?` | Immediate parent of **owner operation**, not previous run | Omit when owner has no parent |
| `source` | Execution owner: `scheduler`, `reconcile`, `refresh` | Required, contextual, never a key |
| `stack?`, `revision?`, `cycleId?` | Exact target, reviewed/observed revision and legacy cycle context when known | Omit when not applicable/unknown; prune has no stack |
| `status` | `running`, `completed`, `failed`, `cancelled` | Required; terminal immutable |
| `phase` | Running: `waiting`, `checking_repository`, `pulling`, `applying`, `pruning`; terminal: `finished` | Required, no fabricated waiting phase |
| `startedAt`, `updatedAt`, `finishedAt?` | Server boundary/update/end timestamps | First two required; finishedAt omitted while running, required terminal |
| `message`, `errorCode?` | Sanitized display text and safe reason token | Message required (may be empty), code omitted when no reason |
| `imageCount`, `images` | Number of known image attempts in this run before truncation, retained detail array | Required; count >= array length; no images is valid |
| `stackResults?` | Scheduler's aggregate and bounded outcome references | Omit for non-global work or before results exist |
| `detailsTruncated` | This run lost any images/layers/outcomes to a bound | Required; false implies complete detail for known data |

`updatedAt >= startedAt`; terminal `finishedAt >= startedAt`. A run ends when its
owned execution returns, not because a layer is done, an SSE connection closes,
an operation disappears or a parent finishes. Terminal status never regresses or
changes outcome under the same ID; a retry allocates a new run. Timestamps don't
override envelope ordering. Lifecycle ownership:

| Execution boundary | Starts / ends | Durable owner and grouping |
|---|---|---|
| Scheduler individual stack pull | Each actual pull invocation / its return, including error or context cancellation | Global image-update operation; stack present, root global update. Each retry gets a fresh run and pull-attempt ID |
| Reconcile child deployment | Child deployment execution starts / ends after pull + Compose apply or failure/cancellation | Deployment child operation and its true root; phases pulling → applying. A scheduler pull may already have completed, without completing the workflow |
| Refresh | Repository check execution starts / returns | Refresh operation; no fabricated image/layer data; children can outlive the completed refresh |
| Scheduler prune | Actual host-wide prune starts / returns | Global image-update operation, no stack, imageCount 0. Root remains active until remaining work/prune ends |
| Accepted action waiting on lock; standalone check without instrumented run | Operation start/update/end | Operation-only work envelopes are sufficient; do not fabricate a run merely from a browser POST |

These boundaries instrument existing execution, not new parallelism, retries,
locks, cancellation APIs or changed scheduler/reconciliation policy.

| Image field | Meaning and rules |
|---|---|
| `pullAttemptId` | Required fresh server ID for this image pull invocation, keyed beneath run; a retry is never a reuse |
| `reference` | Required configured pull reference; not an image identity/registry-freshness claim |
| `status`, `done` | Required WorkStatus and boolean. done means invocation ended, including failed/cancelled; true iff status terminal |
| `position?`, `total?` | Known one-based image position and known total, not bytes; omit position if unknown, total null/absent unknown; position <= known total |
| `layerCount`, `layers` | Required known distinct layer count before truncation and retained array; count >= array length |
| `detailsTruncated` | Required; layer omissions declared here and propagated to run/snapshot |

| Layer field | Meaning and rules |
|---|---|
| `id` | Required opaque Docker layer identifier; unique only within `(epoch, runId, pullAttemptId)` |
| `phase` | Required normalized `waiting`, `downloading`, `extracting`, `finished`, `unknown` |
| `status` | Required sanitized Docker status text; not the work lifecycle enum |
| `bytesCurrent?`, `bytesTotal?` | Phase-local byte values; absence/null unknown. Never sum download and extraction, or divide by unknown/zero total. A phase change may reset current; done does not prove known bytes total |
| `done`, `updatedAt` | Required layer completion observation and server update time; a done layer cannot complete its image/run/workflow by inference |

Each upsert replaces a run including nested arrays; omitted former layers/images
are removed, not merged. Duplicated layer IDs in different images/runs are valid;
duplicate IDs within the same scope are invalid. Pull completion cannot be
presented as “100% deployed.” No invented aggregate denominator across phases.

`stackResults` has required `processed`, `skipped`, `failed`, `outcomeCount`,
`outcomes`, `detailsTruncated`. Processed counts attempted stacks including failed;
skipped is separate and failed <= processed. These are observed results, not the
number of discovered/eligible stacks. Outcome entries contain required `stack`,
`status` (`completed`/`skipped`/`failed`), safe `message`, optional child
`operationId`. `outcomeCount` is the number of known references before truncation,
not a discovered-stack denominator. No container-level mutation is implied.

### Fixed detail budgets and retention

- At most **200 retained layers per image attempt**, **32 retained image attempts
  per run**, **100 outcome references per run**. Retain a stable ID order when
  selecting details; never use truncation to manufacture healthy/completed state.
- **2 MiB (2,097,152 bytes) serialized detail budget** per snapshot/upsert and
  globally in the server's current live-detail cache. Precisely: sum UTF-8 lengths
  of compact JSON encoding of each run's `images` array and each present
  `stackResults.outcomes` array (including each array's `[]` bytes). Remove layers,
  then images/outcomes until budget fits; omitted detail is not retained in an
  unbounded shadow cache. Mandatory run/operation summaries and counts are never
  dropped. This is a *detail* budget, not a promise that arbitrarily many active
  summaries fit in a 2 MiB total HTTP/SSE body. No pagination of active summaries.
- `detailsTruncated` is true at every affected containing level. Counts preserve
  pre-truncation known cardinalities. False with shorter arrays than known counts
  is invalid. Unknown future discovery is not truncation. An operation-only event
  with no detail has no extra truncation field.
- Coalesce byte-only changes per run to **at most one delivery per 500 ms**; the
  latest full replacement wins. Flush phase changes, image terminal changes,
  operation metadata changes and run terminal transitions immediately. Terminal
  flush cancels pending byte ticks for that attempt.
- Release a terminal run's server live layers after immediate delivery. Snapshots
  contain no terminal attempts. Client may retain at most 50 terminal run summaries
  for 60 seconds for transient feedback, without layer arrays; durable inspection
  uses operation history and says “Live pull detail is no longer retained.”
- Former active runs omitted by reconnect: discard bytes, show “No longer active;
  retrieving outcome,” query owner by ID. **Snapshot omission is not proof of
  success**. HTTP 404 means “Outcome unavailable or expired”; transport failure
  means “Outcome lookup failed”; neither completes a run. Existing persisted
  operation restart behavior records `interrupted`, not synthetic completion.

## 4. Active operations and ancestry independent of history pages

`WorkOperation` is a **complete live summary**, not the durable `Operation` record:
required `version`, `id`, `kind`, `trigger`, `status`, `message`, `startedAt`;
optional `parentId`, `rootOperationId`, `stack`, `revision`, `finishedAt`.
There is intentionally **no `steps` field** in live summaries; use durable ID
lookup for steps. Status preserves existing operation-store strings (including
`running`, `completed_with_errors`, `interrupted`), not the narrower run enum.
FinishedAt is omitted while running and present when terminal. Version is the
same monotonic record version used by durable history, not envelope sequence.

Snapshots include every running operation, each active run's owner, and all
available ancestors needed to reach its root, even completed parents and records
older than the latest 50. Keep needed summaries independently of bounded retained
history. Upserts include changed operation summaries; first reference to a new
owner includes its owner/ancestor chain, so clients can group without history.
If ancestry genuinely cannot be recovered, omit summary `rootOperationId`, keep
known parentId, and show “Grouping unavailable”; missing root is not proof of a
standalone root. A run's known root ID may still be displayed with an unavailable
ancestor record, but do not invent ancestor metadata. Operation-only envelopes
must contain at least one summary.

An active workflow is a unique authoritative root with any running operation,
running descendant or running run. A completed root with an active child counts
once. A completed pull with a running root remains active. Ended child operations
do not end sibling work. No hidden filter may exclude active workflow summaries.
Terminal operation summaries leave active membership unless needed as ancestors;
they may remain in durable/history state. Latest-50 history is never the bootstrap
authority. `GET /api/operations/:id` resolves absent summaries; 404 explicitly
means not found/expired. Task 2 adds exact immediate `parentId` filtering to the
existing cursor-paginated `/api/operations` (combined with existing filters,
`limit` default 50/max 100); it is not recursive descendants and does not replace
the snapshot. ID lookup/failed lookup is not permission to guess grouping.

## 5. Local running-image identity (v1)

`ContainerInfo.imageIdentity?` is additive on both existing runtime and
stack-container responses. Old `id` (public short-ID compatibility), `image`,
state/health/ports remain unchanged. The backend retains the full enumerated
container ID internally, verifies discovered-stack ownership/Compose labels,
inspects that exact container, then inspects **its immutable image ID**. Never
resolve identity using a mutable tag, short ID, replacement name or browser input.

| Field | Server ownership / absence |
|---|---|
| `status` | Required `available`, `unavailable`, `ambiguous`: repository identity availability, not container health |
| `observedAt` | Required time this identity resolution was attempted; cached results retain actual observation time |
| `source` | Required literal `container_image_inspect`; local daemon metadata only |
| `imageId?` | Exact immutable ID from inspected container, labeled “Local image ID”; omit if not safely known |
| `reference` | Required configured image reference from enumeration/inspection, not newly resolved tag |
| `platform?` | When safely known, object with required `os`, `architecture`, optional `variant`; omit rather than invent platform |
| `repositoryDigests` | Required array of full `{repository,digest,kind}` records returned by local image inspection; preserve all reported identities, deduplicate exact tuples |
| `preferredRepositoryDigest?` | Exactly one member matching normalized configured repository; omit for no match or ambiguity |
| `reason?` | Safe enum below; required on unavailable/ambiguous, omitted on available |
| `freshness` | Required literal `not_checked`, displayed “Registry freshness: Not checked” |

Each repository digest contains required normalized repository name, full digest
including algorithm prefix, and `kind: index | manifest | unknown`. **Digest kind
may be unknown**; platform does not prove kind. On the currently available SDK,
RepoDigests alone supply no proof of index/manifest: use `unknown`. Never relabel
a local image ID as repository digest, shorten a copied digest, or claim a tag is
latest/outdated. Normalize repositories with the existing distribution-reference
parser; `postgres:17` matches `docker.io/library/postgres`, not a similarly named
different registry/path. Compare repository, not mutable tag or time.

Resolution outcomes (canonical executable fixtures use these exact shapes):

| Outcome | Status / reason | Preferred |
|---|---|---|
| Unique normalized repository match, even among other repositories | available / omitted | Exact matching tuple, unknown kind allowed |
| No RepoDigests / local build | unavailable / not_recorded | omitted; imageId/platform may still exist |
| Digests exist but none match configured repository | unavailable / not_recorded | omitted; preserve alternatives |
| Multiple distinct plausible matches for configured repository | ambiguous / repository_ambiguous | omitted; preserve alternatives |
| Container disappeared/replaced before exact inspection | unavailable / container_replaced | omitted; never follow name to replacement |
| Image disappeared between calls | unavailable / image_missing | omitted; imageId may remain if safely obtained |
| Inspection denied | unavailable / inspection_denied | omitted |
| Inspector unsupported | unavailable / unsupported | omitted |
| Stack/container ownership unverified | unavailable / ownership_unverified | omitted; no arbitrary host inspection |
| Other sanitized inspection failure/timeout | unavailable / inspection_failed | omitted |

Absent imageIdentity is older-server absence, not the unavailable object. Fields
inside an emitted object are not null; omit unknown metadata. Only whitelisted
reference/identity/platform fields may cross the wire. No env, mounts, raw labels,
raw inspect JSON, SDK errors, registry credentials or auth data. No arbitrary
inspection endpoint, registry call, pull, prune or Compose mutation is authorized.
Remote freshness checks, TTLs, checkedAt, outdated-image UI and registry access
remain **separate, excluded scope**; the fixed `not_checked` value is not a stub
implementation of that feature.

## 6. Shared runtime observation contract

`GET /api/runtime` stays a map of discovered exact stack paths to observations:
required `containers: ContainerInfo[]`, `error: string | null`,
`observedAt: string | null`; optional additive `attemptedAt: string`.
`RuntimeObservation` in api.ts is the wire DTO, not the existing store's client
loading/stale state. Stack-container GET remains `ContainerInfo[]`, not a new
wrapper. Both readers use the same server-enriched observations.

- On success, error null and observedAt is the actual last successful container
  listing time, even for an empty listing. No known observation is observedAt
  null, never a fabricated fresh time. Older backends may omit timing metadata;
  consumers treat that as unknown provenance, not healthy/fresh.
- Partial failure is per-stack: keep last successful containers and observedAt,
  set a safe error, and attemptedAt to failed attempt time. With no success yet,
  containers [] + observedAt null + error does **not** mean no containers.
  A transport failure is separate client error; retain last-known rows. Identity
  failure only affects imageIdentity; it must not erase a valid container listing.
- Cache reuse does not update observedAt, attemptedAt or identity observedAt.
  Share/coalesce observation cycles (15-second target) across clients; bound
  inspect concurrency at four and cache image inspection by immutable image ID,
  not tag. Enforce cancellation/timeouts. Terminal deployment/reconnect invalidates
  relevant cached observations; no overlapping unbounded Compose fan-out.
- Frontend later shares one visible-page 15-second timer, hidden pause, immediate
  return/reconnect observation and debounced stack/terminal invalidation. Mark
  stale at 45 seconds since actual successful observation while visible. Request
  generations prevent a slower old poll overwriting newer targeted reads.
  SSE connectivity is not runtime freshness. An absent health check is not failure.

## 7. Executable examples and expected state

`operationalWorkspaceFixtures()` in `tests/e2e/fixtures/frontend-data.ts` is the
opt-in typed payload catalogue. It does not change default fixture responses,
advertise capabilities, install routes or consume SSE. Times/IDs are fixed and
JSON serializable. `frontend/tests/work-contracts.spec.ts` checks structure,
scope, bounds, identity selection and unchanged legacy/default fixture behavior.

Every row below is applied after the preceding row unless called independent.
Expected state deliberately distinguishes transient terminal feedback from active
membership. All events have schemaVersion 2. First epoch is `fixture-agent-a`.

| Payload | Expected state after event |
|---|---|
| `snapshot`, W=10 | Active runs run-deployment-1 + run-stack-pull-1; two roots old-refresh + global-update. old-refresh is already completed, but its old-deployment child is active. Neither old operation appears in latest50; both appear in snapshot |
| `operationOnly`, seq=11 | Adds accepted-action root waiting on execution lock with **no run**; three workflows, not a fake pull |
| `checkOnly`, seq=12 | Adds check-only refresh root with no image work; four workflows |
| `bytes`, seq=13 | Full child run replacement: shared-layer current 256, total null. Scheduler's identical image/layer remains at 128; no known percentage |
| duplicate seq=13 or delayed seq=9 from same epoch | Ignore completely; no bytes/status regression |
| `terminal`, seq=14 | run-stack-pull-1 ends completed, transient summary only; child run remains active and global-update operation remains running, still four workflows, not “deployment completed” |
| `pruning`, seq=15 | Adds run-prune-1 under global-update without stack/images. Results attempted 2 (1 failed), skipped 1; root count unchanged |
| `retry`, seq=16 | New run-stack-pull-2 + new image attempt, same stack/image/cycle/layer permitted; terminal run-stack-pull-1 is not resurrected. Fixture represents identity isolation, not permission for new execution concurrency |
| `reconnect`, W=20, empty | Clears all active membership/bytes. Previously active runs need owner lookup; outcomes unknown, **not successful**. Transient terminal summaries are not active runs |
| `restart`, epoch fixture-agent-b, W=0 | Clears prior epoch caches despite lower sequence. Owner lookup can return interruptedOperation; show interrupted, not completed. Late epoch-a upsert ignored |
| `truncated`, independent epoch-a W=21 snapshot | All active summaries preserved; child imageCount 40 with images [], truncation true. Other run still has detail; no fabricated denominator |

Concrete example operation-only JSON (no `run: null`):

```json
{"schemaVersion":2,"epoch":"fixture-agent-a","sequence":11,"operations":[{"id":"accepted-action","rootOperationId":"accepted-action","version":1,"kind":"action","trigger":"retry","stack":"tools/linkding","revision":"a81bc927","status":"running","message":"Waiting for execution lock","startedAt":"2026-10-23T12:00:01.000Z"}]}
```

Concrete restart barrier (empty arrays are authoritative active membership, not
terminal history):

```json
{"schemaVersion":2,"epoch":"fixture-agent-b","sequence":0,"runs":[],"operations":[],"detailsTruncated":false}
```

The catalogue's `identities` examples cover unique unknown-kind digest, multiple
repositories with one preferred, no matching repository, local build, ambiguity,
missing image, disappeared container, denied/unsupported/unowned inspection.
Every example says freshness not_checked. A pulled newer tag must not change an
old running container's identity; task 3 and real Docker task 9 verify resolution,
not these fixture-only examples.
