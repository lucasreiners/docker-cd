//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/tests/integration/dind"
)

func TestDinD_ComposeUp_WritesFilesAndApplies(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n")
	composeHash := desiredstate.ComposeHash(composeContent)

	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)

	overrideContent := reconcile.TestGenerateLabelOverride("mystack", "abc123", "test deploy", composeHash, []string{"web"})
	composeFile, overrideFile, cleanup, err := reconcile.TestWriteTempComposeDir("docker-compose.yml", composeContent, overrideContent)
	if err != nil {
		t.Fatalf("WriteTempComposeDir failed: %v", err)
	}
	defer cleanup()

	t.Logf("Compose file: %s", composeFile)
	t.Logf("Override file: %s", overrideFile)

	if !strings.HasPrefix(composeFile, "/") {
		t.Errorf("compose path should be absolute, got %q", composeFile)
	}

	err = composeRunner.ComposeUp(context.Background(), "mystack", composeFile, overrideFile, filepath.Dir(composeFile))
	if err != nil {
		t.Fatalf("compose up failed: %v", err)
	}

	waitForContainers(t, runner, 1, 15*time.Second)

	client := docker.NewClient(runner, env.DockerHost)
	containers, err := client.ListContainersWithLabel(context.Background(), reconcile.LabelStackPath)
	if err != nil {
		t.Fatalf("ListContainersWithLabel failed: %v", err)
	}

	if len(containers) < 1 {
		t.Fatal("expected at least 1 container with labels after compose up")
	}

	found := false
	for _, c := range containers {
		if c.Labels[reconcile.LabelStackPath] == "mystack" {
			found = true
			if c.Labels[reconcile.LabelDesiredRevision] != "abc123" {
				t.Errorf("revision label: got %q, want 'abc123'", c.Labels[reconcile.LabelDesiredRevision])
			}
			break
		}
	}
	if !found {
		t.Error("no container found with stack path 'mystack'")
	}
	cleanupStack(t, runner, "mystack")
}

func TestDinD_ComposeDown_RemovesContainers(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n")
	composeHash := desiredstate.ComposeHash(composeContent)
	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)

	overrideContent := reconcile.TestGenerateLabelOverride("downstack", "rev1", "test", composeHash, []string{"web"})
	composeFile, overrideFile, cleanup, err := reconcile.TestWriteTempComposeDir("docker-compose.yml", composeContent, overrideContent)
	if err != nil {
		t.Fatalf("WriteTempComposeDir failed: %v", err)
	}
	defer cleanup()

	err = composeRunner.ComposeUp(context.Background(), "downstack", composeFile, overrideFile, filepath.Dir(composeFile))
	if err != nil {
		t.Fatalf("compose up failed: %v", err)
	}
	waitForContainers(t, runner, 1, 15*time.Second)

	err = composeRunner.ComposeDown(context.Background(), "downstack", composeFile, filepath.Dir(composeFile))
	if err != nil {
		t.Fatalf("compose down failed: %v", err)
	}

	time.Sleep(2 * time.Second)

	client := docker.NewClient(runner, env.DockerHost)
	containers, err := client.ListContainersWithLabel(context.Background(), reconcile.LabelStackPath)
	if err != nil {
		t.Fatalf("ListContainersWithLabel failed: %v", err)
	}
	for _, c := range containers {
		if c.Labels[reconcile.LabelStackPath] == "downstack" {
			t.Error("expected no containers for 'downstack' after compose down")
		}
	}
}

