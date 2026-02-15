package docker

import (
	"context"
	"os/exec"
)

// CommandRunner abstracts command execution for testability.
type CommandRunner interface {
	// Run executes a command and returns its combined output.
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner implements CommandRunner using os/exec.
type ExecRunner struct{}

// Run executes the given command and returns combined stdout/stderr output.
func (r *ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
