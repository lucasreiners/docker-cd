package docker

import "time"

// Status represents a snapshot of Docker engine state.
type Status struct {
	RunningContainers int
	RetrievedAt       time.Time
}
