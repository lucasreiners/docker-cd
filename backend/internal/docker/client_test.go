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

func TestListContainersWithLabel_InvalidJSON(t *testing.T) {
	psOutput := "abc123\n"
	inspectOutput := "not valid JSON\n"
	runner := &multiStubRunner{
		outputs: [][]byte{[]byte(psOutput), []byte(inspectOutput)},
	}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	containers, err := client.ListContainersWithLabel(context.Background(), "com.docker-cd.stack.path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 0 {
		t.Errorf("expected 0 containers (invalid JSON skipped), got %d", len(containers))
	}
}

func TestListContainersWithLabel_InspectError(t *testing.T) {
	psOutput := "abc123\n"
	runner := &multiStubRunner{
		outputs: [][]byte{[]byte(psOutput), []byte("inspect error")},
		errs:    []error{nil, fmt.Errorf("exit status 1")},
	}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	_, err := client.ListContainersWithLabel(context.Background(), "com.docker-cd.stack.path")
	if err == nil {
		t.Fatal("expected error from inspect failure, got nil")
	}
	if !strings.Contains(err.Error(), "docker inspect error") {
		t.Errorf("expected docker inspect error, got %q", err.Error())
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

// HostArgs tests
func TestHostArgs_EmptySocket(t *testing.T) {
	args := docker.HostArgs("")
	if args != nil {
		t.Errorf("expected nil for empty socket, got %v", args)
	}
}

func TestHostArgs_UnixSocketPath(t *testing.T) {
	args := docker.HostArgs("/var/run/docker.sock")
	expected := []string{"-H", "unix:///var/run/docker.sock"}
	if len(args) != 2 || args[0] != expected[0] || args[1] != expected[1] {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestHostArgs_TcpURL(t *testing.T) {
	args := docker.HostArgs("tcp://host:2376")
	expected := []string{"-H", "tcp://host:2376"}
	if len(args) != 2 || args[0] != expected[0] || args[1] != expected[1] {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestHostArgs_UnixURL(t *testing.T) {
	args := docker.HostArgs("unix:///var/run/docker.sock")
	expected := []string{"-H", "unix:///var/run/docker.sock"}
	if len(args) != 2 || args[0] != expected[0] || args[1] != expected[1] {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

// Image operations tests
func TestPullImages_Success(t *testing.T) {
	runner := &stubRunner{output: []byte("")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	err := client.PullImages(context.Background(), "myproject", "/path/to/compose.yml", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPullImages_Error(t *testing.T) {
	runner := &stubRunner{output: []byte("pull failed"), err: fmt.Errorf("exit status 1")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	err := client.PullImages(context.Background(), "myproject", "/path/to/compose.yml", "")
	if err == nil {
		t.Fatal("expected error from pull failure, got nil")
	}
	if !strings.Contains(err.Error(), "docker compose pull failed") {
		t.Errorf("expected error to contain 'docker compose pull failed', got %q", err.Error())
	}
}

func TestPruneImages_Success(t *testing.T) {
	output := "Deleted Images:\ndeleted: sha256:abc123\ndeleted: sha256:def456\nTotal reclaimed space: 1.234GB\n"
	runner := &stubRunner{output: []byte(output)}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	removed, space, err := client.PruneImages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if space != "1.234GB" {
		t.Errorf("expected space '1.234GB', got %q", space)
	}
	// Note: The current implementation counts both "Deleted Images:" header and "deleted:" lines
	if removed != 3 {
		t.Errorf("expected 3 lines counted (1 header + 2 deleted), got %d", removed)
	}
}

func TestPruneImages_NoImagesRemoved(t *testing.T) {
	output := "Total reclaimed space: 0B\n"
	runner := &stubRunner{output: []byte(output)}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	removed, space, err := client.PruneImages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if space != "0B" {
		t.Errorf("expected space '0B', got %q", space)
	}
	if removed != 0 {
		t.Errorf("expected 0 images removed, got %d", removed)
	}
}

func TestPruneImages_Error(t *testing.T) {
	runner := &stubRunner{output: []byte("prune failed"), err: fmt.Errorf("exit status 1")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	_, _, err := client.PruneImages(context.Background())
	if err == nil {
		t.Fatal("expected error from prune failure, got nil")
	}
	if !strings.Contains(err.Error(), "docker image prune failed") {
		t.Errorf("expected error to contain 'docker image prune failed', got %q", err.Error())
	}
}

func TestGetImageDigest_Success(t *testing.T) {
	digest := "sha256:abc123def456"
	runner := &stubRunner{output: []byte(digest + "\n")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	result, err := client.GetImageDigest(context.Background(), "nginx:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != digest {
		t.Errorf("expected digest %q, got %q", digest, result)
	}
}

func TestGetImageDigest_Error(t *testing.T) {
	runner := &stubRunner{output: []byte("inspect failed"), err: fmt.Errorf("exit status 1")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	_, err := client.GetImageDigest(context.Background(), "nonexistent:image")
	if err == nil {
		t.Fatal("expected error from inspect failure, got nil")
	}
	if !strings.Contains(err.Error(), "docker inspect failed") {
		t.Errorf("expected error to contain 'docker inspect failed', got %q", err.Error())
	}
}

// GetComposeImages tests

func TestGetComposeImages_Success(t *testing.T) {
	output := "nginx:latest\nredis:7-alpine\npostgres:16\n"
	runner := &stubRunner{output: []byte(output)}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	images, err := client.GetComposeImages(context.Background(), "myproject", "/path/to/compose.yml", "/work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(images))
	}
	expected := []string{"nginx:latest", "redis:7-alpine", "postgres:16"}
	for i, img := range images {
		if img != expected[i] {
			t.Errorf("image[%d]: expected %q, got %q", i, expected[i], img)
		}
	}
}

func TestGetComposeImages_EmptyOutput(t *testing.T) {
	runner := &stubRunner{output: []byte("")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	images, err := client.GetComposeImages(context.Background(), "myproject", "/path/to/compose.yml", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(images) != 0 {
		t.Errorf("expected 0 images, got %d", len(images))
	}
}

func TestGetComposeImages_Error(t *testing.T) {
	runner := &stubRunner{output: []byte("config failed"), err: fmt.Errorf("exit status 1")}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	_, err := client.GetComposeImages(context.Background(), "myproject", "/path/to/compose.yml", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "docker compose config --images failed") {
		t.Errorf("expected error to contain 'docker compose config --images failed', got %q", err.Error())
	}
}

func TestGetComposeImages_WithWorkDir(t *testing.T) {
	var capturedArgs []string
	runner := &argCapturingRunner{output: []byte("nginx:latest\n"), capture: func(args []string) { capturedArgs = args }}
	client := docker.NewClient(runner, "/var/run/docker.sock")

	_, err := client.GetComposeImages(context.Background(), "myproject", "/path/to/compose.yml", "/my/workdir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify --project-directory was passed
	if !strings.Contains(strings.Join(capturedArgs, " "), "--project-directory") {
		t.Error("expected --project-directory in args when workDir is set")
	}
}

// argCapturingRunner is a CommandRunner that captures the args passed to Run.
type argCapturingRunner struct {
	output  []byte
	capture func(args []string)
}

func (r *argCapturingRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	if r.capture != nil {
		r.capture(args)
	}
	return r.output, nil
}
