//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/internal/scheduler"
	"github.com/lucasreiners/docker-cd/tests/integration/dind"
)

func TestDinD_UpdateCycleUsesLocalClone(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	repoDir := t.TempDir()

	stackPath := "updateapp"
	stackDir := filepath.Join(repoDir, stackPath)
	if err := os.MkdirAll(stackDir, 0755); err != nil {
		t.Fatalf("failed to create stack dir: %v", err)
	}
	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n")
	composeFile := "docker-compose.yml"
	composePath := filepath.Join(stackDir, composeFile)
	if err := os.WriteFile(composePath, composeContent, 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	composeHash := desiredstate.ComposeHash(composeContent)
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:       "rev1",
		CommitMessage:  "update cycle",
		RefreshStatus:  desiredstate.RefreshStatusCompleted,
		UpdatesBlocked: false,
		Stacks: []desiredstate.StackRecord{
			{
				Path:        stackPath,
				ComposeFile: composeFile,
				ComposeHash: composeHash,
				Status:      desiredstate.StackSyncMissing,
				Content:     composeContent,
			},
		},
	})

	sdkAPI, err := dind.NewSDKClient(env.DockerHost)
	if err != nil {
		t.Fatalf("SDK client: %v", err)
	}
	t.Cleanup(func() { _ = sdkAPI.Close() })
	client := docker.NewClient(sdkAPI)
	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	policy := reconcile.DefaultPolicy()
	reconciler := newDindReconciler(store, policy, composeRunner, inspector, reconcile.NewAckStore(), "")

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{UpdaterEnabled: true, UpdaterCron: "0 3 * * *"}
	schedulerSvc, err := scheduler.NewSchedulerService(cfg, logger, store, client, reconciler, nil)
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	if schedulerSvc == nil {
		t.Fatal("expected scheduler to be enabled")
	}
	schedulerSvc.SetRepoPath(repoDir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer stopCancel()
		_ = schedulerSvc.Stop(stopCtx)
	})
	if run := reconciler.ReconcileStack(ctx, stackPath); run.Result != "success" {
		t.Fatalf("initial reconcile: result=%s error=%s", run.Result, run.Error)
	}

	if err := schedulerSvc.TriggerUpdateCycle(ctx); err != nil {
		t.Fatalf("TriggerUpdateCycle failed: %v", err)
	}

	waitForUpdateCompletion(t, schedulerSvc, 30*time.Second)
	waitForContainers(t, runner, 1, 20*time.Second)

	labels, err := inspector.GetStackLabels(context.Background())
	if err != nil {
		t.Fatalf("GetStackLabels failed: %v", err)
	}
	meta, ok := labels[stackPath]
	if !ok {
		t.Fatalf("expected label metadata for stack %q", stackPath)
	}
	if meta.SyncStatus != "synced" {
		t.Fatalf("expected synced status, got %q", meta.SyncStatus)
	}

	cleanupStack(t, runner, stackPath)
}

