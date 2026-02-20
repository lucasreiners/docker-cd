package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
)

// DockerComposeRunner implements ComposeRunner using the Docker CLI.
type DockerComposeRunner struct {
	Runner docker.CommandRunner
	Socket string
}

// NewDockerComposeRunner creates a compose runner that uses docker compose CLI commands.
func NewDockerComposeRunner(runner docker.CommandRunner, socket string) *DockerComposeRunner {
	return &DockerComposeRunner{Runner: runner, Socket: socket}
}

// ComposeUp runs docker compose up -d with the given project name and compose file.
// If overrideFile is not empty, it is included as an additional -f argument.
// workDir sets --project-directory so Docker Compose resolves relative paths correctly.
func (r *DockerComposeRunner) ComposeUp(ctx context.Context, projectName, composeFile, overrideFile, workDir string) error {
	args := docker.HostArgs(r.Socket)
	args = append(args, "compose", "-p", projectName)
	if workDir != "" {
		args = append(args, "--project-directory", workDir)
	}
	args = append(args, "-f", composeFile)
	if overrideFile != "" {
		args = append(args, "-f", overrideFile)
	}
	args = append(args, "up", "-d")

	out, err := r.Runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("docker compose up failed: %s: %w", string(out), err)
	}
	return nil
}

// ComposeDown runs docker compose down --remove-orphans for the given project.
// workDir sets --project-directory so Docker Compose resolves relative paths correctly.
func (r *DockerComposeRunner) ComposeDown(ctx context.Context, projectName, composeFile, workDir string) error {
	args := docker.HostArgs(r.Socket)
	args = append(args, "compose", "-p", projectName)
	if workDir != "" {
		args = append(args, "--project-directory", workDir)
	}
	if composeFile != "" {
		args = append(args, "-f", composeFile)
	}
	args = append(args, "down", "--remove-orphans")

	out, err := r.Runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("docker compose down failed: %s: %w", string(out), err)
	}
	return nil
}

// composePsJSON represents the JSON output of docker compose ps --format json.
type composePsJSON struct {
	ID         string `json:"ID"`
	Name       string `json:"Name"`
	Service    string `json:"Service"`
	State      string `json:"State"`
	Health     string `json:"Health"`
	Image      string `json:"Image"`
	Publishers []struct {
		URL           string `json:"URL"`
		TargetPort    int    `json:"TargetPort"`
		PublishedPort int    `json:"PublishedPort"`
		Protocol      string `json:"Protocol"`
	} `json:"Publishers"`
}

// ComposePs lists running containers for a compose project.
func (r *DockerComposeRunner) ComposePs(ctx context.Context, projectName string) ([]desiredstate.ContainerInfo, error) {
	args := docker.HostArgs(r.Socket)
	args = append(args, "compose", "-p", projectName, "ps", "-a", "--format", "json")

	out, err := r.Runner.Run(ctx, "docker", args...)
	if err != nil {
		return nil, fmt.Errorf("docker compose ps failed: %s: %w", string(out), err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}

	var containers []desiredstate.ContainerInfo

	// docker compose ps --format json outputs one JSON object per line
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var ps composePsJSON
		if err := json.Unmarshal([]byte(line), &ps); err != nil {
			continue
		}

		health := ps.Health
		if health == "" {
			health = "none"
		}

		var ports []string
		for _, p := range ps.Publishers {
			if p.PublishedPort > 0 {
				ports = append(ports, fmt.Sprintf("%d:%d/%s", p.PublishedPort, p.TargetPort, p.Protocol))
			} else {
				ports = append(ports, fmt.Sprintf("%d/%s", p.TargetPort, p.Protocol))
			}
		}

		containers = append(containers, desiredstate.ContainerInfo{
			ID:      ps.ID[:12], // short ID
			Name:    ps.Name,
			Service: ps.Service,
			State:   ps.State,
			Health:  health,
			Image:   ps.Image,
			Ports:   strings.Join(ports, ", "),
		})
	}

	return containers, nil
}

