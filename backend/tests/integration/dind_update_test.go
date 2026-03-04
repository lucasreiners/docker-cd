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

	if err := os.MkdirAll("/repo", 0755); err != nil {
		t.Skipf("unable to create /repo: %v", err)
	}

	stackPath := "updateapp"
	stackDir := filepath.Join("/repo", stackPath)
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
	reconciler := reconcile.NewReconciler(store, policy, composeRunner, inspector, reconcile.NewAckStore(), "")

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{UpdaterEnabled: true, UpdaterCron: "0 3 * * *"}
	schedulerSvc, err := scheduler.NewSchedulerService(cfg, logger, store, client, reconciler, nil)
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	if schedulerSvc == nil {
		t.Fatal("expected scheduler to be enabled")
	}

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
