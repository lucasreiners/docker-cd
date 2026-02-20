package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Client queries the Docker engine via the CLI.
type Client struct {
	Runner CommandRunner
	Socket string
}

// NewClient creates a Client that talks to Docker via the given socket path or host URL.
// The socket can be a Unix socket path (e.g. "/var/run/docker.sock"),
// a full URL (e.g. "tcp://host:port" or "unix:///path"), or empty
// to rely on the DOCKER_HOST environment variable.
func NewClient(runner CommandRunner, socket string) *Client {
	return &Client{Runner: runner, Socket: socket}
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
	// Use "docker ps -q" to list running container IDs, then count lines.
	args := append(HostArgs(c.Socket), "ps", "-q")
	out, err := c.Runner.Run(ctx, "docker", args...)
	if err != nil {
		return Status{}, fmt.Errorf("docker CLI error: %w", err)
	}

	lines := strings.TrimSpace(string(out))
	count := 0
	if lines != "" {
		count = len(strings.Split(lines, "\n"))
	}

	// Validate that the count is a reasonable number.
	_ = strconv.Itoa(count)

	return Status{
		RunningContainers: count,
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
// Uses a two-step approach: docker ps to get container IDs, then docker inspect
// for reliable JSON label parsing (avoids issues with comma-separated label output).
func (c *Client) ListContainersWithLabel(ctx context.Context, labelKey string) ([]ContainerLabels, error) {
	// Step 1: Get container IDs with the label filter
	args := append(HostArgs(c.Socket),
		"ps", "-q", "--no-trunc",
		"--filter", "label="+labelKey,
	)
	out, err := c.Runner.Run(ctx, "docker", args...)
	if err != nil {
		return nil, fmt.Errorf("docker CLI error: %w", err)
	}

	ids := strings.TrimSpace(string(out))
	if ids == "" {
		return nil, nil
	}

	containerIDs := strings.Split(ids, "\n")

	// Step 2: Inspect containers for reliable label extraction
	inspectArgs := append(HostArgs(c.Socket),
		"inspect",
		"--format", "{{json .}}",
	)
	inspectArgs = append(inspectArgs, containerIDs...)

	inspectOut, err := c.Runner.Run(ctx, "docker", inspectArgs...)
	if err != nil {
		return nil, fmt.Errorf("docker inspect error: %w", err)
	}

	var result []ContainerLabels
	// docker inspect with multiple IDs outputs one JSON object per line
	for _, line := range strings.Split(strings.TrimSpace(string(inspectOut)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var info struct {
			ID     string `json:"Id"`
			Name   string `json:"Name"`
			Config struct {
				Labels map[string]string `json:"Labels"`
			} `json:"Config"`
		}
		if err := json.Unmarshal([]byte(line), &info); err != nil {
			continue // skip unparseable entries
		}

		name := strings.TrimPrefix(info.Name, "/")
		result = append(result, ContainerLabels{
			ContainerID:   info.ID,
			ContainerName: name,
			Labels:        info.Config.Labels,
		})
	}
	return result, nil
}