func TestDinD_UpdateCycleRestartsContainers(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	fixture, err := dind.NewMutableImage(ctx, env)
	if err != nil {
		t.Fatalf("create mutable image fixture: %v", err)
	}

	repoDir := t.TempDir()

	stackPath := "restartapp"
	stackDir := filepath.Join(repoDir, stackPath)
	if err := os.MkdirAll(stackDir, 0755); err != nil {
		t.Fatalf("failed to create stack dir: %v", err)
	}
	composeContent := []byte(fmt.Sprintf("services:\n  web:\n    image: %s\n    command: [\"sleep\", \"86400\"]\n", fixture.Ref))
	composeFile := "docker-compose.yml"
	if err := os.WriteFile(filepath.Join(stackDir, composeFile), composeContent, 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	composeHash := desiredstate.ComposeHash(composeContent)
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:       "rev1",
		CommitMessage:  "initial deploy",
		RefreshStatus:  desiredstate.RefreshStatusCompleted,
		UpdatesBlocked: false,
		Stacks: []desiredstate.StackRecord{{
			Path: stackPath, ComposeFile: composeFile,
			ComposeHash: composeHash, Status: desiredstate.StackSyncMissing,
			Content: composeContent,
		}},
	})

	sdkAPI, err := dind.NewSDKClient(env.DockerHost)
	if err != nil {
		t.Fatalf("SDK client: %v", err)
	}
	t.Cleanup(func() { _ = sdkAPI.Close() })
	client := docker.NewClient(sdkAPI)
	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	policy := reconcile.DefaultPolicy()
	reconciler := newDindReconciler(store, policy, composeRunner, inspector, reconcile.NewAckStore(), "")

	// A is prewarmed, so explicitly deploy instead of expecting an unchanged
	// image update to create a missing stack.
	if run := reconciler.ReconcileStack(ctx, stackPath); run.Result != "success" {
		t.Fatalf("initial reconcile: result=%s error=%s", run.Result, run.Error)
	}
	initial := waitForMutableImageContainer(t, ctx, sdkAPI, stackPath, fixture.Initial.ImageID, "")
	if initial.Config.Image != fixture.Ref {
		t.Fatalf("initial image reference = %q, want %q", initial.Config.Image, fixture.Ref)
	}
	remote, err := fixture.RemoteDigest(ctx)
	if err != nil || remote != fixture.Initial.Digest {
		t.Fatalf("initial remote digest = %q, want %q (error: %v)", remote, fixture.Initial.Digest, err)
	}
	t.Logf("initial: C1=%s imageA=%s ref=%s manifestA=%s composeHash=%s", initial.ID, initial.Image, initial.Config.Image, remote, composeHash)

	cfg := config.Config{UpdaterEnabled: true, UpdaterCron: "0 3 * * *"}
	schedulerSvc, err := scheduler.NewSchedulerService(cfg, logger, store, client, reconciler, nil)
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	schedulerSvc.SetRepoPath(repoDir)
	t.Cleanup(func() {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer stopCancel()
		_ = schedulerSvc.Stop(stopCtx)
	})

	// An unchanged tag must not force-recreate a healthy running container.
	if err := schedulerSvc.TriggerUpdateCycle(ctx); err != nil {
		t.Fatalf("unchanged TriggerUpdateCycle: %v", err)
	}
	waitForUpdateCompletion(t, schedulerSvc, 30*time.Second)
	unchanged := waitForMutableImageContainer(t, ctx, sdkAPI, stackPath, fixture.Initial.ImageID, "")
	if unchanged.ID != initial.ID {
		t.Fatalf("unchanged image recreated C1: before=%s after=%s", initial.ID, unchanged.ID)
	}
	t.Logf("unchanged update: C1=%s imageA=%s", unchanged.ID, unchanged.Image)

	// Publish B only now; the unchanged cycle prunes unused images. Restore
	// local A without pushing so scheduler's before/after pull IDs really differ.
	changed, err := fixture.PublishChanged(ctx)
	if err != nil {
		t.Fatalf("publish changed image: %v", err)
	}
	local, err := fixture.LocalImageID(ctx)
	if err != nil || local != fixture.Initial.ImageID {
		t.Fatalf("before changed update local image = %q, want A=%q (error: %v)", local, fixture.Initial.ImageID, err)
	}
	remote, err = fixture.RemoteDigest(ctx)
	if err != nil || remote != changed.Digest {
		t.Fatalf("before changed update remote digest = %q, want B=%q (error: %v)", remote, changed.Digest, err)
	}
	beforePull := waitForMutableImageContainer(t, ctx, sdkAPI, stackPath, fixture.Initial.ImageID, "")
	if beforePull.ID != initial.ID {
		t.Fatalf("publishing B unexpectedly replaced running A: %s", beforePull.ID)
	}
	t.Logf("before changed update: remoteB=%s imageB=%s localA=%s runningC1=%s", remote, changed.ImageID, local, beforePull.ID)

	if err := schedulerSvc.TriggerUpdateCycle(ctx); err != nil {
		t.Fatalf("changed TriggerUpdateCycle: %v", err)
	}
	waitForUpdateCompletion(t, schedulerSvc, 30*time.Second)
	replaced := waitForMutableImageContainer(t, ctx, sdkAPI, stackPath, changed.ImageID, initial.ID)
	if replaced.Config.Image != fixture.Ref || replaced.Config.Labels[reconcile.LabelDesiredComposeHash] != composeHash {
		t.Fatalf("image reference/compose hash changed: ref=%q labels=%v", replaced.Config.Image, replaced.Config.Labels)
	}
	// Generated deployment labels (such as synced_at) can change Compose's
	// effective config-hash. Assert the actual source configuration instead.
	afterCompose, err := os.ReadFile(filepath.Join(stackDir, composeFile))
	if err != nil || !bytes.Equal(afterCompose, composeContent) {
		t.Fatalf("source Compose changed during image-only update (error: %v)", err)
	}
	if snap := store.Get(); len(snap.Stacks) != 1 || !bytes.Equal(snap.Stacks[0].Content, composeContent) || snap.Stacks[0].ComposeHash != composeHash {
		t.Fatal("desired source configuration changed during image-only update")
	}
	local, err = fixture.LocalImageID(ctx)
	if err != nil || local != changed.ImageID {
		t.Fatalf("after update local image = %q, want B=%q (error: %v)", local, changed.ImageID, err)
	}
	t.Logf("changed update: C2=%s imageB=%s ref=%s manifestB=%s composeHash=%s", replaced.ID, replaced.Image, replaced.Config.Image, changed.Digest, composeHash)
}

// Filter the exact managed stack: the private registry is also a running
// container. Poll actual runtime image/state, not merely scheduler idleness.
func waitForMutableImageContainer(t *testing.T, ctx context.Context, api *dind.AuthenticatedClient, stack, imageID, previousID string) types.ContainerJSON {
	t.Helper()
	pollCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var last string
	for {
		items, err := api.ContainerList(pollCtx, container.ListOptions{All: true, Filters: filters.NewArgs(filters.Arg("label", reconcile.LabelStackPath+"="+stack))})
		if err == nil && len(items) == 1 {
			current, inspectErr := api.ContainerInspect(pollCtx, items[0].ID)
			if inspectErr == nil && current.State != nil && current.State.Running && current.Image == imageID && current.ID != previousID {
				return current
			}
			last = fmt.Sprintf("container=%s image=%s state=%+v inspectError=%v", current.ID, current.Image, current.State, inspectErr)
		} else {
			last = fmt.Sprintf("containers=%d error=%v", len(items), err)
		}
		select {
		case <-pollCtx.Done():
			t.Fatalf("waiting for stack %s running image %s (previous=%s): %s: %v", stack, imageID, previousID, last, pollCtx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func waitForUpdateCompletion(t *testing.T, schedulerSvc *scheduler.SchedulerService, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if schedulerSvc.GetUpdateStatus() == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("timed out waiting for update cycle completion")
}
