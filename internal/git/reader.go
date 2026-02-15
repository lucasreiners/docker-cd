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
	// ComposeFile is the filename (docker-compose.yml or docker-compose.yaml).
	ComposeFile string
	// Content is the raw compose file content.
	Content []byte
}

// ComposeReader reads compose files from a Git repository.
type ComposeReader interface {
	// ReadComposeFiles clones the repo (shallow, in-memory) and returns all compose entries
	// found under the given deploy directory.
	ReadComposeFiles(ctx context.Context, repoURL, token, revision, deployDir string) ([]ComposeEntry, string, error)
}

// GoGitComposeReader implements ComposeReader using go-git.
type GoGitComposeReader struct{}

// ReadComposeFiles performs a shallow clone and scans for docker-compose.yml/yaml files
// in immediate subdirectories of the deploy directory.
// Returns the list of compose entries and the resolved commit hash.
func (g *GoGitComposeReader) ReadComposeFiles(ctx context.Context, repoURL, token, revision, deployDir string) ([]ComposeEntry, string, error) {
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
		return nil, "", fmt.Errorf("shallow clone failed: %w", err)
	}

	ref, err := repo.Head()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	commitHash := ref.Hash().String()

	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, "", fmt.Errorf("failed to get commit: %w", err)
	}

	rootTree, err := commit.Tree()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get tree: %w", err)
	}

	// Navigate to deploy directory if specified
	tree := rootTree
	if deployDir != "" {
		deployDir = strings.Trim(deployDir, "/")
		tree, err = rootTree.Tree(deployDir)
		if err != nil {
			return nil, "", fmt.Errorf("deploy dir %q not found: %w", deployDir, err)
		}
	}

	var entries []ComposeEntry

	// Iterate immediate subdirectories
	for _, entry := range tree.Entries {
		if entry.Mode.IsFile() {
			continue
		}

		subtree, err := tree.Tree(entry.Name)
		if err != nil {
			continue
		}

		composeFile, content, err := findComposeFile(subtree)
		if err != nil || composeFile == "" {
			continue
		}

		entries = append(entries, ComposeEntry{
			StackPath:   entry.Name,
			ComposeFile: composeFile,
			Content:     content,
		})
	}

	return entries, commitHash, nil
}

// findComposeFile looks for docker-compose.yml or docker-compose.yaml in a tree.
// Prefers docker-compose.yml when both exist.
func findComposeFile(tree *object.Tree) (string, []byte, error) {
	candidates := []string{"docker-compose.yml", "docker-compose.yaml"}

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
