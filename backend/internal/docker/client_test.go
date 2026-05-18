package docker_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/lucasreiners/docker-cd/internal/docker"
)

// mockDockerAPI implements docker.DockerAPI for unit tests.
type mockDockerAPI struct {
	containerListFn       func(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	containerInspectFn    func(ctx context.Context, containerID string) (types.ContainerJSON, error)
	imageInspectWithRawFn func(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
	imagePullFn           func(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
	imagesPruneFn         func(ctx context.Context, pruneFilters filters.Args) (image.PruneReport, error)
}

func (m *mockDockerAPI) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	if m.containerListFn != nil {
		return m.containerListFn(ctx, options)
	}
	return nil, nil
}

func (m *mockDockerAPI) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	if m.containerInspectFn != nil {
		return m.containerInspectFn(ctx, containerID)
	}
	return types.ContainerJSON{}, nil
}

func (m *mockDockerAPI) ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
	if m.imageInspectWithRawFn != nil {
		return m.imageInspectWithRawFn(ctx, imageID)
	}
	return types.ImageInspect{}, nil, nil
}

func (m *mockDockerAPI) ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error) {
	if m.imagePullFn != nil {
		return m.imagePullFn(ctx, refStr, options)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *mockDockerAPI) ImagesPrune(ctx context.Context, pruneFilters filters.Args) (image.PruneReport, error) {
	if m.imagesPruneFn != nil {
		return m.imagesPruneFn(ctx, pruneFilters)
	}
	return image.PruneReport{}, nil
}

// --- ContainerCount tests (now SDK-based) ---

func TestContainerCount_ThreeRunning(t *testing.T) {
	api := &mockDockerAPI{
		containerListFn: func(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
			return []types.Container{{ID: "abc"}, {ID: "def"}, {ID: "ghi"}}, nil
		},
	}
	client := docker.NewClient(api)

	status, err := client.ContainerCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.RunningContainers != 3 {
		t.Errorf("expected 3 running containers, got %d", status.RunningContainers)
	}
}

func TestContainerCount_ZeroRunning(t *testing.T) {
	api := &mockDockerAPI{
		containerListFn: func(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
			return nil, nil
		},
	}
	client := docker.NewClient(api)

	status, err := client.ContainerCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.RunningContainers != 0 {
		t.Errorf("expected 0 running containers, got %d", status.RunningContainers)
	}
}

func TestContainerCount_OneRunning(t *testing.T) {
	api := &mockDockerAPI{
		containerListFn: func(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
			return []types.Container{{ID: "abc"}}, nil
		},
	}
	client := docker.NewClient(api)

	status, err := client.ContainerCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.RunningContainers != 1 {
		t.Errorf("expected 1 running container, got %d", status.RunningContainers)
	}
}

func TestContainerCount_APIError(t *testing.T) {
	api := &mockDockerAPI{
		containerListFn: func(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	client := docker.NewClient(api)

	_, err := client.ContainerCount(context.Background())
	if err == nil {
		t.Fatal("expected error from API failure, got nil")
	}
	if !strings.Contains(err.Error(), "docker API error") {
		t.Errorf("expected error to contain 'docker API error', got %q", err.Error())
	}
}

func TestContainerCount_RetrievedAtSet(t *testing.T) {
	api := &mockDockerAPI{
		containerListFn: func(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
			return []types.Container{{ID: "abc"}}, nil
		},
	}
	client := docker.NewClient(api)

	status, err := client.ContainerCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.RetrievedAt.IsZero() {
		t.Error("expected RetrievedAt to be set, got zero time")
	}
}

// --- ListContainersWithLabel tests (now SDK-based) ---

func TestListContainersWithLabel_Empty(t *testing.T) {
	api := &mockDockerAPI{
		containerListFn: func(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
			return nil, nil
		},
	}
	client := docker.NewClient(api)

	containers, err := client.ListContainersWithLabel(context.Background(), "com.docker-cd.stack.path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containers != nil {
		t.Errorf("expected nil for no containers, got %v", containers)
	}
}

func TestListContainersWithLabel_SingleContainer(t *testing.T) {
	api := &mockDockerAPI{
		containerListFn: func(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
			return []types.Container{
				{
					ID:     "abc123",
					Names:  []string{"/my-app"},
					Labels: map[string]string{"com.docker-cd.stack.path": "app1", "com.docker-cd.sync.status": "synced"},
				},
			}, nil
		},
	}
	client := docker.NewClient(api)

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
	api := &mockDockerAPI{
		containerListFn: func(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
			return []types.Container{
				{ID: "abc123", Names: []string{"/app1-web"}, Labels: map[string]string{"com.docker-cd.stack.path": "app1"}},
				{ID: "abc456", Names: []string{"/app2-web"}, Labels: map[string]string{"com.docker-cd.stack.path": "app2"}},
			}, nil
		},
	}
	client := docker.NewClient(api)

	containers, err := client.ListContainersWithLabel(context.Background(), "com.docker-cd.stack.path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}
}

func TestListContainersWithLabel_APIError(t *testing.T) {
	api := &mockDockerAPI{
		containerListFn: func(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	client := docker.NewClient(api)

	_, err := client.ListContainersWithLabel(context.Background(), "com.docker-cd.stack.path")
	if err == nil {
		t.Fatal("expected error from API failure, got nil")
	}
	if !strings.Contains(err.Error(), "docker API error") {
		t.Errorf("expected docker API error, got %q", err.Error())
	}
}

// --- HostArgs tests (unchanged) ---

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

// --- PullImages tests (now SDK-based) ---

func TestPullImages_Success(t *testing.T) {
	api := &mockDockerAPI{
		imagePullFn: func(_ context.Context, refStr string, _ image.PullOptions) (io.ReadCloser, error) {
			stream := `{"status":"Pulling from library/nginx","id":"latest"}` + "\n" +
				`{"status":"Pull complete","id":"abc123"}` + "\n"
			return io.NopCloser(strings.NewReader(stream)), nil
		},
	}
	client := docker.NewClient(api)

	var progress []docker.PullProgress
	err := client.PullImages(context.Background(), []string{"nginx:latest"}, func(p docker.PullProgress) {
		progress = append(progress, p)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have at least the final "Pull complete" event
	if len(progress) == 0 {
		t.Fatal("expected at least one progress event")
	}
	last := progress[len(progress)-1]
	if last.Status != "Pull complete" {
		t.Errorf("expected last status 'Pull complete', got %q", last.Status)
	}
	if last.Image != "nginx:latest" {
		t.Errorf("expected image 'nginx:latest', got %q", last.Image)
	}
	if last.Current != 1 || last.Total != 1 {
		t.Errorf("expected 1/1, got %d/%d", last.Current, last.Total)
	}
}

func TestPullImages_Error(t *testing.T) {
	api := &mockDockerAPI{
		imagePullFn: func(_ context.Context, _ string, _ image.PullOptions) (io.ReadCloser, error) {
			return nil, fmt.Errorf("unauthorized")
		},
	}
	client := docker.NewClient(api)

	err := client.PullImages(context.Background(), []string{"private/image:latest"}, nil)
	if err == nil {
		t.Fatal("expected error from pull failure, got nil")
	}
	if !strings.Contains(err.Error(), "pull private/image:latest failed") {
		t.Errorf("expected error to contain image name, got %q", err.Error())
	}
}

func TestPullImages_StreamError(t *testing.T) {
	api := &mockDockerAPI{
		imagePullFn: func(_ context.Context, _ string, _ image.PullOptions) (io.ReadCloser, error) {
			stream := `{"error":"image not found"}` + "\n"
			return io.NopCloser(strings.NewReader(stream)), nil
		},
	}
	client := docker.NewClient(api)

	err := client.PullImages(context.Background(), []string{"nonexistent:latest"}, nil)
	if err == nil {
		t.Fatal("expected error from stream error event")
	}
	if !strings.Contains(err.Error(), "image not found") {
		t.Errorf("expected 'image not found' in error, got %q", err.Error())
	}
}

func TestPullImages_NilCallback(t *testing.T) {
	api := &mockDockerAPI{
		imagePullFn: func(_ context.Context, _ string, _ image.PullOptions) (io.ReadCloser, error) {
			stream := `{"status":"Pull complete"}` + "\n"
			return io.NopCloser(strings.NewReader(stream)), nil
		},
	}
	client := docker.NewClient(api)

	err := client.PullImages(context.Background(), []string{"nginx:latest"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPullImages_MultipleImages(t *testing.T) {
	pullCount := 0
	api := &mockDockerAPI{
		imagePullFn: func(_ context.Context, refStr string, _ image.PullOptions) (io.ReadCloser, error) {
			pullCount++
			stream := fmt.Sprintf(`{"status":"Pull complete","id":"%s"}`, refStr) + "\n"
			return io.NopCloser(strings.NewReader(stream)), nil
		},
	}
	client := docker.NewClient(api)

	var events []docker.PullProgress
	err := client.PullImages(context.Background(), []string{"nginx:latest", "redis:7"}, func(p docker.PullProgress) {
		events = append(events, p)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pullCount != 2 {
		t.Errorf("expected 2 pull calls, got %d", pullCount)
	}
	// Check the final events have correct Current/Total
	found := map[string]bool{}
	for _, e := range events {
		if e.Status == "Pull complete" {
			found[e.Image] = true
			if e.Total != 2 {
				t.Errorf("expected total=2, got %d for %s", e.Total, e.Image)
			}
		}
	}
	if !found["nginx:latest"] || !found["redis:7"] {
		t.Errorf("expected both images in progress events, got %v", found)
	}
}

// --- PruneImages tests (now SDK-based) ---

func TestPruneImages_Success(t *testing.T) {
	api := &mockDockerAPI{
		imagesPruneFn: func(_ context.Context, _ filters.Args) (image.PruneReport, error) {
			return image.PruneReport{
				ImagesDeleted: []image.DeleteResponse{
					{Deleted: "sha256:abc123"},
					{Deleted: "sha256:def456"},
					{Untagged: "nginx:latest"},
				},
				SpaceReclaimed: 1234000000, // ~1.234GB
			}, nil
		},
	}
	client := docker.NewClient(api)

	removed, space, err := client.PruneImages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if space != "1.234GB" {
		t.Errorf("expected space '1.234GB', got %q", space)
	}
	if removed != 3 {
		t.Errorf("expected 3 images removed, got %d", removed)
	}
}

func TestPruneImages_NoImagesRemoved(t *testing.T) {
	api := &mockDockerAPI{
		imagesPruneFn: func(_ context.Context, _ filters.Args) (image.PruneReport, error) {
			return image.PruneReport{
				SpaceReclaimed: 0,
			}, nil
		},
	}
	client := docker.NewClient(api)

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
	api := &mockDockerAPI{
		imagesPruneFn: func(_ context.Context, _ filters.Args) (image.PruneReport, error) {
			return image.PruneReport{}, fmt.Errorf("permission denied")
		},
	}
	client := docker.NewClient(api)

	_, _, err := client.PruneImages(context.Background())
	if err == nil {
		t.Fatal("expected error from prune failure, got nil")
	}
	if !strings.Contains(err.Error(), "docker image prune failed") {
		t.Errorf("expected error to contain 'docker image prune failed', got %q", err.Error())
	}
}

// --- GetImageDigest tests (now SDK-based) ---

func TestGetImageDigest_Success(t *testing.T) {
	api := &mockDockerAPI{
		imageInspectWithRawFn: func(_ context.Context, _ string) (types.ImageInspect, []byte, error) {
			return types.ImageInspect{ID: "sha256:abc123def456"}, nil, nil
		},
	}
	client := docker.NewClient(api)

	result, err := client.GetImageDigest(context.Background(), "nginx:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "sha256:abc123def456" {
		t.Errorf("expected digest %q, got %q", "sha256:abc123def456", result)
	}
}

func TestGetImageDigest_Error(t *testing.T) {
	api := &mockDockerAPI{
		imageInspectWithRawFn: func(_ context.Context, _ string) (types.ImageInspect, []byte, error) {
			return types.ImageInspect{}, nil, fmt.Errorf("image not found")
		},
	}
	client := docker.NewClient(api)

	_, err := client.GetImageDigest(context.Background(), "nonexistent:image")
	if err == nil {
		t.Fatal("expected error from inspect failure, got nil")
	}
	if !strings.Contains(err.Error(), "docker inspect failed") {
		t.Errorf("expected error to contain 'docker inspect failed', got %q", err.Error())
	}
}
