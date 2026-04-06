//go:build integration

package integration_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

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

	client := docker.NewClient(runner, env.DockerHost)
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

	if err := schedulerSvc.TriggerUpdateCycle(context.Background()); err != nil {
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

	repoDir := t.TempDir()

	// 1. Create stack dir with nginx:alpine compose file
	stackPath := "restartapp"
	stackDir := filepath.Join(repoDir, stackPath)
	if err := os.MkdirAll(stackDir, 0755); err != nil {
		t.Fatalf("failed to create stack dir: %v", err)
	}
	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n")
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

	client := docker.NewClient(runner, env.DockerHost)
	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	policy := reconcile.DefaultPolicy()
	reconciler := newDindReconciler(store, policy, composeRunner, inspector, reconcile.NewAckStore(), "")

	// 2. First update cycle — deploys the stack
	cfg := config.Config{UpdaterEnabled: true, UpdaterCron: "0 3 * * *"}
	schedulerSvc, err := scheduler.NewSchedulerService(cfg, logger, store, client, reconciler, nil)
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	schedulerSvc.SetRepoPath(repoDir)

	if err := schedulerSvc.TriggerUpdateCycle(context.Background()); err != nil {
		t.Fatalf("first TriggerUpdateCycle failed: %v", err)
	}
	waitForUpdateCompletion(t, schedulerSvc, 30*time.Second)
	waitForContainers(t, runner, 1, 20*time.Second)

	// 3. Record container IDs BEFORE second update
	containers1, err := client.ListContainersWithLabel(context.Background(), reconcile.LabelStackPath)
	if err != nil {
		t.Fatalf("ListContainersWithLabel failed: %v", err)
	}
	if len(containers1) == 0 {
		t.Fatal("expected at least 1 container after first deploy")
	}
	idsBefore := make(map[string]bool)
	for _, c := range containers1 {
		idsBefore[c.ContainerID] = true
	}
	t.Logf("container IDs before update: %v", idsBefore)

	// 4. Mark stack as synced (simulates compose unchanged, just image pull)
	store.Set(&desiredstate.Snapshot{
		Revision:       "rev1",
		CommitMessage:  "update cycle",
		RefreshStatus:  desiredstate.RefreshStatusCompleted,
		UpdatesBlocked: false,
		Stacks: []desiredstate.StackRecord{{
			Path: stackPath, ComposeFile: composeFile,
			ComposeHash: composeHash,
			Status:      desiredstate.StackSyncSynced,
			Content:     composeContent,
		}},
	})

	// 5. Second update cycle — should pull images and force-recreate
	if err := schedulerSvc.TriggerUpdateCycle(context.Background()); err != nil {
		t.Fatalf("second TriggerUpdateCycle failed: %v", err)
	}
	waitForUpdateCompletion(t, schedulerSvc, 30*time.Second)
	// Allow time for new containers to spin up
	time.Sleep(3 * time.Second)

	// 6. Record container IDs AFTER second update
	containers2, err := client.ListContainersWithLabel(context.Background(), reconcile.LabelStackPath)
	if err != nil {
		t.Fatalf("ListContainersWithLabel after update failed: %v", err)
	}
	if len(containers2) == 0 {
		t.Fatal("expected at least 1 container after second update")
	}

	// 7. ASSERT: container IDs must have changed (containers were recreated)
	allSame := true
	for _, c := range containers2 {
		if !idsBefore[c.ContainerID] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Fatal("BUG: container IDs are the same after update cycle — containers were NOT recreated with new images")
	}

	t.Log("SUCCESS: containers were recreated after update cycle")
	cleanupStack(t, runner, stackPath)
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
