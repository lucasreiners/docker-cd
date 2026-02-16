package reconcile

import (
	"context"

	"github.com/lucasreiners/docker-cd/internal/docker"
)

// DockerContainerInspector implements ContainerInspector using the Docker CLI.
type DockerContainerInspector struct {
	Client *docker.Client
}

// NewDockerContainerInspector creates an inspector that reads container labels.
func NewDockerContainerInspector(client *docker.Client) *DockerContainerInspector {
	return &DockerContainerInspector{Client: client}
}

// GetStackLabels lists all containers with docker-cd labels and groups them by stack path.
// For each stack path, it returns the sync metadata from the first container found.
func (d *DockerContainerInspector) GetStackLabels(ctx context.Context) (map[string]StackSyncMetadata, error) {
	containers, err := d.Client.ListContainersWithLabel(ctx, LabelStackPath)
	if err != nil {
		return nil, err
	}

	result := make(map[string]StackSyncMetadata)

	for _, c := range containers {
		stackPath := c.Labels[LabelStackPath]
		if stackPath == "" {
			continue
		}

		// Use the first container's labels for a stack path
		if _, exists := result[stackPath]; exists {
			continue
		}

		result[stackPath] = StackSyncMetadata{
			StackPath:            stackPath,
			DesiredRevision:      c.Labels[LabelDesiredRevision],
			DesiredCommitMessage: c.Labels[LabelDesiredCommitMessage],
			DesiredComposeHash:   c.Labels[LabelDesiredComposeHash],
			SyncedAt:             c.Labels[LabelSyncedAt],
			LastSyncAt:           c.Labels[LabelSyncAt],
			SyncStatus:           c.Labels[LabelSyncStatus],
			SyncError:            c.Labels[LabelSyncError],
		}
	}

	return result, nil
}

// MapLabelsToMetadata converts a map of docker labels to StackSyncMetadata.
func MapLabelsToMetadata(labels map[string]string) StackSyncMetadata {
	return StackSyncMetadata{
		StackPath:            labels[LabelStackPath],
		DesiredRevision:      labels[LabelDesiredRevision],
		DesiredCommitMessage: labels[LabelDesiredCommitMessage],
		DesiredComposeHash:   labels[LabelDesiredComposeHash],
		SyncedAt:             labels[LabelSyncedAt],
		LastSyncAt:           labels[LabelSyncAt],
		SyncStatus:           labels[LabelSyncStatus],
		SyncError:            labels[LabelSyncError],
	}
}
