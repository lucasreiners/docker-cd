# Pre-SDK-Refactor Unit Tests

## TL;DR
> **Summary**: Add unit tests to lock down observable CLI behaviors (args built, status transitions, JSON parsing, error handling) before replacing Docker CLI shelling with the Go SDK.
> **Estimated Effort**: Medium

## Context
### Original Request
Before refactoring from Docker CLI (`CommandRunner`) to the Docker Go SDK, we need unit tests that capture *what* each function does — the args it passes, the status transitions it triggers, and the errors it returns — so the SDK replacement can be verified against identical observable behavior.

### Key Findings
- `DockerComposeRunner` methods (`ComposeUp`, `ComposeDown`, `ComposePs`) build CLI args via `docker.HostArgs()` and call `runner.Run()`. Tests need an `argCapturingRunner` (pattern exists in `docker/client_test.go`).
- `reconcile_test.go` already has a `stubComposeRunner` that records calls at the `ComposeRunner` interface level — good for reconciler tests but doesn't capture raw CLI args. A new `compose_test.go` in the `reconcile` package (internal, not `_test`) needs an arg-capturing `CommandRunner` stub.
- `generateLabelOverride` and `generateLabelArgs` are unexported but `export_test_helpers.go` already exposes `TestGenerateLabelOverride`. `generateLabelArgs` needs a similar export or tests in the `reconcile` package (not `_test`).
- `CancelActive()` and `CancelActiveUpdate()` use mutex-guarded `context.CancelFunc` — testable by starting a reconcile/update in a goroutine and verifying context cancellation.
- `UpdateContainerCounts()` calls `ComposePs` then writes counts to the store — fully testable with the existing `stubComposeRunner` pattern extended to return container data.
- `AllLabelKeys()` is a pure function — trivial to test.
- Test style: manual `if` checks with `t.Errorf`/`t.Fatalf`, no testify. Table-driven where useful.

## Objectives
### Core Objective
Create a regression-safe test suite that captures the observable contract of every function that will be touched during the SDK migration.

### Deliverables
- `internal/reconcile/compose_test.go` — ComposeUp/Down/Ps arg + behavior tests
- `internal/reconcile/labels_test.go` — label generation tests
- `internal/reconcile/state_manager_test.go` — UpdateContainerCounts tests
- Extended `internal/reconcile/reconcile_test.go` — CancelActive test
- Extended `internal/scheduler/scheduler_test.go` — CancelActiveUpdate test

### Definition of Done
- `cd backend && go test ./internal/reconcile/... ./internal/scheduler/... -count=1` passes
- Every function listed in scope has ≥1 test covering its happy path and ≥1 covering its error path

### Guardrails (Must NOT)
- Must NOT use testify or any external test dependency
- Must NOT require Docker-in-Docker or build tags
- Must NOT test implementation details that will change (e.g. exact temp dir paths)
- Must NOT duplicate existing test coverage in `reconcile_test.go`

## Progress

- [x] 1. ComposeUp arg-capture tests
- [x] 2. ComposeDown arg-capture tests
- [x] 3. ComposePs JSON parsing tests
- [x] 4. generateLabelArgs + generateLabelOverride tests
- [x] 5. CancelActive context cancellation test
- [x] 6. CancelActiveUpdate cancellation test
- [x] 7. UpdateContainerCounts store update test
- [x] 8. AllLabelKeys completeness test
- [x] All tests pass
- [x] No regressions

## TODOs

### 1. ComposeUp arg-capture tests
**What**: Create an `argCapturingRunner` (implements `docker.CommandRunner`) in a new `compose_test.go`. Write tests verifying:
- Base args include `-H unix:///socket` when socket is set
- Args contain `compose -p <project> -f <file> up -d`
- `--project-directory` is included when `workDir` is non-empty
- Override file `-f <override>` is included when `overrideFile` is non-empty
- Override file is absent when `overrideFile` is empty
- Error from runner propagates wrapped in `"docker compose up failed: ..."`
**Files**: `backend/internal/reconcile/compose_test.go` (new, package `reconcile` — internal access)
**Acceptance**: `go test ./internal/reconcile/ -run TestComposeUp -v` passes with ≥4 subtests

### 2. ComposeDown arg-capture tests
**What**: In the same `compose_test.go`, test `ComposeDown`:
- Args contain `compose -p <project> down --remove-orphans`
- `--project-directory` present when `workDir` set
- `-f <file>` present when `composeFile` non-empty, absent when empty
- Error wrapping matches `"docker compose down failed: ..."`
**Files**: `backend/internal/reconcile/compose_test.go`
**Acceptance**: `go test ./internal/reconcile/ -run TestComposeDown -v` passes with ≥3 subtests

