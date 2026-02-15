package render_test

import (
	"strings"
	"testing"

	"github.com/lucasreiners/docker-cd/internal/render"
)

func TestStatusPage_ContainsProjectName(t *testing.T) {
	page := render.StatusPage("Docker-CD", 5)
	if !strings.Contains(page, "Docker-CD") {
		t.Errorf("status page should contain project name, got:\n%s", page)
	}
}

func TestStatusPage_ContainsContainerCount(t *testing.T) {
	page := render.StatusPage("Docker-CD", 42)
	if !strings.Contains(page, "Running containers: 42") {
		t.Errorf("status page should contain 'Running containers: 42', got:\n%s", page)
	}
}

func TestStatusPage_ZeroContainers(t *testing.T) {
	page := render.StatusPage("Docker-CD", 0)
	if !strings.Contains(page, "Running containers: 0") {
		t.Errorf("status page should contain 'Running containers: 0', got:\n%s", page)
	}
}

func TestStatusPage_ContainsASCIIArt(t *testing.T) {
	page := render.StatusPage("Docker-CD", 1)
	if !strings.Contains(page, "____") {
		t.Errorf("status page should contain ASCII art banner, got:\n%s", page)
	}
}

func TestStatusPage_CustomProjectName(t *testing.T) {
	page := render.StatusPage("MyProject", 3)
	if !strings.Contains(page, "MyProject") {
		t.Errorf("status page should contain custom project name, got:\n%s", page)
	}
}
