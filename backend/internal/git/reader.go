package git

import (
	"context"
	"fmt"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

// ComposeEntry represents a compose file found in the repository.
type ComposeEntry struct {
	// StackPath is the directory containing the compose file (relative to deploy dir).
	StackPath string
	// ComposeFile is a supported Compose filename.
	ComposeFile string
	// Content is the raw compose file content.
	Content []byte
}

// ComposeReader reads compose files from a Git repository.
type ComposeReader interface {
	// ReadComposeFiles clones the repo (shallow, in-memory) and returns all compose entries
	// found under the given deploy directory.
	// Returns entries, commit hash, commit message, and error.
	ReadComposeFiles(ctx context.Context, repoURL, token, revision, deployDir string) ([]ComposeEntry, string, string, error)
}

// GoGitComposeReader implements ComposeReader using go-git.
type GoGitComposeReader struct{}

// ReadComposeFiles performs a shallow clone and recursively scans stack directories.
// Returns the list of compose entries, resolved commit hash, commit message, and error.
func (g *GoGitComposeReader) ReadComposeFiles(ctx context.Context, repoURL, token, revision, deployDir string) ([]ComposeEntry, string, string, error) {
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
		return nil, "", "", fmt.Errorf("shallow clone failed: %w", err)
	}

	ref, err := repo.Head()
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	commitHash := ref.Hash().String()

	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get commit: %w", err)
	}

	commitMessage := strings.TrimSpace(commit.Message)

	rootTree, err := commit.Tree()
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get tree: %w", err)
	}

	// Navigate to deploy directory if specified
	tree := rootTree
	if deployDir = strings.Trim(deployDir, "/"); deployDir != "" {
		tree, err = rootTree.Tree(deployDir)
		if err != nil {
			return nil, "", "", fmt.Errorf("deploy dir %q not found: %w", deployDir, err)
		}
	}

	entries, err := scanComposeEntries(tree, "")
	return entries, commitHash, commitMessage, err
}

// findComposeFile preserves legacy filename precedence, then accepts Compose's
// newer compose.yaml/compose.yml names. One directory produces one stack.
func findComposeFile(tree *object.Tree) (string, []byte, error) {
	candidates := []string{"docker-compose.yml", "docker-compose.yaml", "compose.yaml", "compose.yml"}

	for _, name := range candidates {
		file, err := tree.File(name)
		if err != nil {
			continue
		}
		content, err := file.Contents()
		if err != nil {
			continue
		}
		return name, []byte(content), nil
	}

	return "", nil, nil
}
