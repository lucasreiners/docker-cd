package git

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

// ValidationResult holds the outcome of a read-only repository validation check.
type ValidationResult struct {
	Success   bool
	Error     *ValidationError
	CheckedAt time.Time
}

// RemoteLister abstracts a read-only git remote listing call for testability.
// Implementations only need read access to the repository (git ls-remote).
type RemoteLister interface {
	ListRefs(ctx context.Context, repoURL, token string) ([]*plumbing.Reference, error)
}

// PathChecker verifies that a path exists at a given revision (read-only).
type PathChecker interface {
	PathExists(ctx context.Context, repoURL, token, revision, path string) (bool, error)
}

// TreeLister counts directories at a given path in the repository tree (read-only).
type TreeLister interface {
	CountDirs(ctx context.Context, repoURL, token, revision, path string) (int, error)
}

// GoGitRemoteLister implements RemoteLister using go-git.
// It only requires read access — it calls git ls-remote under the hood.
type GoGitRemoteLister struct{}

func (g *GoGitRemoteLister) ListRefs(ctx context.Context, repoURL, token string) ([]*plumbing.Reference, error) {
	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoURL},
	})
	refs, err := remote.ListContext(ctx, &gogit.ListOptions{
		Auth: &http.BasicAuth{
			Username: "x-access-token",
			Password: token,
		},
	})
	return refs, err
}

// GoGitTreeLister implements TreeLister using go-git.
// It performs a shallow clone (depth 1, single branch) to memory and inspects
// the tree to count immediate subdirectories at the given path.
type GoGitTreeLister struct{}

func (g *GoGitTreeLister) CountDirs(ctx context.Context, repoURL, token, revision, path string) (int, error) {
	repo, err := gogit.CloneContext(ctx, memory.NewStorage(), nil, &gogit.CloneOptions{
		URL: repoURL,
		Auth: &http.BasicAuth{
			Username: "x-access-token",
			Password: token,
		},
		ReferenceName: plumbing.NewBranchReferenceName(revision),
		SingleBranch:  true,
		Depth:         1,
	})
	if err != nil {
		return 0, fmt.Errorf("shallow clone failed: %w", err)
	}

	ref, err := repo.Head()
	if err != nil {
		return 0, fmt.Errorf("failed to get HEAD: %w", err)
	}

	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return 0, fmt.Errorf("failed to get commit: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return 0, fmt.Errorf("failed to get tree: %w", err)
	}

	// Navigate to subdirectory if path is set
	if path != "" {
		path = strings.Trim(path, "/")
	}
	if path != "" {
		tree, err = tree.Tree(path)
		if err != nil {
			return 0, fmt.Errorf("path %q not found in tree: %w", path, err)
		}
	}

	count := 0
	for _, entry := range tree.Entries {
		if entry.Mode.IsFile() {
			continue
		}
		count++
	}

	return count, nil
}

// Validate performs a read-only check that the repository is reachable with the
// given token, the revision exists, and the optional deploy directory is present.
// Only read access is required; no write/push permissions are needed.
// pathChecker may be nil if deployDir is empty.
func Validate(ctx context.Context, lister RemoteLister, repoURL, token, revision, deployDir string, pathChecker ...PathChecker) ValidationResult {
	now := time.Now()

	// Validate HTTPS URL
	u, err := url.Parse(repoURL)
	if err != nil || !strings.EqualFold(u.Scheme, "https") {
		return ValidationResult{
			Success:   false,
			Error:     &ValidationError{Type: ErrInvalidURL, Message: fmt.Sprintf("repository URL must be HTTPS, got %q", repoURL)},
			CheckedAt: now,
		}
	}

	// List remote refs to verify read access + connectivity (git ls-remote)
	refs, err := lister.ListRefs(ctx, repoURL, token)
	if err != nil {
		return ValidationResult{
			Success:   false,
			Error:     &ValidationError{Type: ErrAuthFailed, Message: "failed to access repository", Cause: err},
			CheckedAt: now,
		}
	}

	// Check that the requested revision exists as a branch or tag
	found := false
	for _, ref := range refs {
		name := ref.Name()
		short := name.Short()
		if short == revision || string(name) == revision {
			found = true
			break
		}
	}
	if !found {
		return ValidationResult{
			Success:   false,
			Error:     &ValidationError{Type: ErrRefNotFound, Message: fmt.Sprintf("revision %q not found in remote refs", revision)},
			CheckedAt: now,
		}
	}

	// Validate deploy directory if specified
	if deployDir != "" && len(pathChecker) > 0 && pathChecker[0] != nil {
		exists, err := pathChecker[0].PathExists(ctx, repoURL, token, revision, deployDir)
		if err != nil {
			return ValidationResult{
				Success:   false,
				Error:     &ValidationError{Type: ErrPathNotFound, Message: fmt.Sprintf("failed to check deploy dir %q", deployDir), Cause: err},
				CheckedAt: now,
			}
		}
		if !exists {
			return ValidationResult{
				Success:   false,
				Error:     &ValidationError{Type: ErrPathNotFound, Message: fmt.Sprintf("deploy directory %q not found at revision %q", deployDir, revision)},
				CheckedAt: now,
			}
		}
	}

	return ValidationResult{
		Success:   true,
		CheckedAt: now,
	}
}
