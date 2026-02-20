//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/tests/integration/dind"
)

type dindRunner struct {
	Host string
}

func (r *dindRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), "DOCKER_HOST="+r.Host)
	return cmd.CombinedOutput()
}

func TestDinD_ReconcileComposeUp(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	out, err := runner.Run(context.Background(), "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil {
		t.Fatalf("DinD daemon unreachable: %v (output: %s)", err, string(out))
	}
	t.Logf("DinD Docker version: %s", strings.TrimSpace(string(out)))

	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n    command: [\"nginx\", \"-g\", \"daemon off;\"]\n")
	composeHash := desiredstate.ComposeHash(composeContent)

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "abc123",
		CommitMessage: "initial deploy",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{
				Path:        "myapp",
				ComposeFile: "docker-compose.yml",
				ComposeHash: composeHash,
				Status:      desiredstate.StackSyncMissing,
				Content:     composeContent,
			},
		},
	})

	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	client := docker.NewClient(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)

	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), composeRunner, inspector, reconcile.NewAckStore(), "")
	runs := r.Reconcile(context.Background())

	if len(runs) != 1 {
		t.Fatalf("expected 1 reconciliation run, got %d", len(runs))
	}
	if runs[0].Result != "success" {
		t.Fatalf("expected success, got %q (error: %s)", runs[0].Result, runs[0].Error)
	}

	waitForContainers(t, runner, 1, 15*time.Second)

	ctx := context.Background()
	labels, err := inspector.GetStackLabels(ctx)
	if err != nil {
		t.Fatalf("GetStackLabels failed: %v", err)
	}

	meta, ok := labels["myapp"]
	if !ok {
		t.Fatalf("expected label metadata for stack 'myapp', got keys: %v", mapKeys(labels))
	}
	if meta.DesiredRevision != "abc123" {
		t.Errorf("DesiredRevision: got %q, want %q", meta.DesiredRevision, "abc123")
	}
	if meta.DesiredComposeHash != composeHash {
		t.Errorf("DesiredComposeHash: got %q, want %q", meta.DesiredComposeHash, composeHash)
	}
	if meta.DesiredCommitMessage != "initial deploy" {
		t.Errorf("DesiredCommitMessage: got %q, want %q", meta.DesiredCommitMessage, "initial deploy")
	}
	if meta.SyncStatus != "synced" {
		t.Errorf("SyncStatus: got %q, want %q", meta.SyncStatus, "synced")
	}
	if meta.SyncedAt == "" {
		t.Error("SyncedAt should not be empty")
	}

	snap := store.Get()
	if snap.Stacks[0].Status != desiredstate.StackSyncSynced {
		t.Errorf("store status: got %q, want %q", snap.Stacks[0].Status, desiredstate.StackSyncSynced)
	}
	cleanupStack(t, runner, "myapp")
}

func TestDinD_ReconcileNoOpWhenInSync(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n")
	composeHash := desiredstate.ComposeHash(composeContent)

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "rev1",
		CommitMessage: "deploy",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{
				Path:        "noopapp",
				ComposeFile: "docker-compose.yml",
				ComposeHash: composeHash,
				Status:      desiredstate.StackSyncMissing,
				Content:     composeContent,
			},
		},
	})

	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	client := docker.NewClient(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), composeRunner, inspector, reconcile.NewAckStore(), "")

	runs := r.Reconcile(context.Background())
	if len(runs) != 1 || runs[0].Result != "success" {
		t.Fatalf("first reconcile: expected 1 success, got %d runs, result=%q", len(runs), safeResult(runs))
	}
	waitForContainers(t, runner, 1, 15*time.Second)

	runs2 := r.Reconcile(context.Background())
	if len(runs2) != 0 {
		t.Errorf("second reconcile should be no-op, got %d runs", len(runs2))
		for _, run := range runs2 {
			t.Logf("  run: stack=%s result=%s error=%s", run.StackPath, run.Result, run.Error)
		}
	}
	cleanupStack(t, runner, "noopapp")
}