func TestDinD_ListContainersWithLabel_RealDaemon(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}
	client := docker.NewClient(runner, env.DockerHost)

	labelValue := "my/stack,path"
	_, err := runner.Run(context.Background(), "docker", "run", "-d",
		"--label", fmt.Sprintf("%s=%s", reconcile.LabelStackPath, labelValue),
		"--label", fmt.Sprintf("%s=rev1", reconcile.LabelDesiredRevision),
		"--label", fmt.Sprintf("%s=hash1", reconcile.LabelDesiredComposeHash),
		"nginx:alpine",
	)
	if err != nil {
		t.Fatalf("docker run failed: %v", err)
	}

	time.Sleep(2 * time.Second)

	containers, err := client.ListContainersWithLabel(context.Background(), reconcile.LabelStackPath)
	if err != nil {
		t.Fatalf("ListContainersWithLabel failed: %v", err)
	}

	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}

	if containers[0].Labels[reconcile.LabelStackPath] != labelValue {
		t.Errorf("label value: got %q, want %q", containers[0].Labels[reconcile.LabelStackPath], labelValue)
	}
}

func TestDinD_ListContainersWithLabel_MultipleContainers(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}
	client := docker.NewClient(runner, env.DockerHost)

	for i := 0; i < 3; i++ {
		_, err := runner.Run(context.Background(), "docker", "run", "-d",
			"--label", fmt.Sprintf("%s=multi-%d", reconcile.LabelStackPath, i),
			"nginx:alpine",
		)
		if err != nil {
			t.Fatalf("docker run %d failed: %v", i, err)
		}
	}

	time.Sleep(2 * time.Second)

	containers, err := client.ListContainersWithLabel(context.Background(), reconcile.LabelStackPath)
	if err != nil {
		t.Fatalf("ListContainersWithLabel failed: %v", err)
	}

	if len(containers) != 3 {
		t.Fatalf("expected 3 containers, got %d", len(containers))
	}
}

func TestDinD_ListContainersWithLabel_NoMatch(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}
	client := docker.NewClient(runner, env.DockerHost)

	containers, err := client.ListContainersWithLabel(context.Background(), "nonexistent.label")
	if err != nil {
		t.Fatalf("ListContainersWithLabel failed: %v", err)
	}
	if len(containers) != 0 {
		t.Errorf("expected 0 containers, got %d", len(containers))
	}
}

func TestDinD_ContainerCount_RealDaemon(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}
	client := docker.NewClient(runner, env.DockerHost)

	status, err := client.ContainerCount(context.Background())
	if err != nil {
		t.Fatalf("ContainerCount failed: %v", err)
	}
	if status.RunningContainers != 0 {
		t.Errorf("expected 0 containers initially, got %d", status.RunningContainers)
	}

	_, err = runner.Run(context.Background(), "docker", "run", "-d", "nginx:alpine")
	if err != nil {
		t.Fatalf("docker run failed: %v", err)
	}

	time.Sleep(2 * time.Second)

	status, err = client.ContainerCount(context.Background())
	if err != nil {
		t.Fatalf("ContainerCount failed: %v", err)
	}
	if status.RunningContainers != 1 {
		t.Errorf("expected 1 container after run, got %d", status.RunningContainers)
	}
}

