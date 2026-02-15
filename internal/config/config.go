package config

import (
	"os"
	"strconv"
)

// Config holds runtime configuration for the Docker-CD skeleton service.
type Config struct {
	Port         int
	ProjectName  string
	DockerSocket string
}

// Load reads configuration from environment variables, falling back to defaults.
func Load() Config {
	port := 8080
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			port = p
		}
	}

	projectName := "Docker-CD"
	if v := os.Getenv("PROJECT_NAME"); v != "" {
		projectName = v
	}

	dockerSocket := "/var/run/docker.sock"
	if v := os.Getenv("DOCKER_SOCKET"); v != "" {
		dockerSocket = v
	}

	return Config{
		Port:         port,
		ProjectName:  projectName,
		DockerSocket: dockerSocket,
	}
}
