package scheduler

import (
	"testing"
	"time"
)

// TestNewUpdateCycle verifies UpdateCycle creation
func TestNewUpdateCycle(t *testing.T) {
	cycle := NewUpdateCycle()

	if cycle == nil {
		t.Fatal("Expected non-nil update cycle")
	}

	if cycle.CycleID == "" {
		t.Error("Expected non-empty cycle ID")
	}

	if cycle.StartTime.IsZero() {
		t.Error("Expected non-zero start time")
	}

	if !cycle.EndTime.IsZero() {
		t.Error("Expected zero end time for new cycle")
	}

	if cycle.StacksProcessed != 0 {
		t.Error("Expected zero stacks processed")
	}
}

// TestUpdateCycleComplete verifies Complete() method
func TestUpdateCycleComplete(t *testing.T) {
	cycle := NewUpdateCycle()
	startTime := cycle.StartTime

	time.Sleep(10 * time.Millisecond) // Ensure some time passes

	cycle.Complete()

	if cycle.EndTime.IsZero() {
		t.Error("Expected non-zero end time after Complete()")
	}

	if !cycle.EndTime.After(startTime) {
		t.Error("Expected end time to be after start time")
	}

	duration := cycle.EndTime.Sub(startTime)
	if duration < 10*time.Millisecond {
		t.Errorf("Expected duration >= 10ms, got %v", duration)
	}
}

// TestUpdateCycleFields verifies all fields can be set
func TestUpdateCycleFields(t *testing.T) {
	cycle := NewUpdateCycle()

	cycle.StacksProcessed = 5
	cycle.ImagesPulled = 10
	cycle.ContainersUpdated = 8
	cycle.ImagesPruned = 3
	cycle.SpaceReclaimed = "1.2GB"

	if cycle.StacksProcessed != 5 {
		t.Errorf("Expected 5 stacks processed, got %d", cycle.StacksProcessed)
	}

	if cycle.ImagesPulled != 10 {
		t.Errorf("Expected 10 images pulled, got %d", cycle.ImagesPulled)
	}

	if cycle.ContainersUpdated != 8 {
		t.Errorf("Expected 8 containers updated, got %d", cycle.ContainersUpdated)
	}

	if cycle.ImagesPruned != 3 {
		t.Errorf("Expected 3 images pruned, got %d", cycle.ImagesPruned)
	}

	if cycle.SpaceReclaimed != "1.2GB" {
		t.Errorf("Expected '1.2GB' space reclaimed, got %s", cycle.SpaceReclaimed)
	}
}

// TestStackUpdateResult verifies StackUpdateResult structure
func TestStackUpdateResult(t *testing.T) {
	result := StackUpdateResult{
		StackName:   "test-stack",
		ProjectName: "test-project",
		Success:     true,
	}

	if result.StackName != "test-stack" {
		t.Errorf("Expected stack name 'test-stack', got %s", result.StackName)
	}

	if result.ProjectName != "test-project" {
		t.Errorf("Expected project name 'test-project', got %s", result.ProjectName)
	}

	if !result.Success {
		t.Error("Expected success to be true")
	}
}

// TestImagePullResult verifies ImagePullResult structure
func TestImagePullResult(t *testing.T) {
	result := ImagePullResult{
		ImageName:      "nginx:latest",
		PreviousDigest: "sha256:abc123",
		NewDigest:      "sha256:def456",
		Changed:        true,
		Success:        true,
		Duration:       5 * time.Second,
	}

	if result.ImageName != "nginx:latest" {
		t.Errorf("Expected image name 'nginx:latest', got %s", result.ImageName)
	}

	if result.PreviousDigest != "sha256:abc123" {
		t.Errorf("Expected previous digest 'sha256:abc123', got %s", result.PreviousDigest)
	}

	if result.NewDigest != "sha256:def456" {
		t.Errorf("Expected new digest 'sha256:def456', got %s", result.NewDigest)
	}

	if !result.Changed {
		t.Error("Expected changed to be true")
	}

	if !result.Success {
		t.Error("Expected success to be true")
	}

	if result.Duration != 5*time.Second {
		t.Errorf("Expected duration 5s, got %v", result.Duration)
	}
}

// TestUpdateOperationLogEntry verifies UpdateOperationLogEntry structure
func TestUpdateOperationLogEntry(t *testing.T) {
	now := time.Now()
	metadata := map[string]interface{}{
		"stack":  "test-stack",
		"images": 3,
	}

	entry := UpdateOperationLogEntry{
		Timestamp: now,
		Level:     "INFO",
		Operation: "image_pull",
		Subject:   "test-stack",
		Status:    "success",
		Message:   "Images pulled successfully",
		Metadata:  metadata,
	}

	if !entry.Timestamp.Equal(now) {
		t.Error("Expected timestamp to match")
	}

	if entry.Level != "INFO" {
		t.Errorf("Expected level 'INFO', got %s", entry.Level)
	}

	if entry.Operation != "image_pull" {
		t.Errorf("Expected operation 'image_pull', got %s", entry.Operation)
	}

	if entry.Subject != "test-stack" {
		t.Errorf("Expected subject 'test-stack', got %s", entry.Subject)
	}

	if entry.Status != "success" {
		t.Errorf("Expected status 'success', got %s", entry.Status)
	}

	if entry.Message != "Images pulled successfully" {
		t.Errorf("Expected message 'Images pulled successfully', got %s", entry.Message)
	}

	if len(entry.Metadata) != 2 {
		t.Errorf("Expected 2 metadata entries, got %d", len(entry.Metadata))
	}

	if entry.Metadata["stack"] != "test-stack" {
		t.Error("Expected metadata stack to be 'test-stack'")
	}
}
