package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

// PullProgress represents progress information for an image pull operation.
type PullProgress struct {
	Image    string `json:"image"`
	Status   string `json:"status"`
	Progress string `json:"progress"`
	Current  int    `json:"current"`
	Total    int    `json:"total"`
}

// PullProgressFn is a callback invoked during image pulls with progress updates.
type PullProgressFn func(PullProgress)

// ImageOperations defines operations for managing Docker images.
type ImageOperations interface {
	// PullImages pulls the given images via the Docker SDK.
	// onProgress is called with throttled progress updates; nil is safe.
	PullImages(ctx context.Context, images []string, onProgress PullProgressFn) error

	// PruneImages removes all unused and dangling images system-wide
	PruneImages(ctx context.Context) (imagesRemoved int, spaceReclaimed string, err error)

	// GetImageDigest returns the digest of an image
	GetImageDigest(ctx context.Context, imageName string) (string, error)
}

// pullStreamEvent represents a single JSON event from the Docker image pull stream.
type pullStreamEvent struct {
	Status   string `json:"status"`
	Progress string `json:"progress"`
	ID       string `json:"id"`
	Error    string `json:"error"`
}

// PullImages pulls the given images individually via the Docker SDK.
// Progress is reported through onProgress (throttled to ~500ms per image). Nil callback is safe.
func (c *Client) PullImages(ctx context.Context, images []string, onProgress PullProgressFn) error {
	total := len(images)
	for i, img := range images {
		if err := c.pullImage(ctx, img, i+1, total, onProgress); err != nil {
			return fmt.Errorf("pull %s failed: %w", img, err)
		}
	}
	return nil
}

func (c *Client) pullImage(ctx context.Context, img string, current, total int, onProgress PullProgressFn) error {
	reader, err := c.API.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	decoder := json.NewDecoder(reader)
	var lastNotify time.Time

	for {
		var event pullStreamEvent
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if event.Error != "" {
			return fmt.Errorf("%s", event.Error)
		}
		if onProgress != nil && time.Since(lastNotify) >= 500*time.Millisecond {
			lastNotify = time.Now()
			onProgress(PullProgress{
				Image:    img,
				Status:   event.Status,
				Progress: event.Progress,
				Current:  current,
				Total:    total,
			})
		}
	}

	// Send a final "complete" progress event for this image
	if onProgress != nil {
		onProgress(PullProgress{
			Image:   img,
			Status:  "Pull complete",
			Current: current,
			Total:   total,
		})
	}
	return nil
}

// PruneImages removes all unused and dangling images system-wide.
// Returns the number of images removed and space reclaimed.
func (c *Client) PruneImages(ctx context.Context) (int, string, error) {
	report, err := c.API.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "false")))
	if err != nil {
		return 0, "", fmt.Errorf("docker image prune failed: %w", err)
	}

	imagesRemoved := len(report.ImagesDeleted)
	spaceReclaimed := formatBytes(report.SpaceReclaimed)

	return imagesRemoved, spaceReclaimed, nil
}

// formatBytes converts bytes to a human-readable string matching Docker CLI output format.
func formatBytes(bytes uint64) string {
	if bytes == 0 {
		return "0B"
	}
	const (
		kb = 1000
		mb = 1000 * kb
		gb = 1000 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.3fGB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.3fMB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.3fkB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// GetImageDigest returns the digest (SHA256 hash) of an image.
// Returns the full digest string like "sha256:abc123..."
func (c *Client) GetImageDigest(ctx context.Context, imageName string) (string, error) {
	inspect, _, err := c.API.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		return "", fmt.Errorf("docker inspect failed: %w", err)
	}

	return inspect.ID, nil
}