func TestDinD_DriftDetectionRevisionChange(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n")
	composeHash := desiredstate.ComposeHash(composeContent)

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "rev1",
		CommitMessage: "v1",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{
				Path:        "driftapp",
				ComposeFile: "docker-compose.yml",
				ComposeHash: composeHash,
				Status:      desiredstate.StackSyncMissing,
				Content:     composeContent,
			},
		},
	})

	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	client := docker.NewClient(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), composeRunner, inspector, reconcile.NewAckStore(), "")

	runs := r.Reconcile(context.Background())
	if len(runs) != 1 || runs[0].Result != "success" {
		t.Fatalf("initial deploy failed: %s", safeResult(runs))
	}
	waitForContainers(t, runner, 1, 15*time.Second)

	store.Set(&desiredstate.Snapshot{
		Revision:      "rev2",
		CommitMessage: "v2 hotfix",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{
				Path:        "driftapp",
				ComposeFile: "docker-compose.yml",
				ComposeHash: composeHash,
				Status:      desiredstate.StackSyncSynced,
				Content:     composeContent,
			},
		},
	})

	runs2 := r.Reconcile(context.Background())
	if len(runs2) != 1 {
		t.Fatalf("expected 1 re-sync run, got %d", len(runs2))
	}
	if runs2[0].Result != "success" {
		t.Fatalf("re-sync failed: %s", runs2[0].Error)
	}

	labels, err := inspector.GetStackLabels(context.Background())
	if err != nil {
		t.Fatalf("GetStackLabels failed: %v", err)
	}
	meta := labels["driftapp"]
	if meta.DesiredRevision != "rev2" {
		t.Errorf("after re-sync, revision should be 'rev2', got %q", meta.DesiredRevision)
	}
	if meta.DesiredCommitMessage != "v2 hotfix" {
		t.Errorf("after re-sync, commit message should be 'v2 hotfix', got %q", meta.DesiredCommitMessage)
	}
	cleanupStack(t, runner, "driftapp")
}

func TestDinD_StackRemoval(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n")
	composeHash := desiredstate.ComposeHash(composeContent)

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "rev1",
		CommitMessage: "deploy",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{
				Path:        "removeapp",
				ComposeFile: "docker-compose.yml",
				ComposeHash: composeHash,
				Status:      desiredstate.StackSyncMissing,
				Content:     composeContent,
			},
		},
	})

	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	client := docker.NewClient(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	policy := reconcile.DefaultPolicy()
	policy.RemoveEnabled = true
	r := reconcile.NewReconciler(store, policy, composeRunner, inspector, reconcile.NewAckStore(), "")

	runs := r.Reconcile(context.Background())
	if len(runs) != 1 || runs[0].Result != "success" {
		t.Fatalf("initial deploy failed: %s", safeResult(runs))
	}
	waitForContainers(t, runner, 1, 15*time.Second)

	store.Set(&desiredstate.Snapshot{
		Revision:      "rev2",
		CommitMessage: "removed app",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks:        []desiredstate.StackRecord{},
	})

	runs2 := r.Reconcile(context.Background())
	if len(runs2) != 1 {
		t.Fatalf("expected 1 removal run, got %d", len(runs2))
	}
	if runs2[0].Result != "success" {
		t.Fatalf("removal failed: %s", runs2[0].Error)
	}

	time.Sleep(2 * time.Second)
	labels, err := inspector.GetStackLabels(context.Background())
	if err != nil {
		t.Fatalf("GetStackLabels failed: %v", err)
	}
	if _, exists := labels["removeapp"]; exists {
		t.Error("expected stack 'removeapp' to be removed, but labels still found")
	}
}

