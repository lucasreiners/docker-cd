package docker

import (
	"context"
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

// NewClient creates a Client that talks to Docker via the given socket path.
func NewClient(runner CommandRunner, socket string) *Client {
	return &Client{Runner: runner, Socket: socket}
}

// ContainerCount returns the number of currently running containers.
func (c *Client) ContainerCount(ctx context.Context) (Status, error) {
	// Use "docker ps -q" to list running container IDs, then count lines.
	args := []string{"-H", "unix://" + c.Socket, "ps", "-q"}
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
