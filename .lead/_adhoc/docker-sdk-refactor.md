# Refactor Docker CLI Shelling to Docker Go SDK

## TL;DR
> **Summary**: Replace `os/exec` CLI shelling for image and container operations with the Docker Go SDK (`github.com/docker/docker/client`), replace `docker compose config --images` with direct YAML parsing, and wire streaming image pull progress through SSE to the frontend — eliminating all `CommandRunner` usage from `docker.Client`.
> **Estimated Effort**: Large

## Context
### Original Request
Replace Docker CLI shelling (`docker ps`, `docker inspect`, `docker image prune`, `docker compose pull`, `docker compose config --images`) with the Docker Go SDK and YAML parsing for image operations, container listing, and container inspection. Keep `docker compose` CLI only for `up`/`down`/`ps` (compose orchestration). Wire streaming pull progress to the existing SSE broadcaster so the frontend shows per-image download status.

### Key Findings
- **`docker.Client`** (client.go) wraps a `CommandRunner` and socket string. Methods: `ContainerCount`, `ListContainersWithLabel`, `PullImages`, `PruneImages`, `GetImageDigest`, `GetComposeImages`.
- **`CommandRunner`** interface (runner.go) is a simple `Run(ctx, name, args...) ([]byte, error)`. Used by both `docker.Client` and `reconcile.DockerComposeRunner`.
- **`ImageOperations`** interface (images.go) defines `PullImages`, `PruneImages`, `GetImageDigest`, `GetComposeImages` — but nothing consumes it via the interface; `scheduler.go` uses `*docker.Client` directly.
- **`DockerContainerInspector`** (metadata.go) wraps `*docker.Client` for `GetStackLabels` → calls `ListContainersWithLabel`.
- **`DockerComposeRunner`** (compose.go) uses `CommandRunner` for `ComposeUp`, `ComposeDown`, `ComposePs`. These **stay as CLI**.
- **`PullImages`** currently calls `docker compose pull` — the SDK replacement should pull each image individually via `client.ImagePull()`, enabling streaming JSON progress.
- **`GetComposeImages`** calls `docker compose config --images`. This can be replaced by YAML parsing since `stack.Content` (raw compose bytes) is already available on `StackRecord`. The codebase has `extractServiceNames()` in `reconcile/compose.go` as a pattern for lightweight YAML line parsing.
- **`updateStack`** in scheduler.go already has access to `stack` (`desiredstate.StackRecord`) which carries `.Content` — but currently ignores it and reads from the filesystem path. The image list can be extracted from `.Content` directly.
- **`go.mod`** already has `github.com/docker/docker v28.5.2` as an indirect dep (from testcontainers). Promoting to direct is trivial.
- **34 unit tests** use `stubRunner`/`multiStubRunner` to mock CLI output. SDK-based methods need a new mockable interface.
- **SSE**: `Broadcaster.PublishUpdateProgress` already sends `update.progress` events with `type` field (`started`, `stack_progress`, `stack_success`, `stack_error`, `completed`, `pruning`). Frontend `stacks.ts` stores `updateProgress` and checks `progress.type` for `started`/`completed`. A new `image_pull_progress` type will pass through transparently — frontend just needs to handle the new type.
- The SDK's `client.NewClientWithOpts()` accepts `client.WithHost()` for socket configuration — maps directly to the current `Socket` field.

## Objectives
### Core Objective
Replace all CLI-shelled Docker operations in `docker.Client` with SDK calls and YAML parsing, enable streaming pull progress piped to SSE, and fully remove `CommandRunner` from the image/container path.

### Deliverables
- A `DockerAPI` interface abstracting SDK operations (mockable for tests)
- SDK-based `ContainerCount`, `ListContainersWithLabel`, `PullImages`, `PruneImages`, `GetImageDigest`
- `ExtractComposeImages()` function replacing `GetComposeImages` CLI call
- Streaming progress on `PullImages` piped through scheduler to SSE broadcaster
- Frontend handling of `image_pull_progress` events
- Updated tests using mock `DockerAPI` instead of `stubRunner` for SDK-migrated methods
- `CommandRunner` fully removed from `docker.Client` (retained only for `DockerComposeRunner`)

