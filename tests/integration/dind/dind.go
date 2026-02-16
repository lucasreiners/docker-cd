package dind

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Env holds connection details for an isolated DinD daemon.
type Env struct {
	// DockerHost is the DOCKER_HOST value (e.g. "tcp://localhost:2375").
	DockerHost string
	// Container is the underlying testcontainer.
	Container testcontainers.Container
}

// Start spins up a docker:dind container and returns an Env.
func Start(ctx context.Context) (*Env, error) {
	req := testcontainers.ContainerRequest{
		Image:        "docker:27-dind",
		Privileged:   true,
		ExposedPorts: []string{"2375/tcp", "2376/tcp"},
		Env: map[string]string{
			"DOCKER_TLS_CERTDIR": "",
		},
		WaitingFor: wait.ForLog("API listen on").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start dind container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("get dind host: %w", err)
	}

	port, err := container.MappedPort(ctx, "2375")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("get dind port: %w", err)
	}

	dockerHost := fmt.Sprintf("tcp://%s:%s", host, port.Port())

	return &Env{
		DockerHost: dockerHost,
		Container:  container,
	}, nil
}

// StartT is a test-friendly wrapper around Start.
func StartT(t *testing.T) *Env {
	t.Helper()

	if os.Getenv("SKIP_DIND_TESTS") == "1" {
		t.Skip("SKIP_DIND_TESTS=1, skipping DinD integration test")
	}

	ctx := context.Background()
	env, err := Start(ctx)
	if err != nil {
		t.Fatalf("failed to start DinD environment: %v", err)
	}

	t.Cleanup(func() {
		env.Cleanup()
	})

	return env
}

// Cleanup terminates the DinD container.
func (e *Env) Cleanup() {
	if e.Container != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = e.Container.Terminate(ctx)
	}
}
