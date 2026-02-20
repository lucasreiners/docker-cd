package docker_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/lucasreiners/docker-cd/internal/docker"
)

type stubRunner struct {
	output []byte
	err    error
}

func (s *stubRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return s.output, s.err
}

// multiStubRunner returns different outputs for sequential calls.
type multiStubRunner struct {
	outputs [][]byte
	errs    []error
	call    int
}

func (m *multiStubRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	i := m.call
	m.call++
	if i >= len(m.outputs) {
		return nil, fmt.Errorf("unexpected call %d", i)
	}
	var err error
	if i < len(m.errs) {
		err = m.errs[i]
	}
	return m.outputs[i], err
}

func TestContainerCount_ThreeRunning(t *testing.T) {
	runner := &stubRunner{output: []byte("abc123\ndef456\nghi789\n")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	status, err := client.ContainerCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.RunningContainers != 3 {
		t.Errorf("expected 3 running containers, got %d", status.RunningContainers)
	}
}

func TestContainerCount_ZeroRunning(t *testing.T) {
	runner := &stubRunner{output: []byte("")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	status, err := client.ContainerCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.RunningContainers != 0 {
		t.Errorf("expected 0 running containers, got %d", status.RunningContainers)
	}
}

func TestContainerCount_OneRunning(t *testing.T) {
	runner := &stubRunner{output: []byte("abc123\n")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	status, err := client.ContainerCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.RunningContainers != 1 {
		t.Errorf("expected 1 running container, got %d", status.RunningContainers)
	}
}

func TestContainerCount_CLIError(t *testing.T) {
	runner := &stubRunner{output: []byte("permission denied"), err: fmt.Errorf("exit status 1")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	_, err := client.ContainerCount(context.Background())
	if err == nil {
		t.Fatal("expected error from CLI failure, got nil")
	}
	if !strings.Contains(err.Error(), "docker CLI error") {
		t.Errorf("expected error to contain 'docker CLI error', got %q", err.Error())
	}
}

func TestContainerCount_RetrievedAtSet(t *testing.T) {
	runner := &stubRunner{output: []byte("abc\n")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	status, err := client.ContainerCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.RetrievedAt.IsZero() {
		t.Error("expected RetrievedAt to be set, got zero time")
	}
}

func TestListContainersWithLabel_Empty(t *testing.T) {
	runner := &stubRunner{output: []byte("")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	containers, err := client.ListContainersWithLabel(context.Background(), "com.docker-cd.stack.path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containers != nil {
		t.Errorf("expected nil for no containers, got %v", containers)
	}
}

func TestListContainersWithLabel_SingleContainer(t *testing.T) {
	psOutput := "abc123\n"
	inspectOutput := `{"Id":"abc123","Name":"/my-app","Config":{"Labels":{"com.docker-cd.stack.path":"app1","com.docker-cd.sync.status":"synced"}}}` + "\n"
	runner := &multiStubRunner{
		outputs: [][]byte{[]byte(psOutput), []byte(inspectOutput)},
	}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	containers, err := client.ListContainersWithLabel(context.Background(), "com.docker-cd.stack.path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if containers[0].ContainerID != "abc123" {
		t.Errorf("expected container ID abc123, got %q", containers[0].ContainerID)
	}
	if containers[0].ContainerName != "my-app" {
		t.Errorf("expected container name my-app, got %q", containers[0].ContainerName)
	}
	if containers[0].Labels["com.docker-cd.stack.path"] != "app1" {
		t.Errorf("expected stack path label app1, got %q", containers[0].Labels["com.docker-cd.stack.path"])
	}
	if containers[0].Labels["com.docker-cd.sync.status"] != "synced" {
		t.Errorf("expected sync status synced, got %q", containers[0].Labels["com.docker-cd.sync.status"])
	}
}

func TestListContainersWithLabel_MultipleContainers(t *testing.T) {
	psOutput := "abc123\nabc456\n"
	inspectOutput := `{"Id":"abc123","Name":"/app1-web","Config":{"Labels":{"com.docker-cd.stack.path":"app1"}}}` + "\n" +
		`{"Id":"abc456","Name":"/app2-web","Config":{"Labels":{"com.docker-cd.stack.path":"app2"}}}` + "\n"
	runner := &multiStubRunner{
		outputs: [][]byte{[]byte(psOutput), []byte(inspectOutput)},
	}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	containers, err := client.ListContainersWithLabel(context.Background(), "com.docker-cd.stack.path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}
}

func TestListContainersWithLabel_CLIError(t *testing.T) {
	runner := &stubRunner{output: []byte("error"), err: fmt.Errorf("exit status 1")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	_, err := client.ListContainersWithLabel(context.Background(), "com.docker-cd.stack.path")
	if err == nil {
		t.Fatal("expected error from CLI failure, got nil")
	}
	if !strings.Contains(err.Error(), "docker CLI error") {
		t.Errorf("expected docker CLI error, got %q", err.Error())
	}
}