func TestDinD_GetStackLabels_GroupsByStack(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	for i := 0; i < 2; i++ {
		_, err := runner.Run(context.Background(), "docker", "run", "-d",
			"--label", fmt.Sprintf("%s=grouped-stack", reconcile.LabelStackPath),
			"--label", fmt.Sprintf("%s=rev1", reconcile.LabelDesiredRevision),
			"--label", fmt.Sprintf("%s=hash1", reconcile.LabelDesiredComposeHash),
			"--label", fmt.Sprintf("%s=test deploy", reconcile.LabelDesiredCommitMessage),
			"--label", fmt.Sprintf("%s=synced", reconcile.LabelSyncStatus),
			"nginx:alpine",
		)
		if err != nil {
			t.Fatalf("docker run %d failed: %v", i, err)
		}
	}

	time.Sleep(2 * time.Second)

	client := docker.NewClient(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	labels, err := inspector.GetStackLabels(context.Background())
	if err != nil {
		t.Fatalf("GetStackLabels failed: %v", err)
	}

	if len(labels) != 1 {
		t.Fatalf("expected 1 stack group, got %d (keys: %v)", len(labels), mapKeys(labels))
	}
	meta, ok := labels["grouped-stack"]
	if !ok {
		t.Fatal("expected 'grouped-stack' in labels")
	}
	if meta.DesiredRevision != "rev1" {
		t.Errorf("revision: got %q, want 'rev1'", meta.DesiredRevision)
	}
}

func TestDinD_GetStackLabels_MultipleStacks(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	stacks := []string{"stack-alpha", "stack-beta"}
	for _, stack := range stacks {
		_, err := runner.Run(context.Background(), "docker", "run", "-d",
			"--label", fmt.Sprintf("%s=%s", reconcile.LabelStackPath, stack),
			"--label", fmt.Sprintf("%s=rev1", reconcile.LabelDesiredRevision),
			"--label", fmt.Sprintf("%s=hash1", reconcile.LabelDesiredComposeHash),
			"nginx:alpine",
		)
		if err != nil {
			t.Fatalf("docker run for %s failed: %v", stack, err)
		}
	}

	time.Sleep(2 * time.Second)

	client := docker.NewClient(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	labels, err := inspector.GetStackLabels(context.Background())
	if err != nil {
		t.Fatalf("GetStackLabels failed: %v", err)
	}

	if len(labels) != 2 {
		t.Fatalf("expected 2 stacks, got %d (keys: %v)", len(labels), mapKeys(labels))
	}
	for _, stack := range stacks {
		if _, ok := labels[stack]; !ok {
			t.Errorf("expected stack %q in labels", stack)
		}
	}
}

func TestDinD_GeneratedOverrideIsValidCompose(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n  api:\n    image: nginx:alpine\n")
	composeHash := desiredstate.ComposeHash(composeContent)

	overrideContent := reconcile.TestGenerateLabelOverride("teststack", "rev1", "test commit message", composeHash, []string{"web", "api"})

	composeFile, overrideFile, cleanup, err := reconcile.TestWriteTempComposeDir("docker-compose.yml", composeContent, overrideContent)
	if err != nil {
		t.Fatalf("WriteTempComposeDir failed: %v", err)
	}
	defer cleanup()

	out, err := runner.Run(context.Background(), "docker", "compose", "-f", composeFile, "-f", overrideFile, "config", "--quiet")
	if err != nil {
		t.Fatalf("docker compose config validation failed: %v\nOutput: %s", err, string(out))
	}
}

func TestDinD_ExtractServiceNames_MatchesRealCompose(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	composeContent := []byte("services:\n  frontend:\n    image: nginx:alpine\n  backend:\n    image: nginx:alpine\n  worker:\n    image: nginx:alpine\n")

	extracted := reconcile.TestExtractServiceNames(composeContent)

	composeFile, _, cleanup, err := reconcile.TestWriteTempComposeDir("docker-compose.yml", composeContent, "")
	if err != nil {
		t.Fatalf("WriteTempComposeDir failed: %v", err)
	}
	defer cleanup()

	out, err := runner.Run(context.Background(), "docker", "compose", "-f", composeFile, "config", "--services")
	if err != nil {
		t.Fatalf("docker compose config --services failed: %v\nOutput: %s", err, string(out))
	}

	realServices := strings.Fields(strings.TrimSpace(string(out)))

	if len(extracted) != len(realServices) {
		t.Fatalf("extractServiceNames returned %d services, docker compose returned %d\nextracted: %v\nreal: %v",
			len(extracted), len(realServices), extracted, realServices)
	}

	realSet := make(map[string]bool)
	for _, s := range realServices {
		realSet[s] = true
	}
	for _, s := range extracted {
		if !realSet[s] {
			t.Errorf("extracted service %q not in docker compose output %v", s, realServices)
		}
	}
}

func TestDinD_LabelRoundTrip(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n")
	composeHash := desiredstate.ComposeHash(composeContent)

	expectedRevision := "deadbeef"
	expectedCommitMsg := "fix: resolved the big bug"
	expectedStackPath := "apps/my-service"

	overrideContent := reconcile.TestGenerateLabelOverride(expectedStackPath, expectedRevision, expectedCommitMsg, composeHash, []string{"web"})
	composeFile, overrideFile, cleanup, err := reconcile.TestWriteTempComposeDir("docker-compose.yml", composeContent, overrideContent)
	if err != nil {
		t.Fatalf("WriteTempComposeDir failed: %v", err)
	}
	defer cleanup()

	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	err = composeRunner.ComposeUp(context.Background(), "labelrt", composeFile, overrideFile, filepath.Dir(composeFile))
	if err != nil {
		t.Fatalf("compose up failed: %v", err)
	}
	waitForContainers(t, runner, 1, 15*time.Second)

	client := docker.NewClient(runner, env.DockerHost)
	containers, err := client.ListContainersWithLabel(context.Background(), reconcile.LabelStackPath)
	if err != nil {
		t.Fatalf("ListContainersWithLabel failed: %v", err)
	}

	if len(containers) < 1 {
		t.Fatal("expected at least 1 labeled container")
	}

	c := containers[0]
	checks := map[string]string{
		reconcile.LabelStackPath:            expectedStackPath,
		reconcile.LabelDesiredRevision:      expectedRevision,
		reconcile.LabelDesiredCommitMessage: expectedCommitMsg,
		reconcile.LabelDesiredComposeHash:   composeHash,
	}
	for label, want := range checks {
		got := c.Labels[label]
		if got != want {
			t.Errorf("label %s: got %q, want %q", label, got, want)
		}
	}
	cleanupStack(t, runner, "labelrt")
}

func TestDinD_MapLabelsToMetadata_RealLabels(t *testing.T) {
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}

	composeContent := []byte("services:\n  web:\n    image: nginx:alpine\n")
	composeHash := desiredstate.ComposeHash(composeContent)

	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{
		Revision:      "metadata-rev",
		CommitMessage: "metadata test commit",
		RefreshStatus: desiredstate.RefreshStatusCompleted,
		Stacks: []desiredstate.StackRecord{
			{Path: "metaapp", ComposeFile: "docker-compose.yml", ComposeHash: composeHash, Status: desiredstate.StackSyncMissing, Content: composeContent},
		},
	})

	composeRunner := reconcile.NewDockerComposeRunner(runner, env.DockerHost)
	client := docker.NewClient(runner, env.DockerHost)
	inspector := reconcile.NewDockerContainerInspector(client)
	r := reconcile.NewReconciler(store, reconcile.DefaultPolicy(), composeRunner, inspector, reconcile.NewAckStore(), "")

	runs := r.Reconcile(context.Background())
	if len(runs) != 1 || runs[0].Result != "success" {
		t.Fatalf("deploy failed: %s", safeResult(runs))
	}
	waitForContainers(t, runner, 1, 15*time.Second)

	labels, err := inspector.GetStackLabels(context.Background())
	if err != nil {
		t.Fatalf("GetStackLabels failed: %v", err)
	}

	meta, ok := labels["metaapp"]
	if !ok {
		t.Fatalf("expected metadata for 'metaapp', got keys: %v", mapKeys(labels))
	}

	if meta.DesiredRevision != "metadata-rev" {
		t.Errorf("DesiredRevision: got %q, want 'metadata-rev'", meta.DesiredRevision)
	}
	if meta.DesiredCommitMessage != "metadata test commit" {
		t.Errorf("DesiredCommitMessage: got %q, want 'metadata test commit'", meta.DesiredCommitMessage)
	}
	if meta.DesiredComposeHash != composeHash {
		t.Errorf("DesiredComposeHash: got %q, want %q", meta.DesiredComposeHash, composeHash)
	}
	if meta.SyncStatus != "synced" {
		t.Errorf("SyncStatus: got %q, want 'synced'", meta.SyncStatus)
	}
	if meta.SyncedAt == "" {
		t.Error("SyncedAt should not be empty")
	}
	cleanupStack(t, runner, "metaapp")
}