func TestDinD_MultiServiceStack(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n  api:\n    image: nginx:alpine\n    command: [\"nginx\", \"-g\", \"daemon off;\"]\n")
	composeHash := desiredstate.ComposeHash(composeContent)

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "rev1",
		CommitMessage: "multi-svc deploy",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{
				Path:        "multisvc",
				ComposeFile: "docker-compose.yml",
				ComposeHash: composeHash,
				Status:      desiredstate.StackSyncMissing,
				Content:     composeContent,
			},
		},
	})

	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	client := docker.NewClient(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), composeRunner, inspector, reconcile.NewAckStore(), "")

	runs := r.Reconcile(context.Background())
	if len(runs) != 1 || runs[0].Result != "success" {
		t.Fatalf("multi-service deploy failed: %s", safeResult(runs))
	}
	waitForContainers(t, runner, 2, 20*time.Second)

	labels, err := inspector.GetStackLabels(context.Background())
	if err != nil {
		t.Fatalf("GetStackLabels failed: %v", err)
	}
	meta, ok := labels["multisvc"]
	if !ok {
		t.Fatal("expected label metadata for stack 'multisvc'")
	}
	if meta.DesiredRevision != "rev1" {
		t.Errorf("revision: got %q, want 'rev1'", meta.DesiredRevision)
	}

	containers, err := client.ListContainersWithLabel(context.Background(), reconcile.LabelStackPath)
	if err != nil {
		t.Fatalf("ListContainersWithLabel failed: %v", err)
	}
	if len(containers) < 2 {
		t.Errorf("expected at least 2 containers with labels, got %d", len(containers))
	}
	cleanupStack(t, runner, "multisvc")
}

func TestDinD_ComposeHashDrift(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	composeV1 := []byte("services:\n  web:\n    image: nginx:alpine\n")
	hashV1 := desiredstate.ComposeHash(composeV1)

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "rev1",
		CommitMessage: "v1",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "hashapp", ComposeFile: "docker-compose.yml", ComposeHash: hashV1, Status: desiredstate.StackSyncMissing, Content: composeV1},
		},
	})

	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	client := docker.NewClient(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), composeRunner, inspector, reconcile.NewAckStore(), "")

	runs := r.Reconcile(context.Background())
	if len(runs) != 1 || runs[0].Result != "success" {
		t.Fatalf("v1 deploy failed: %s", safeResult(runs))
	}
	waitForContainers(t, runner, 1, 15*time.Second)

	composeV2 := []byte("services:\n  web:\n    image: nginx:stable-alpine\n")
	hashV2 := desiredstate.ComposeHash(composeV2)

	store.Set(&desiredstate.Snapshot{
		Revision:      "rev1",
		CommitMessage: "v2 image update",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "hashapp", ComposeFile: "docker-compose.yml", ComposeHash: hashV2, Status: desiredstate.StackSyncSynced, Content: composeV2},
		},
	})

	runs2 := r.Reconcile(context.Background())
	if len(runs2) != 1 {
		t.Fatalf("expected 1 re-deploy for hash drift, got %d", len(runs2))
	}
	if runs2[0].Result != "success" {
		t.Fatalf("hash drift re-deploy failed: %s", runs2[0].Error)
	}

	labels, err := inspector.GetStackLabels(context.Background())
	if err != nil {
		t.Fatalf("GetStackLabels failed: %v", err)
	}
	if labels["hashapp"].DesiredComposeHash != hashV2 {
		t.Errorf("compose hash should be %q after re-deploy, got %q", hashV2, labels["hashapp"].DesiredComposeHash)
	}
	cleanupStack(t, runner, "hashapp")
}

func TestDinD_FlagPolicyRequiresAck(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n")
	composeHash := desiredstate.ComposeHash(composeContent)

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "rev1",
		CommitMessage: "deploy",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "flagapp", ComposeFile: "docker-compose.yml", ComposeHash: composeHash, Status: desiredstate.StackSyncMissing, Content: composeContent},
		},
	})

	policy := reconcile.DefaultPolicy()
	policy.DriftPolicy = "flag"

	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	client := docker.NewClient(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	ackStore := reconcile.NewAckStore()
	r := reconcile.NewReconciler(store, policy, composeRunner, inspector, ackStore, "")

	runs := r.Reconcile(context.Background())
	if len(runs) != 0 {
		t.Errorf("expected 0 runs without ack, got %d", len(runs))
	}

	ackStore.Acknowledge("flagapp")
	runs2 := r.Reconcile(context.Background())
	if len(runs2) != 1 {
		t.Fatalf("expected 1 run after ack, got %d", len(runs2))
	}
	if runs2[0].Result != "success" {
		t.Fatalf("reconcile after ack failed: %s", runs2[0].Error)
	}
	waitForContainers(t, runner, 1, 15*time.Second)
	cleanupStack(t, runner, "flagapp")
}

