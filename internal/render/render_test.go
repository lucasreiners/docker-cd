package render_test

import (
	"strings"
	"testing"

	"github.com/lucasreiners/docker-cd/internal/render"
)

func TestStatusPage_ContainsProjectName(t *testing.T) {
	page := render.StatusPage("Docker-CD", 5, nil)
	if !strings.Contains(page, "Docker-CD") {
		t.Errorf("status page should contain project name, got:\n%s", page)
	}
}

func TestStatusPage_ContainsContainerCount(t *testing.T) {
	page := render.StatusPage("Docker-CD", 42, nil)
	if !strings.Contains(page, "Running containers: 42") {
		t.Errorf("status page should contain 'Running containers: 42', got:\n%s", page)
	}
}

func TestStatusPage_ZeroContainers(t *testing.T) {
	page := render.StatusPage("Docker-CD", 0, nil)
	if !strings.Contains(page, "Running containers: 0") {
		t.Errorf("status page should contain 'Running containers: 0', got:\n%s", page)
	}
}

func TestStatusPage_ContainsASCIIArt(t *testing.T) {
	page := render.StatusPage("Docker-CD", 1, nil)
	if !strings.Contains(page, "____") {
		t.Errorf("status page should contain ASCII art banner, got:\n%s", page)
	}
}

func TestStatusPage_CustomProjectName(t *testing.T) {
	page := render.StatusPage("MyProject", 3, nil)
	if !strings.Contains(page, "MyProject") {
		t.Errorf("status page should contain custom project name, got:\n%s", page)
	}
}

func TestStatusPage_WithRepoInfo(t *testing.T) {
	repo := &render.RepoInfo{
		URL:       "https://github.com/org/repo.git",
		Revision:  "main",
		DeployDir: "deployments/host-a",
	}
	page := render.StatusPage("Docker-CD", 2, repo)

	if !strings.Contains(page, "Repository: https://github.com/org/repo.git") {
		t.Errorf("should contain repo URL, got:\n%s", page)
	}
	if !strings.Contains(page, "Revision: main") {
		t.Errorf("should contain revision, got:\n%s", page)
	}
	if !strings.Contains(page, "Deploy dir: deployments/host-a") {
		t.Errorf("should contain deploy dir, got:\n%s", page)
	}
}

func TestStatusPage_RepoInfoDefaultDir(t *testing.T) {
	repo := &render.RepoInfo{
		URL:      "https://github.com/org/repo.git",
		Revision: "main",
	}
	page := render.StatusPage("Docker-CD", 1, repo)

	if !strings.Contains(page, "Deploy dir: /") {
		t.Errorf("should default deploy dir to /, got:\n%s", page)
	}
}

func TestStatusPage_RepoInfoNoToken(t *testing.T) {
	repo := &render.RepoInfo{
		URL:       "https://github.com/org/repo.git",
		Revision:  "main",
		DeployDir: "prod",
	}
	page := render.StatusPage("Docker-CD", 1, repo)

	if strings.Contains(page, "token") || strings.Contains(page, "Token") {
		t.Errorf("should never contain token, got:\n%s", page)
	}
}
