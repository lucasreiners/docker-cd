package reconcile

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lucasreiners/docker-cd/internal/docker"
)

// argCapturingRunner records all calls to Run.
type argCapturingRunner struct {
	calls []runCall
	out   []byte
	err   error
}

type runCall struct {
	Name string
	Args []string
}

func (r *argCapturingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, runCall{Name: name, Args: args})
	return r.out, r.err
}

// --- ComposeUp tests ---

func TestComposeUp_BaseArgs(t *testing.T) {
	runner := &argCapturingRunner{}
	cr := NewDockerComposeRunner(runner, "/var/run/docker.sock")

	err := cr.ComposeUp(context.Background(), "myproject", "/tmp/compose.yml", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}

	args := runner.calls[0].Args
	expected := []string{"-H", "unix:///var/run/docker.sock", "compose", "-p", "myproject", "-f", "/tmp/compose.yml", "up", "-d"}
	if !sliceEqual(args, expected) {
		t.Errorf("args = %v, want %v", args, expected)
	}
}

func TestComposeUp_WithWorkDir(t *testing.T) {
	runner := &argCapturingRunner{}
	cr := NewDockerComposeRunner(runner, "")

	err := cr.ComposeUp(context.Background(), "proj", "/tmp/c.yml", "", "/work/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := runner.calls[0].Args
	if !containsSequence(args, "--project-directory", "/work/dir") {
		t.Errorf("expected --project-directory /work/dir in args: %v", args)
	}
}

func TestComposeUp_WithOverrideFile(t *testing.T) {
	runner := &argCapturingRunner{}
	cr := NewDockerComposeRunner(runner, "")

	err := cr.ComposeUp(context.Background(), "proj", "/tmp/c.yml", "/tmp/override.yml", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := runner.calls[0].Args
	// Should have two -f flags: compose file then override
	fIndices := findAllIndices(args, "-f")
	if len(fIndices) != 2 {
		t.Fatalf("expected 2 -f flags, got %d in %v", len(fIndices), args)
	}
	if args[fIndices[0]+1] != "/tmp/c.yml" {
		t.Errorf("first -f value = %s, want /tmp/c.yml", args[fIndices[0]+1])
	}
	if args[fIndices[1]+1] != "/tmp/override.yml" {
		t.Errorf("second -f value = %s, want /tmp/override.yml", args[fIndices[1]+1])
	}
}

func TestComposeUp_NoOverrideFile(t *testing.T) {
	runner := &argCapturingRunner{}
	cr := NewDockerComposeRunner(runner, "")

	_ = cr.ComposeUp(context.Background(), "proj", "/tmp/c.yml", "", "")

	args := runner.calls[0].Args
	fIndices := findAllIndices(args, "-f")
	if len(fIndices) != 1 {
		t.Errorf("expected 1 -f flag when no override, got %d in %v", len(fIndices), args)
	}
}

func TestComposeUp_ErrorWrapping(t *testing.T) {
	runner := &argCapturingRunner{out: []byte("some output"), err: errors.New("exit 1")}
	cr := NewDockerComposeRunner(runner, "")

	err := cr.ComposeUp(context.Background(), "proj", "/tmp/c.yml", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "docker compose up failed") {
		t.Errorf("error = %q, want to contain 'docker compose up failed'", err.Error())
	}
	if !strings.Contains(err.Error(), "some output") {
		t.Errorf("error = %q, want to contain CLI output", err.Error())
	}
}

func TestComposeUp_EmptySocket(t *testing.T) {
	runner := &argCapturingRunner{}
	cr := NewDockerComposeRunner(runner, "")

	_ = cr.ComposeUp(context.Background(), "proj", "/tmp/c.yml", "", "")

	args := runner.calls[0].Args
	// No -H flag when socket is empty
	for _, a := range args {
		if a == "-H" {
			t.Errorf("expected no -H flag with empty socket, got args: %v", args)
			break
		}
	}
}

// --- ComposeDown tests ---

func TestComposeDown_BaseArgs(t *testing.T) {
	runner := &argCapturingRunner{}
	cr := NewDockerComposeRunner(runner, "/var/run/docker.sock")

	err := cr.ComposeDown(context.Background(), "myproject", "/tmp/compose.yml", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := runner.calls[0].Args
	expected := []string{"-H", "unix:///var/run/docker.sock", "compose", "-p", "myproject", "-f", "/tmp/compose.yml", "down", "--remove-orphans"}
	if !sliceEqual(args, expected) {
		t.Errorf("args = %v, want %v", args, expected)
	}
}

func TestComposeDown_WithWorkDir(t *testing.T) {
	runner := &argCapturingRunner{}
	cr := NewDockerComposeRunner(runner, "")

	_ = cr.ComposeDown(context.Background(), "proj", "/tmp/c.yml", "/work")

	args := runner.calls[0].Args
	if !containsSequence(args, "--project-directory", "/work") {
		t.Errorf("expected --project-directory /work in args: %v", args)
	}
}

func TestComposeDown_EmptyComposeFile(t *testing.T) {
	runner := &argCapturingRunner{}
	cr := NewDockerComposeRunner(runner, "")

	_ = cr.ComposeDown(context.Background(), "proj", "", "")

	args := runner.calls[0].Args
	for _, a := range args {
		if a == "-f" {
			t.Errorf("expected no -f flag with empty compose file, got args: %v", args)
			break
		}
	}
}

func TestComposeDown_ErrorWrapping(t *testing.T) {
	runner := &argCapturingRunner{out: []byte("error output"), err: errors.New("exit 1")}
	cr := NewDockerComposeRunner(runner, "")

	err := cr.ComposeDown(context.Background(), "proj", "/tmp/c.yml", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "docker compose down failed") {
		t.Errorf("error = %q, want to contain 'docker compose down failed'", err.Error())
	}
}

// --- ComposePs tests ---

func TestComposePs_SingleContainer(t *testing.T) {
	jsonLine := `{"ID":"abcdef1234567890","Name":"web-1","Service":"web","State":"running","Health":"healthy","Image":"nginx:latest","Publishers":[{"URL":"","TargetPort":80,"PublishedPort":8080,"Protocol":"tcp"}]}`
	runner := &argCapturingRunner{out: []byte(jsonLine)}
	cr := NewDockerComposeRunner(runner, "")

	containers, err := cr.ComposePs(context.Background(), "proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	c := containers[0]
	if c.ID != "abcdef123456" { // truncated to 12
		t.Errorf("ID = %q, want %q", c.ID, "abcdef123456")
	}
	if c.Name != "web-1" {
		t.Errorf("Name = %q, want %q", c.Name, "web-1")
	}
	if c.Service != "web" {
		t.Errorf("Service = %q, want %q", c.Service, "web")
	}
	if c.State != "running" {
		t.Errorf("State = %q, want %q", c.State, "running")
	}
	if c.Health != "healthy" {
		t.Errorf("Health = %q, want %q", c.Health, "healthy")
	}
	if c.Ports != "8080:80/tcp" {
		t.Errorf("Ports = %q, want %q", c.Ports, "8080:80/tcp")
	}
}

func TestComposePs_MultipleContainers(t *testing.T) {
	lines := `{"ID":"aaaaaaaaaaaa1234","Name":"web-1","Service":"web","State":"running","Health":"","Image":"nginx","Publishers":[]}
{"ID":"bbbbbbbbbbbb5678","Name":"db-1","Service":"db","State":"exited","Health":"","Image":"postgres","Publishers":[]}`
	runner := &argCapturingRunner{out: []byte(lines)}
	cr := NewDockerComposeRunner(runner, "")

	containers, err := cr.ComposePs(context.Background(), "proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}
}

func TestComposePs_EmptyOutput(t *testing.T) {
	runner := &argCapturingRunner{out: []byte("")}
	cr := NewDockerComposeRunner(runner, "")

	containers, err := cr.ComposePs(context.Background(), "proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containers != nil {
		t.Errorf("expected nil, got %v", containers)
	}
}

func TestComposePs_HealthDefaultsToNone(t *testing.T) {
	jsonLine := `{"ID":"abcdef1234567890","Name":"web-1","Service":"web","State":"running","Health":"","Image":"nginx","Publishers":[]}`
	runner := &argCapturingRunner{out: []byte(jsonLine)}
	cr := NewDockerComposeRunner(runner, "")

	containers, err := cr.ComposePs(context.Background(), "proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containers[0].Health != "none" {
		t.Errorf("Health = %q, want %q", containers[0].Health, "none")
	}
}

func TestComposePs_UnpublishedPort(t *testing.T) {
	jsonLine := `{"ID":"abcdef1234567890","Name":"web-1","Service":"web","State":"running","Health":"","Image":"nginx","Publishers":[{"URL":"","TargetPort":3000,"PublishedPort":0,"Protocol":"tcp"}]}`
	runner := &argCapturingRunner{out: []byte(jsonLine)}
	cr := NewDockerComposeRunner(runner, "")

	containers, err := cr.ComposePs(context.Background(), "proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containers[0].Ports != "3000/tcp" {
		t.Errorf("Ports = %q, want %q", containers[0].Ports, "3000/tcp")
	}
}

func TestComposePs_InvalidJSONSkipped(t *testing.T) {
	lines := "not json\n" + `{"ID":"abcdef1234567890","Name":"web-1","Service":"web","State":"running","Health":"","Image":"nginx","Publishers":[]}`
	runner := &argCapturingRunner{out: []byte(lines)}
	cr := NewDockerComposeRunner(runner, "")

	containers, err := cr.ComposePs(context.Background(), "proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 1 {
		t.Errorf("expected 1 container (invalid line skipped), got %d", len(containers))
	}
}

func TestComposePs_RunnerError(t *testing.T) {
	runner := &argCapturingRunner{err: errors.New("fail")}
	cr := NewDockerComposeRunner(runner, "")

	_, err := cr.ComposePs(context.Background(), "proj")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "docker compose ps failed") {
		t.Errorf("error = %q, want to contain 'docker compose ps failed'", err.Error())
	}
}

func TestComposePs_VerifiesArgs(t *testing.T) {
	runner := &argCapturingRunner{out: []byte("")}
	cr := NewDockerComposeRunner(runner, "/sock")

	_, _ = cr.ComposePs(context.Background(), "myproj")

	args := runner.calls[0].Args
	hostArgs := docker.HostArgs("/sock")
	expected := append(hostArgs, "compose", "-p", "myproj", "ps", "-a", "--format", "json")
	if !sliceEqual(args, expected) {
		t.Errorf("args = %v, want %v", args, expected)
	}
}

// --- helpers ---

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsSequence(s []string, a, b string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == a && s[i+1] == b {
			return true
		}
	}
	return false
}

func findAllIndices(s []string, val string) []int {
	var indices []int
	for i, v := range s {
		if v == val {
			indices = append(indices, i)
		}
	}
	return indices
}