// TestDinD_FourSpaceIndentLabels is a regression test: compose files with
// 4-space indentation must have labels applied and the second reconcile
// must be a no-op (previously they caused infinite re-deploys).
func TestDinD_FourSpaceIndentLabels(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	// 4-space indentation — the exact format that triggered the bug.
	composeContent := []byte("services:\n    web:\n        image: nginx:alpine\n        command: [\"nginx\", \"-g\", \"daemon off;\"]\n")
	composeHash := desiredstate.ComposeHash(composeContent)

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "fix-rev",
		CommitMessage: "4-space indent deploy",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{
				Path:        "indent4app",
				ComposeFile: "docker-compose.yml",
				ComposeHash: composeHash,
				Status:      desiredstate.StackSyncMissing,
				Content:     composeContent,
			},
		},
	})

	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	client := docker.NewClient(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), composeRunner, inspector, reconcile.NewAckStore(), "")

	// First reconcile: should deploy.
	runs := r.Reconcile(context.Background())
	if len(runs) != 1 || runs[0].Result != "success" {
		t.Fatalf("first reconcile: expected 1 success, got %s", safeResult(runs))
	}
	waitForContainers(t, runner, 1, 15*time.Second)

	// Verify labels were applied.
	labels, err := inspector.GetStackLabels(context.Background())
	if err != nil {
		t.Fatalf("GetStackLabels failed: %v", err)
	}
	meta, ok := labels["indent4app"]
	if !ok {
		t.Fatalf("expected label metadata for stack 'indent4app', got keys: %v", mapKeys(labels))
	}
	if meta.DesiredRevision != "fix-rev" {
		t.Errorf("DesiredRevision: got %q, want %q", meta.DesiredRevision, "fix-rev")
	}
	if meta.DesiredComposeHash != composeHash {
		t.Errorf("DesiredComposeHash: got %q, want %q", meta.DesiredComposeHash, composeHash)
	}

	// Second reconcile: must be a no-op (previously caused infinite loop).
	runs2 := r.Reconcile(context.Background())
	if len(runs2) != 0 {
		t.Errorf("second reconcile should be no-op, got %d runs", len(runs2))
		for _, run := range runs2 {
			t.Logf("  run: stack=%s result=%s reason=%s", run.StackPath, run.Result, run.Error)
		}
	}

	cleanupStack(t, runner, "indent4app")
}

// --- Helpers ---

func waitForContainers(t *testing.T, runner *dindRunner, min int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := runner.Run(context.Background(), "docker", "ps", "-q", "--filter", "label="+reconcile.LabelStackPath)
		if err == nil {
			lines := strings.TrimSpace(string(out))
			if lines != "" && len(strings.Split(lines, "\n")) >= min {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for at least %d containers (timeout: %s)", min, timeout)
}

func cleanupStack(t *testing.T, runner *dindRunner, projectName string) {
	t.Helper()
	_, _ = runner.Run(context.Background(), "docker", "compose", "-p", projectName, "down", "--remove-orphans", "--timeout", "5")
}

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func safeResult(runs []reconcile.ReconciliationRun) string {
	if len(runs) == 0 {
		return "<no runs>"
	}
	parts := make([]string, len(runs))
	for i, run := range runs {
		parts[i] = fmt.Sprintf("stack=%s result=%s error=%s", run.StackPath, run.Result, run.Error)
	}
	return strings.Join(parts, "; ")
}