// generateLabelOverride creates a docker-compose override YAML that adds
// sync metadata labels to every service in the stack.
func generateLabelOverride(stackPath, revision, commitMessage, composeHash string, serviceNames []string) string {
	now := formatNow()

	if len(serviceNames) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("services:\n")
	for _, svc := range serviceNames {
		fmt.Fprintf(&b, "  %s:\n", svc)
		b.WriteString("    labels:\n")
		fmt.Fprintf(&b, "      %s: \"%s\"\n", LabelStackPath, stackPath)
		fmt.Fprintf(&b, "      %s: \"%s\"\n", LabelDesiredRevision, revision)
		fmt.Fprintf(&b, "      %s: \"%s\"\n", LabelDesiredCommitMessage, escapeYAMLValue(commitMessage))
		fmt.Fprintf(&b, "      %s: \"%s\"\n", LabelDesiredComposeHash, composeHash)
		fmt.Fprintf(&b, "      %s: \"%s\"\n", LabelSyncedAt, now)
		fmt.Fprintf(&b, "      %s: \"%s\"\n", LabelSyncAt, now)
		fmt.Fprintf(&b, "      %s: \"synced\"\n", LabelSyncStatus)
	}

	return b.String()
}

// generateLabelArgs returns docker compose label arguments for the given metadata.
func generateLabelArgs(stackPath, revision, commitMessage, composeHash string) map[string]string {
	now := formatNow()
	return map[string]string{
		LabelStackPath:            stackPath,
		LabelDesiredRevision:      revision,
		LabelDesiredCommitMessage: commitMessage,
		LabelDesiredComposeHash:   composeHash,
		LabelSyncedAt:             now,
		LabelSyncAt:               now,
		LabelSyncStatus:           "synced",
	}
}

// writeTempComposeDir creates a temp directory containing the compose file and
// override file, returning absolute paths and a cleanup function.
// This ensures docker compose receives absolute file paths regardless of CWD.
func writeTempComposeDir(composeFileName string, composeContent []byte, overrideContent string) (composeFile, overrideFile string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "docker-cd-compose-*")
	if err != nil {
		return "", "", func() {}, fmt.Errorf("create temp dir: %w", err)
	}

	composeFile = filepath.Join(tmpDir, composeFileName)
	if err := os.WriteFile(composeFile, composeContent, 0644); err != nil {
		os.RemoveAll(tmpDir)
		return "", "", func() {}, fmt.Errorf("write compose file: %w", err)
	}

	overrideFile = filepath.Join(tmpDir, "docker-cd-override.yml")
	if err := os.WriteFile(overrideFile, []byte(overrideContent), 0644); err != nil {
		os.RemoveAll(tmpDir)
		return "", "", func() {}, fmt.Errorf("write override file: %w", err)
	}

	cleanup = func() {
		os.RemoveAll(tmpDir)
	}

	return composeFile, overrideFile, cleanup, nil
}

func formatNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func escapeYAMLValue(s string) string {
	// Replace double quotes and newlines for safe YAML embedding
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// extractServiceNames parses compose file content and returns the top-level
// service names. Uses lightweight line-based parsing to avoid a YAML dependency.
// Supports any consistent indentation (2 spaces, 4 spaces, tabs, etc.).
func extractServiceNames(content []byte) []string {
	lines := strings.Split(string(content), "\n")
	inServices := false
	serviceIndent := -1 // indent level of service-name lines, detected from first service
	var names []string

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")

		// Detect the top-level "services:" key (with optional surrounding whitespace)
		if strings.TrimSpace(trimmed) == "services:" && countIndent(trimmed) == 0 {
			inServices = true
			continue
		}

		if !inServices {
			continue
		}

		// Skip empty lines and comments
		stripped := strings.TrimSpace(trimmed)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}

		indent := countIndent(trimmed)

		// A non-indented non-empty line means a new top-level key — stop.
		if indent == 0 {
			break
		}

		// First indented line under services: establishes the service indent level.
		if serviceIndent < 0 {
			serviceIndent = indent
		}

		// Only collect lines at the service indent level that end with ":"
		if indent == serviceIndent && strings.HasSuffix(stripped, ":") {
			name := strings.TrimSuffix(stripped, ":")
			if name != "" {
				names = append(names, name)
			}
		}
	}

	return names
}

// countIndent returns the effective indentation width of a line,
// treating tabs as 4 spaces.
func countIndent(line string) int {
	indent := 0
	for _, ch := range line {
		if ch == ' ' {
			indent++
		} else if ch == '\t' {
			indent += 4
		} else {
			break
		}
	}
	return indent
}
