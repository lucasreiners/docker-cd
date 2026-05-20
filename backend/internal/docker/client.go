package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

// Client queries the Docker engine via the SDK.
type Client struct {
	API DockerAPI // SDK client for container/image operations
}

// NewClient creates a Client that talks to Docker via the SDK.
func NewClient(api DockerAPI) *Client {
	return &Client{API: api}
}

// HostArgs returns the -H flag arguments for docker CLI commands.
// Supports Unix socket paths, tcp:// URLs, unix:// URLs, or empty (no flag).
func HostArgs(socket string) []string {
	if socket == "" {
		return nil
	}
	if strings.HasPrefix(socket, "tcp://") || strings.HasPrefix(socket, "unix://") {
		return []string{"-H", socket}
	}
	return []string{"-H", "unix://" + socket}
}

// ContainerCount returns the number of currently running containers.
func (c *Client) ContainerCount(ctx context.Context) (Status, error) {
	containers, err := c.API.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return Status{}, fmt.Errorf("docker API error: %w", err)
	}

	return Status{
		RunningContainers: len(containers),
		RetrievedAt:       time.Now(),
	}, nil
}

// ContainerLabels represents a running container's labels.
type ContainerLabels struct {
	ContainerID   string
	ContainerName string
	Labels        map[string]string
}

// ListContainersWithLabel lists running containers that have the given label key set.
// Returns container ID, name, and all labels for each matching container.
func (c *Client) ListContainersWithLabel(ctx context.Context, labelKey string) ([]ContainerLabels, error) {
	containers, err := c.API.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("label", labelKey)),
	})
	if err != nil {
		return nil, fmt.Errorf("docker API error: %w", err)
	}

	if len(containers) == 0 {
		return nil, nil
	}

	result := make([]ContainerLabels, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		result = append(result, ContainerLabels{
			ContainerID:   c.ID,
			ContainerName: name,
			Labels:        c.Labels,
		})
	}
	return result, nil
}
