# Reconcile Path: Image Pull Progress

## TL;DR
> **Summary**: Add an explicit image pull step with SSE progress reporting to `syncStack()` so initial stack deployments show per-image pull progress, reusing the existing `PullImages`/`image_pull_progress` infrastructure.
> **Estimated Effort**: Short

## Context
### Original Request
When a stack is first deployed via `syncStack()` in the reconcile path, `ComposeUp()` pulls images implicitly with no progress feedback. The scheduler's `updateStack()` already has explicit pulls with SSE progress — we need the same for reconciliation.

### Key Findings
- `Reconciler` struct has no `docker.Client` or image-pulling capability — it only has `ComposeRunner` and `ContainerInspector` interfaces.
- The scheduler wires `PullProgressFn` → `broadcaster.PublishUpdateProgress()` with `type: "image_pull_progress"`, including `stack`, `image`, `status`, `progress`, `current`, `total`.
- The frontend `stacks.ts` store already handles `image_pull_progress` events and populates `pullProgress` state — no frontend changes needed if we emit the same event shape.
- `reconcile.ExtractComposeImages(content []byte)` already exists and is used by the scheduler.
- `NewReconciler()` is called in `main.go:121` and in ~20 test call sites — the signature change must be backward-compatible or all tests updated.
- The `Broadcaster` is available in `main.go` and could be passed to the reconciler.

## Objectives
### Core Objective
Show per-image pull progress during reconcile-path deployments using the existing SSE infrastructure.

### Deliverables
- `ImagePuller` interface in `reconcile` package
- `Reconciler` accepts an optional `ImagePuller` + `Broadcaster`
- `syncStack()` pulls images before `ComposeUp()` with progress callbacks
- All existing tests compile and pass

### Definition of Done
- `go build ./...` succeeds
- `go test ./internal/reconcile/...` passes
- `go test ./...` passes
- Manual test: deploy a new stack, observe `image_pull_progress` events in browser SSE stream

### Guardrails (Must NOT)
- Must not break the scheduler's existing pull progress path
- Must not import `docker` package into `reconcile` — use an interface
- Must not require frontend changes (reuse exact same SSE event shape)
- Must not block reconciliation if puller is nil (graceful fallback to implicit pull)

## Progress

- [x] 1. Define ImagePuller interface
- [x] 2. Add ImagePuller and Broadcaster to Reconciler
- [x] 3. Wire image pull into syncStack()
- [x] 4. Pass dependencies in main.go
- [x] 5. Update tests
- [x] All tests pass
- [x] No regressions

## TODOs

### 1. Define ImagePuller interface
**What**: Add a minimal `ImagePuller` interface to `reconcile/reconcile.go` that wraps only what `syncStack()` needs. Reuse `docker.PullProgress` and `docker.PullProgressFn` types by defining local aliases or a local callback type to avoid importing the `docker` package.
**Files**: `backend/internal/reconcile/reconcile.go`
**Acceptance**: Interface compiles; `go build ./internal/reconcile/...` succeeds

```go
// PullProgress mirrors docker.PullProgress for decoupling.
type PullProgress struct {
    Image    string
    Status   string
    Progress string
    Current  int
    Total    int
}

// PullProgressFn is called with pull progress updates.
type PullProgressFn func(PullProgress)

// ImagePuller pulls container images with progress reporting.
type ImagePuller interface {
    PullImages(ctx context.Context, images []string, onProgress PullProgressFn) error
}
```

### 2. Add ImagePuller and Broadcaster to Reconciler
**What**: Add `imagePuller ImagePuller` and `broadcaster *desiredstate.Broadcaster` fields to `Reconciler`. Add them as **optional** parameters to `NewReconciler()`. Since there are ~20 test call sites, use a functional options pattern or simply add the two new params at the end (both can be nil). Recommendation: add to the end of the parameter list since all call sites are explicit — a simple find/replace adds `, nil, nil` to tests.
**Files**: `backend/internal/reconcile/reconcile.go`
**Acceptance**: `go build ./internal/reconcile/...` succeeds with nil values

### 3. Wire image pull into syncStack()
**What**: In `syncStack()`, after writing temp compose files and before calling `ComposeUp()`:
1. Extract images via `ExtractComposeImages(stack.Content)`
2. If `r.imagePuller != nil` and images are non-empty, build a `PullProgressFn` that publishes `image_pull_progress` events via `r.broadcaster.PublishUpdateProgress()` using the exact same map shape as `scheduler.go:356-368` (but without `cycle_id` — use empty string or omit)
3. Call `r.imagePuller.PullImages(ctx, images, onProgress)`
4. If pull fails, mark stack as failed and return early
5. If `r.imagePuller` is nil, skip — `ComposeUp()` will pull implicitly (existing behavior)
**Files**: `backend/internal/reconcile/reconcile.go`
**Acceptance**: `go build ./...` succeeds; logic is correct by code review

### 4. Pass dependencies in main.go
**What**: In `main.go`, create an adapter that wraps `docker.Client` to satisfy `reconcile.ImagePuller`. The adapter converts `docker.PullProgress` → `reconcile.PullProgress`. Pass the adapter and `broadcaster` to `NewReconciler()`.
**Files**: `backend/cmd/docker-cd/main.go`
**Acceptance**: `go build ./cmd/docker-cd/...` succeeds

Adapter approach (can live in `main.go` or a small file):
```go
type dockerImagePuller struct {
    client *docker.Client
}

func (d *dockerImagePuller) PullImages(ctx context.Context, images []string, onProgress reconcile.PullProgressFn) error {
    var dockerProgress docker.PullProgressFn
    if onProgress != nil {
        dockerProgress = func(p docker.PullProgress) {
            onProgress(reconcile.PullProgress{
                Image: p.Image, Status: p.Status, Progress: p.Progress,
                Current: p.Current, Total: p.Total,
            })
        }
    }
    return d.client.PullImages(ctx, images, dockerProgress)
}
```

### 5. Update tests
**What**: Update all `NewReconciler()` call sites in test files to pass `nil, nil` for the two new parameters. Optionally add one test in `reconcile_test.go` that verifies `syncStack()` calls `ImagePuller.PullImages` when provided.
**Files**: `backend/internal/reconcile/reconcile_test.go`, `backend/internal/scheduler/scheduler_test.go`, `backend/tests/integration/dind_reconcile_test.go`
**Acceptance**: `go test ./...` passes

### Verification
- All tests pass: `go test ./...`
- No regressions: scheduler pull progress still works (same SSE event shape, different origin)
- Manual: deploy a stack that needs image pulls, confirm `image_pull_progress` events appear in browser DevTools SSE stream
