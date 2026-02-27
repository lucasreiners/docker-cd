package scheduler

import (
	"log/slog"
	"os"
	"testing"

	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
)

// TestNewSchedulerService_Disabled verifies scheduler is nil when disabled
func TestNewSchedulerService_Disabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: false,
	}

	store := desiredstate.NewStore()
	runner := &docker.ExecRunner{}
	dockerClient := docker.NewClient(runner, cfg.DockerSocket)

	svc, err := NewSchedulerService(cfg, logger, store, dockerClient, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if svc != nil {
		t.Fatal("Expected nil scheduler when disabled")
	}
}

// TestNewSchedulerService_ValidCron verifies scheduler accepts valid cron expressions
func TestNewSchedulerService_ValidCron(t *testing.T) {
	tests := []struct {
		name string
		cron string
	}{
		{"default schedule", "0 3 * * *"},
		{"every 6 hours", "0 */6 * * *"},
		{"weekdays only", "0 4 * * 1-5"},
		{"monthly", "0 2 1 * *"},
		{"every minute", "* * * * *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
			cfg := config.Config{
				UpdaterEnabled: true,
				UpdaterCron:    tt.cron,
			}

			store := desiredstate.NewStore()
			runner := &docker.ExecRunner{}
			dockerClient := docker.NewClient(runner, cfg.DockerSocket)

			svc, err := NewSchedulerService(cfg, logger, store, dockerClient, nil)
			if err != nil {
				t.Fatalf("Unexpected error for valid cron %q: %v", tt.cron, err)
			}

			if svc == nil {
				t.Fatal("Expected non-nil scheduler when enabled")
			}

			if svc.config.UpdaterCron != tt.cron {
				t.Errorf("Expected cron %q, got %q", tt.cron, svc.config.UpdaterCron)
			}
		})
	}
}

// TestNewSchedulerService_InvalidCron verifies fallback to default for invalid expressions
func TestNewSchedulerService_InvalidCron(t *testing.T) {
	tests := []struct {
		name       string
		cron       string
		expectCron string
	}{
		{"empty string", "", "0 3 * * *"},
		{"garbage text", "not-a-cron", "0 3 * * *"},
		{"too many fields", "* * * * * * *", "0 3 * * *"},
		{"invalid syntax", "60 25 * * *", "0 3 * * *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
			cfg := config.Config{
				UpdaterEnabled: true,
				UpdaterCron:    tt.cron,
			}

			store := desiredstate.NewStore()
			runner := &docker.ExecRunner{}
			dockerClient := docker.NewClient(runner, cfg.DockerSocket)

			svc, err := NewSchedulerService(cfg, logger, store, dockerClient, nil)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if svc == nil {
				t.Fatal("Expected non-nil scheduler when enabled")
			}

			if svc.config.UpdaterCron != tt.expectCron {
				t.Errorf("Expected fallback to %q, got %q", tt.expectCron, svc.config.UpdaterCron)
			}
		})
	}
}

// TestNewSchedulerService_NilDependencies verifies scheduler handles nil dependencies gracefully
func TestNewSchedulerService_NilDependencies(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{
		UpdaterEnabled: true,
		UpdaterCron:    "0 3 * * *",
	}

	// Should not panic with nil dependencies
	svc, err := NewSchedulerService(cfg, logger, nil, nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if svc == nil {
		t.Fatal("Expected non-nil scheduler when enabled")
	}
}