### Definition of Done
- `cd backend && go test ./...` passes
- `PullImages` accepts a progress callback and streams per-image progress (verified by test with mock reader)
- `GetComposeImages` is removed from `docker.Client`; image extraction uses YAML parsing
- No remaining `CommandRunner` usage in `docker.Client`
- `DockerComposeRunner` still uses `CommandRunner` for `ComposeUp`/`ComposeDown`/`ComposePs`
- SSE emits `image_pull_progress` events during image pulls
- Frontend displays per-image pull progress

### Guardrails (Must NOT)
- Change the `ComposeRunner` interface contract
- Break existing test assertions (observable outcomes unchanged)
- Change status flow: missing→syncing→synced/failed
- Change reconciliation or drift detection logic
- Break existing SSE event types (`started`, `stack_progress`, `stack_success`, `stack_error`, `completed`, `pruning`) — new type is additive
- Attempt to parse `build:` directives as pullable images — services with `build:` and no `image:` must be skipped

## Progress

- [x] 1. Define DockerAPI Interface and SDK Client
- [x] 2. Migrate ContainerCount to SDK
- [x] 3. Migrate ListContainersWithLabel to SDK
- [x] 4. Migrate GetImageDigest to SDK
- [x] 5. Migrate PruneImages to SDK
- [x] 6. Extract Compose Images via YAML Parsing
- [x] 7. Migrate PullImages to SDK with Progress Callback
- [x] 8. Wire Pull Progress to SSE Broadcaster
- [x] 9. Update Scheduler and Inspector to Use New Client
- [x] 10. Remove CommandRunner from docker.Client
- [x] All tests pass
- [x] No regressions

## TODOs

### 1. Define DockerAPI Interface and SDK Client
**What**: Create a `DockerAPI` interface in `internal/docker/` that wraps the subset of `github.com/docker/docker/client` methods we need. Create an SDK-backed implementation and a constructor that accepts the socket string. Promote `docker/docker` to a direct dependency in `go.mod`.

The interface should wrap exactly these SDK methods:
```go
type DockerAPI interface {
    ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error)
    ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
    ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
    ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
    ImagesPrune(ctx context.Context, pruneFilters filters.Args) (image.PruneReport, error)
}
```

Create `NewSDKClient(socket string) (DockerAPI, error)` that calls `client.NewClientWithOpts(client.WithHost(...), client.WithAPIVersionNegotiation())`. Handle the socket→host URL conversion (reuse `HostArgs` logic: bare path → `unix:///path`, already-prefixed → pass through, empty → use `client.FromEnv`).

**Files**: `internal/docker/sdk.go` (new), `go.mod`, `go.sum`
**Acceptance**: `go build ./...` succeeds; new file compiles; `DockerAPI` interface exists and is satisfied by `*client.Client`

### 2. Migrate ContainerCount to SDK
**What**: Rewrite `Client.ContainerCount` to use `DockerAPI.ContainerList` with `filters.NewArgs()` and count the results instead of parsing `docker ps -q` output. Add `DockerAPI` field to `Client` struct alongside existing `Runner`. Update `NewClient` to accept `DockerAPI` (or add a setter/alternate constructor).

Update tests: replace `stubRunner` with a mock `DockerAPI` that returns `[]types.Container` slices.

**Files**: `internal/docker/client.go`, `internal/docker/client_test.go`
**Acceptance**: `go test ./internal/docker/...` passes; `ContainerCount` no longer calls `Runner.Run`

### 3. Migrate ListContainersWithLabel to SDK
**What**: Rewrite `Client.ListContainersWithLabel` to use `DockerAPI.ContainerList` with label filter (`filters.NewArgs(filters.Arg("label", labelKey))`). The SDK returns `types.Container` with `.ID`, `.Names`, `.Labels` — no need for the two-step ps+inspect approach.

Update tests to use mock `DockerAPI`.

**Files**: `internal/docker/client.go`, `internal/docker/client_test.go`
**Acceptance**: `go test ./internal/docker/...` passes; same `[]ContainerLabels` output shape

