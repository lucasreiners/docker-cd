package docker

import (
	"context"
	"io"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// DockerAPI abstracts the subset of the Docker SDK client used by this package.
// Implementations include the real SDK client and test mocks.
type DockerAPI interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
	ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
	ImagesPrune(ctx context.Context, pruneFilters filters.Args) (image.PruneReport, error)
}

// NewSDKClient creates a DockerAPI backed by the real Docker SDK.
// The socket parameter can be:
//   - Empty string: uses DOCKER_HOST env or default socket
//   - A Unix socket path (e.g. "/var/run/docker.sock"): converted to "unix:///path"
//   - A full URL (e.g. "tcp://host:port" or "unix:///path"): used as-is
func NewSDKClient(socket string) (DockerAPI, error) {
	opts := []client.Opt{client.WithAPIVersionNegotiation()}

	host := socketToHost(socket)
	if host != "" {
		opts = append(opts, client.WithHost(host))
	} else {
		opts = append(opts, client.FromEnv)
	}

	return client.NewClientWithOpts(opts...)
}

// socketToHost converts a socket string to a Docker host URL.
// Returns empty string if the socket is empty (use default/env).
func socketToHost(socket string) string {
	if socket == "" {
		return ""
	}
	if strings.HasPrefix(socket, "tcp://") || strings.HasPrefix(socket, "unix://") {
		return socket
	}
	return "unix://" + socket
}
