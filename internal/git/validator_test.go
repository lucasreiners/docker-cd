package git_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/lucasreiners/docker-cd/internal/git"
)

type stubLister struct {
	refs []*plumbing.Reference
	err  error
}

func (s *stubLister) ListRefs(_ context.Context, _, _ string) ([]*plumbing.Reference, error) {
	return s.refs, s.err
}

func makeRef(name string) *plumbing.Reference {
	return plumbing.NewReferenceFromStrings(name, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
}

func TestValidate_InvalidURL(t *testing.T) {
	lister := &stubLister{}
	result := git.Validate(context.Background(), lister, "git@github.com:org/repo.git", "tok", "main", "")

	if result.Success {
		t.Fatal("expected failure for non-HTTPS URL")
	}
	if result.Error.Type != git.ErrInvalidURL {
		t.Errorf("expected ErrInvalidURL, got %d", result.Error.Type)
	}
}

func TestValidate_AuthFailure(t *testing.T) {
	lister := &stubLister{err: fmt.Errorf("authentication required")}
	result := git.Validate(context.Background(), lister, "https://github.com/org/repo.git", "bad-token", "main", "")

	if result.Success {
		t.Fatal("expected failure for auth error")
	}
	if result.Error.Type != git.ErrAuthFailed {
		t.Errorf("expected ErrAuthFailed, got %d", result.Error.Type)
	}
}

func TestValidate_RefNotFound(t *testing.T) {
	lister := &stubLister{
		refs: []*plumbing.Reference{
			makeRef("refs/heads/main"),
		},
	}
	result := git.Validate(context.Background(), lister, "https://github.com/org/repo.git", "tok", "develop", "")

	if result.Success {
		t.Fatal("expected failure for missing revision")
	}
	if result.Error.Type != git.ErrRefNotFound {
		t.Errorf("expected ErrRefNotFound, got %d", result.Error.Type)
	}
}

func TestValidate_Success(t *testing.T) {
	lister := &stubLister{
		refs: []*plumbing.Reference{
			makeRef("refs/heads/main"),
			makeRef("refs/heads/develop"),
		},
	}
	result := git.Validate(context.Background(), lister, "https://github.com/org/repo.git", "tok", "main", "")

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.CheckedAt.IsZero() {
		t.Error("expected CheckedAt to be set")
	}
}

func TestValidate_HTTPURL(t *testing.T) {
	lister := &stubLister{}
	result := git.Validate(context.Background(), lister, "http://github.com/org/repo.git", "tok", "main", "")

	if result.Success {
		t.Fatal("expected failure for HTTP (non-HTTPS) URL")
	}
	if result.Error.Type != git.ErrInvalidURL {
		t.Errorf("expected ErrInvalidURL, got %d", result.Error.Type)
	}
}

type stubPathChecker struct {
	exists bool
	err    error
}

func (s *stubPathChecker) PathExists(_ context.Context, _, _, _, _ string) (bool, error) {
	return s.exists, s.err
}

type stubTreeLister struct {
	count int
	err   error
}

func (s *stubTreeLister) CountDirs(_ context.Context, _, _, _, _ string) (int, error) {
	return s.count, s.err
}

func TestValidate_DeployDirExists(t *testing.T) {
	lister := &stubLister{
		refs: []*plumbing.Reference{makeRef("refs/heads/main")},
	}
	checker := &stubPathChecker{exists: true}
	result := git.Validate(context.Background(), lister, "https://github.com/org/repo.git", "tok", "main", "deployments/host-a", checker)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
}

func TestValidate_DeployDirNotFound(t *testing.T) {
	lister := &stubLister{
		refs: []*plumbing.Reference{makeRef("refs/heads/main")},
	}
	checker := &stubPathChecker{exists: false}
	result := git.Validate(context.Background(), lister, "https://github.com/org/repo.git", "tok", "main", "nonexistent/dir", checker)

	if result.Success {
		t.Fatal("expected failure for missing deploy dir")
	}
	if result.Error.Type != git.ErrPathNotFound {
		t.Errorf("expected ErrPathNotFound, got %d", result.Error.Type)
	}
}

func TestValidate_DeployDirCheckError(t *testing.T) {
	lister := &stubLister{
		refs: []*plumbing.Reference{makeRef("refs/heads/main")},
	}
	checker := &stubPathChecker{err: fmt.Errorf("network error")}
	result := git.Validate(context.Background(), lister, "https://github.com/org/repo.git", "tok", "main", "deployments/host-a", checker)

	if result.Success {
		t.Fatal("expected failure for path check error")
	}
	if result.Error.Type != git.ErrPathNotFound {
		t.Errorf("expected ErrPathNotFound, got %d", result.Error.Type)
	}
}

func TestValidate_EmptyDeployDirSkipsCheck(t *testing.T) {
	lister := &stubLister{
		refs: []*plumbing.Reference{makeRef("refs/heads/main")},
	}
	checker := &stubPathChecker{exists: false}
	result := git.Validate(context.Background(), lister, "https://github.com/org/repo.git", "tok", "main", "", checker)

	if !result.Success {
		t.Fatalf("expected success with empty deploy dir, got error: %v", result.Error)
	}
}

func TestCountDirs_ReturnsCount(t *testing.T) {
	tl := &stubTreeLister{count: 5}
	count, err := tl.CountDirs(context.Background(), "https://github.com/org/repo.git", "tok", "main", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 dirs, got %d", count)
	}
}

func TestCountDirs_WithSubdir(t *testing.T) {
	tl := &stubTreeLister{count: 3}
	count, err := tl.CountDirs(context.Background(), "https://github.com/org/repo.git", "tok", "main", "deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 dirs, got %d", count)
	}
}

func TestCountDirs_Error(t *testing.T) {
	tl := &stubTreeLister{err: fmt.Errorf("clone failed")}
	_, err := tl.CountDirs(context.Background(), "https://github.com/org/repo.git", "tok", "main", "")
	if err == nil {
		t.Fatal("expected error")
	}
}