### 3. ComposePs JSON parsing tests
**What**: Test `ComposePs` with stubbed runner output:
- Single container JSON line → returns 1 `ContainerInfo` with correct fields (ID truncated to 12 chars, Name, Service, State, Health, Image, Ports)
- Multiple JSON lines → returns multiple containers
- Empty output → returns nil, nil
- Health defaults to `"none"` when omitted from JSON
- Published ports format: `published:target/proto` vs `target/proto` when unpublished
- Invalid JSON lines are skipped silently
- Runner error propagates
**Files**: `backend/internal/reconcile/compose_test.go`
**Acceptance**: `go test ./internal/reconcile/ -run TestComposePs -v` passes with ≥5 subtests

### 4. generateLabelArgs + generateLabelOverride tests
**What**: Test both label-generation functions:
- `generateLabelArgs` returns map with all 7 expected keys (`LabelStackPath`, `LabelDesiredRevision`, `LabelDesiredCommitMessage`, `LabelDesiredComposeHash`, `LabelSyncedAt`, `LabelSyncAt`, `LabelSyncStatus`)
- `LabelSyncStatus` value is `"synced"`
- Values match inputs (stackPath, revision, commitMessage, composeHash)
- `generateLabelOverride` produces valid YAML-ish structure with `services:` header and all labels per service
- Empty `serviceNames` returns `""`
- `escapeYAMLValue` handles quotes and newlines (test via `generateLabelOverride` with commit messages containing those)
- Use `TestGenerateLabelOverride` export for `_test` package tests, or write internal package tests

Note: `generateLabelArgs` is unexported. Either add an `export_test_helpers.go` entry or write these tests in package `reconcile` (not `reconcile_test`).
**Files**: `backend/internal/reconcile/labels_test.go` (new) + possibly `backend/internal/reconcile/export_test_helpers.go` (add `TestGenerateLabelArgs` export)
**Acceptance**: `go test ./internal/reconcile/ -run TestGenerateLabel -v` passes with ≥4 subtests

### 5. CancelActive context cancellation test
**What**: Test `Reconciler.CancelActive()`:
- Start a `Reconcile()` in a goroutine with a `stubComposeRunner` that blocks on `ComposeUp` (using a channel)
- Call `CancelActive()` from the main goroutine
- Verify the blocked `ComposeUp` receives context cancellation
- Verify `CancelActive()` is safe to call when no reconciliation is active (no panic)
**Files**: `backend/internal/reconcile/reconcile_test.go` (append)
**Acceptance**: `go test ./internal/reconcile/ -run TestCancelActive -v -timeout=5s` passes

### 6. CancelActiveUpdate cancellation test
**What**: Test `SchedulerService.CancelActiveUpdate()`:
- Create scheduler with a `mockRunner` that has `pullDelay` set
- Trigger an update cycle
- Call `CancelActiveUpdate()`
- Verify the cycle terminates early (fewer stacks processed than total)
- Verify `CancelActiveUpdate()` is safe when no cycle is active
**Files**: `backend/internal/scheduler/scheduler_test.go` (append)
**Acceptance**: `go test ./internal/scheduler/ -run TestCancelActiveUpdate -v -timeout=5s` passes

### 7. UpdateContainerCounts store update test
**What**: Test `StateManager.UpdateContainerCounts()`:
- Set up store with a stack at a known path
- Create a `stubComposeRunner` whose `ComposePs` returns N containers (e.g. 2 running, 1 exited)
- Call `UpdateContainerCounts`
- Verify `store.Get().Stacks[i].ContainersRunning` and `ContainersTotal` match expected values
- Test with `ComposePs` returning error → counts not updated, no panic
- Test with nil store snapshot → no panic
**Files**: `backend/internal/reconcile/state_manager_test.go` (new, package `reconcile_test`)
**Acceptance**: `go test ./internal/reconcile/ -run TestUpdateContainerCounts -v` passes with ≥3 subtests

### 8. AllLabelKeys completeness test
**What**: Test `AllLabelKeys()`:
- Returns exactly 8 keys
- Contains every `Label*` constant defined in `labels.go`
- No duplicates
**Files**: `backend/internal/reconcile/labels_test.go` (same file as task 4)
**Acceptance**: `go test ./internal/reconcile/ -run TestAllLabelKeys -v` passes

### Verification
- All tests pass: `cd backend && go test ./internal/reconcile/... ./internal/scheduler/... -count=1`
- No regressions: existing tests in `reconcile_test.go`, `extract_test.go`, `metadata_test.go`, `scheduler_test.go`, and `client_test.go` continue to pass
- Race detector: `go test -race ./internal/reconcile/... ./internal/scheduler/...`
