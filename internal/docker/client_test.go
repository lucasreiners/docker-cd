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
