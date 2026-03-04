package docker

import (
	"context"
	"fmt"
	"strings"
)

// ImageOperations defines operations for managing Docker images.
type ImageOperations interface {
	// PullImages pulls all images for a compose project
	PullImages(ctx context.Context, projectName, composeFile string) error

	// PruneImages removes all unused and dangling images system-wide
	PruneImages(ctx context.Context) (imagesRemoved int, spaceReclaimed string, err error)

	// GetImageDigest returns the digest of an image
	GetImageDigest(ctx context.Context, imageName string) (string, error)
}

// PullImages pulls all images defined in a compose file.
// Uses "docker compose pull" to handle all images in the compose file.
func (c *Client) PullImages(ctx context.Context, projectName, composeFile string) error {
	args := append(HostArgs(c.Socket),
		"compose", "-p", projectName,
		"-f", composeFile,
		"pull")

	out, err := c.Runner.Run(ctx, "docker", args...)
	if err != nil {
		output := strings.TrimSpace(string(out))
		if output != "" {
			return fmt.Errorf("docker compose pull failed: %s: %w", truncateOutput(output, 2000), err)
		}
		return fmt.Errorf("docker compose pull failed: %w", err)
	}

	return nil
}

func truncateOutput(output string, maxLen int) string {
	if maxLen <= 0 || len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + "..."
}

// PruneImages removes all unused and dangling images system-wide.
// Returns the number of images removed and space reclaimed.
func (c *Client) PruneImages(ctx context.Context) (int, string, error) {
	args := append(HostArgs(c.Socket),
		"image", "prune", "-af")

	out, err := c.Runner.Run(ctx, "docker", args...)
	if err != nil {
		return 0, "", fmt.Errorf("docker image prune failed: %w", err)
	}

	// Parse output for space reclaimed
	// Expected format: "Total reclaimed space: 1.234GB"
	output := string(out)
	spaceReclaimed := "0B"
	imagesRemoved := 0

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Total reclaimed space:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				spaceReclaimed = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "Deleted Images:") || strings.Contains(line, "deleted:") {
			// Count number of deleted lines
			imagesRemoved++
		}
	}

	return imagesRemoved, spaceReclaimed, nil
}

// GetImageDigest returns the digest (SHA256 hash) of an image.
// Returns the full digest string like "sha256:abc123..."
func (c *Client) GetImageDigest(ctx context.Context, imageName string) (string, error) {
	args := append(HostArgs(c.Socket),
		"inspect",
		"--format", "{{.Id}}",
		imageName)

	out, err := c.Runner.Run(ctx, "docker", args...)
	if err != nil {
		return "", fmt.Errorf("docker inspect failed: %w", err)
	}

	digest := strings.TrimSpace(string(out))
	return digest, nil
}