### 4. Migrate GetImageDigest to SDK
**What**: Rewrite `Client.GetImageDigest` to use `DockerAPI.ImageInspectWithRaw`. Return `inspect.ID` (which is the `sha256:...` digest).

Update tests.

**Files**: `internal/docker/images.go`, `internal/docker/client_test.go`
**Acceptance**: `go test ./internal/docker/...` passes; returns same digest format

### 5. Migrate PruneImages to SDK
**What**: Rewrite `Client.PruneImages` to use `DockerAPI.ImagesPrune` with `filters.NewArgs(filters.Arg("dangling", "false"))` (equivalent to `-a`). The SDK returns `image.PruneReport` with `.ImagesDeleted` (count) and `.SpaceReclaimed` (uint64 bytes). Convert bytes to human-readable string to match current return signature.

Update tests.

**Files**: `internal/docker/images.go`, `internal/docker/client_test.go`
**Acceptance**: `go test ./internal/docker/...` passes; same `(int, string, error)` signature

### 6. Extract Compose Images via YAML Parsing
**What**: Create an `ExtractComposeImages(content []byte) []string` function that parses raw compose YAML to extract the `image:` value for each service. Follow the same lightweight line-parsing approach as `extractServiceNames()` in `reconcile/compose.go`.

The parser should:
1. Find each service block under `services:`
2. Within each service block, look for an `image:` key
3. Skip services that have `build:` but no `image:` (these are built locally, not pulled)
4. Return deduplicated image names (multiple services can reference the same image)
5. Handle quoted and unquoted values, trailing whitespace, comments

Place this in `internal/reconcile/compose.go` next to `extractServiceNames()` since it follows the same pattern and operates on the same compose content.

Remove `GetComposeImages` from `docker.Client` and the `ImageOperations` interface. Update `updateStack` in scheduler.go to call `ExtractComposeImages(stack.Content)` instead of `s.dockerClient.GetComposeImages(ctx, ...)` — this eliminates the need for filesystem access and `CommandRunner` in the image discovery path.

**Files**: `internal/reconcile/compose.go`, `internal/reconcile/extract_test.go`, `internal/docker/images.go` (remove `GetComposeImages`), `internal/scheduler/scheduler.go` (update `updateStack`)
**Acceptance**: `go test ./internal/reconcile/...` passes with new tests covering: single image, multiple services, `build:`-only services skipped, `build:` + `image:` included, deduplication, empty content, quoted values. `go test ./internal/scheduler/...` passes. `GetComposeImages` no longer exists on `Client`.

### 7. Migrate PullImages to SDK with Progress Callback
**What**: Rewrite `Client.PullImages` to pull individual images via the SDK. Change the signature to accept an image list and a progress callback:

```go
type PullProgress struct {
    Image    string `json:"image"`
    Status   string `json:"status"`
    Progress string `json:"progress"`   // e.g. "[===>     ] 12MB/45MB"
    Current  int    `json:"current"`    // image index (1-based)
    Total    int    `json:"total"`      // total images
}

type PullProgressFn func(PullProgress)

func (c *Client) PullImages(ctx context.Context, images []string, onProgress PullProgressFn) error
```

For each image:
1. Call `DockerAPI.ImagePull` which returns `io.ReadCloser`
2. Decode the JSON stream line-by-line (`{"status":"Pulling...","progress":"[===>  ]","id":"layer-id"}`)
3. Invoke `onProgress` with coalesced status (throttle to ~1 call per 500ms per image to avoid flooding SSE)
4. Read to EOF to ensure pull completes
5. On error, wrap with image name context

If `onProgress` is nil, progress is silently consumed (drain to EOF).

Remove the old `PullImages(ctx, projectName, composeFile, workDir)` signature. Update `ImageOperations` interface accordingly. Update `updateStack` in scheduler.go to pass the image list (from Task 6) and a progress callback (wired in Task 8).

**Files**: `internal/docker/images.go`, `internal/docker/client_test.go`, `internal/scheduler/scheduler.go`
**Acceptance**: `go test ./internal/docker/...` passes; mock `DockerAPI.ImagePull` returns `bytes.Reader` with sample JSON lines; progress callback receives expected events; pull reads stream to completion

