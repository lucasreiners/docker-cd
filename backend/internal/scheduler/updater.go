package scheduler

import (
	"time"

	"github.com/google/uuid"
)

// UpdateCycle represents a single execution of the scheduled update process.
type UpdateCycle struct {
	CycleID           string
	StartTime         time.Time
	EndTime           time.Time
	StacksProcessed   int
	ImagesPulled      int
	ContainersUpdated int
	ImagesPruned      int
	SpaceReclaimed    string
	Errors            []error
}

// NewUpdateCycle creates a new update cycle with a unique ID.
func NewUpdateCycle() *UpdateCycle {
	return &UpdateCycle{
		CycleID:   uuid.New().String(),
		StartTime: time.Now(),
	}
}

// Complete marks the update cycle as finished.
func (c *UpdateCycle) Complete() {
	c.EndTime = time.Now()
}

// StackUpdateResult captures the outcome of updating a specific stack.
type StackUpdateResult struct {
	StackName          string
	ProjectName        string
	PullStartTime      time.Time
	PullEndTime        time.Time
	ImagesPulled       []ImagePullResult
	ReconcileTriggered bool
	ContainersUpdated  int
	Success            bool
	Error              error
}

// ImagePullResult records the outcome of pulling a container image.
type ImagePullResult struct {
	ImageName      string
	PreviousDigest string
	NewDigest      string
	Changed        bool
	Success        bool
	Error          error
	Duration       time.Duration
}

// UpdateOperationLogEntry represents a structured log entry for update operations.
type UpdateOperationLogEntry struct {
	Timestamp time.Time
	Level     string
	Operation string
	Subject   string
	Status    string
	Message   string
	Metadata  map[string]interface{}
	Error     string
}