### 8. Wire Pull Progress to SSE Broadcaster
**What**: In `scheduler.go`'s `updateStack`, construct a `PullProgressFn` that calls `s.broadcaster.PublishUpdateProgress` with a new event sub-type `image_pull_progress`. The payload structure:

```json
{
  "type": "image_pull_progress",
  "cycle_id": "...",
  "stack": "app1",
  "image": "nginx:latest",
  "status": "Downloading",
  "progress": "[===>     ] 12MB/45MB",
  "current": 1,
  "total": 3,
  "timestamp": "..."
}
```

This uses the existing `EventUpdateProgress` event type and `PublishUpdateProgress` method — no changes to the broadcaster needed. The frontend needs to handle the new `image_pull_progress` type in `stacks.ts`.

On the frontend side, update `stacks.ts` to store `image_pull_progress` events (they pass through `onUpdateProgress` already). Add display logic in the update progress UI component to show the current image being pulled and its download progress.

**Files**: `internal/scheduler/scheduler.go`, `frontend/src/store/stacks.ts`
**Acceptance**: `go test ./internal/scheduler/...` passes; when `PullImages` runs, SSE emits `image_pull_progress` events; frontend store handles the new event type without errors

### 9. Update Scheduler and Inspector to Use New Client
**What**: Update `scheduler.go` to work with the refactored `docker.Client` (which now uses `DockerAPI` internally). Key changes:
- `updateStack` now calls `reconcile.ExtractComposeImages(stack.Content)` instead of `s.dockerClient.GetComposeImages`
- `updateStack` passes image list + progress callback to `s.dockerClient.PullImages(ctx, images, onProgress)`
- Verify `DockerContainerInspector` works with the updated `Client` (should be transparent since `ListContainersWithLabel` keeps the same signature)

Wire up SDK client creation in `main.go` or wherever `docker.NewClient` is called: create the `DockerAPI` via `NewSDKClient(socket)` and pass it to `NewClient`.

Verify `scheduler_test.go` and `updater_test.go` still pass — these mock at the `ImageOperations`/`ComposeRunner`/`ContainerInspector` interface level. The `ImageOperations` interface needs updating to match the new `PullImages` signature, and test mocks need to follow.

**Files**: `internal/reconcile/metadata.go`, `internal/scheduler/scheduler.go`, `internal/scheduler/scheduler_test.go`, `internal/scheduler/updater_test.go`, `cmd/server/main.go` (or equivalent entrypoint)
**Acceptance**: `go test ./...` passes; application starts and connects to Docker

### 10. Remove CommandRunner from docker.Client
**What**: Remove the `Runner CommandRunner` field and `Socket string` field from `docker.Client` since all its methods now use `DockerAPI`. Remove `GetComposeImages` (already done in Task 6) and the `HostArgs` function from `client.go` — move `HostArgs` to `reconcile/compose.go` where `DockerComposeRunner` still needs it, or keep it in `runner.go`.

`CommandRunner` stays in `runner.go` for `DockerComposeRunner`'s use only. Update `NewClient` to accept only `DockerAPI`. Update all callers.

Audit completeness:
- `grep -r 'c\.Runner' internal/docker/` returns nothing
- `grep -r 'CommandRunner' internal/docker/client.go` returns nothing
- `HostArgs` only referenced from `reconcile/compose.go`

**Files**: `internal/docker/client.go`, `internal/docker/runner.go` (keep file), `internal/reconcile/compose.go` (import `HostArgs` or inline), all callers of `NewClient`
**Acceptance**: `go test ./...` passes; `Client` struct has no `Runner` field; `CommandRunner` not imported in `client.go`

### Verification
- All tests pass: `cd backend && go test ./...`
- No regressions: all 34 existing unit tests pass with equivalent assertions (some rewritten for mock `DockerAPI`)
- `PullImages` streams progress via callback (test with mock `io.ReadCloser`)
- `docker.Client` has zero `CommandRunner` usage
- `GetComposeImages` CLI call eliminated; YAML parsing used instead
- `DockerComposeRunner` still uses `CommandRunner` for compose CLI ops
- SSE emits `image_pull_progress` events during pulls
- Frontend handles new event type
- Application builds: `cd backend && go build ./...`
