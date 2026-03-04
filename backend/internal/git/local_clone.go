package git

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

const DefaultLocalRepoPath = "/repo"

// LocalClone manages a persistent on-disk Git clone.
type LocalClone struct {
	mu       sync.Mutex
	Path     string
	RepoURL  string
	Token    string
	Revision string
}

// NewLocalClone creates a LocalClone manager for a fixed path.
func NewLocalClone(path, repoURL, token, revision string) *LocalClone {
	return &LocalClone{
		Path:     path,
		RepoURL:  repoURL,
		Token:    token,
		Revision: revision,
	}
}

// LocalComposeReader implements ComposeReader using a LocalClone.
type LocalComposeReader struct {
	Clone *LocalClone
}

// NewLocalComposeReader returns a ComposeReader backed by a LocalClone.
func NewLocalComposeReader(clone *LocalClone) *LocalComposeReader {
	return &LocalComposeReader{Clone: clone}
}

// ReadComposeFiles refreshes the local clone and reads compose files from it.
func (r *LocalComposeReader) ReadComposeFiles(ctx context.Context, _, _, _, deployDir string) ([]ComposeEntry, string, string, error) {
	return r.Clone.ReadComposeFiles(ctx, deployDir)
}

// ReadComposeFiles refreshes the local clone and returns compose entries.
func (c *LocalClone) ReadComposeFiles(ctx context.Context, deployDir string) ([]ComposeEntry, string, string, error) {
	repo, hash, commitMessage, err := c.refreshRepo(ctx)
	if err != nil {
		return nil, "", "", err
	}

	entries, err := readComposeEntries(repo, *hash, deployDir)
	if err != nil {
		return nil, "", "", err
	}

	return entries, hash.String(), commitMessage, nil
}

func (c *LocalClone) refreshRepo(ctx context.Context) (*gogit.Repository, *plumbing.Hash, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	repo, err := gogit.PlainOpen(c.Path)
	if err != nil {
		if err == gogit.ErrRepositoryNotExists {
			return c.cloneRepo(ctx)
		}
		return c.reclone(ctx, fmt.Errorf("open repo failed: %w", err))
	}

	if err := c.fetchRepo(ctx, repo); err != nil {
		return c.reclone(ctx, err)
	}

	hash, err := c.resolveRevision(repo)
	if err != nil {
		return c.reclone(ctx, err)
	}

	if err := checkoutHash(repo, *hash); err != nil {
		return c.reclone(ctx, err)
	}

	message, err := commitMessage(repo, *hash)
	if err != nil {
		return c.reclone(ctx, err)
	}

	return repo, hash, message, nil
}

func (c *LocalClone) cloneRepo(ctx context.Context) (*gogit.Repository, *plumbing.Hash, string, error) {
	_ = os.RemoveAll(c.Path)

	opts := &gogit.CloneOptions{
		URL:           c.RepoURL,
		ReferenceName: plumbing.NewBranchReferenceName(c.Revision),
		SingleBranch:  true,
	}
	if c.Token != "" {
		opts.Auth = &http.BasicAuth{Username: "x-access-token", Password: c.Token}
	}

	repo, err := gogit.PlainCloneContext(ctx, c.Path, false, opts)
	if err != nil {
		return nil, nil, "", fmt.Errorf("clone failed: %w", err)
	}

	hash, err := c.resolveRevision(repo)
	if err != nil {
		return nil, nil, "", err
	}

	message, err := commitMessage(repo, *hash)
	if err != nil {
		return nil, nil, "", err
	}

	return repo, hash, message, nil
}

func (c *LocalClone) reclone(ctx context.Context, err error) (*gogit.Repository, *plumbing.Hash, string, error) {
	_ = os.RemoveAll(c.Path)
	return c.cloneRepo(ctx)
}

func (c *LocalClone) fetchRepo(ctx context.Context, repo *gogit.Repository) error {
	opts := &gogit.FetchOptions{
		RemoteName: "origin",
		Force:      true,
		Prune:      true,
		Tags:       gogit.AllTags,
	}
	if c.Token != "" {
		opts.Auth = &http.BasicAuth{Username: "x-access-token", Password: c.Token}
	}

	if err := repo.FetchContext(ctx, opts); err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch failed: %w", err)
	}
	return nil
}

func (c *LocalClone) resolveRevision(repo *gogit.Repository) (*plumbing.Hash, error) {
	if c.Revision == "" {
		ref, err := repo.Head()
		if err != nil {
			return nil, fmt.Errorf("head lookup failed: %w", err)
		}
		hash := ref.Hash()
		return &hash, nil
	}

	rev := plumbing.Revision("refs/remotes/origin/" + c.Revision)
	hash, err := repo.ResolveRevision(rev)
	if err == nil {
		return hash, nil
	}

	rev = plumbing.Revision(c.Revision)
	hash, err = repo.ResolveRevision(rev)
	if err != nil {
		return nil, fmt.Errorf("resolve revision failed: %w", err)
	}
	return hash, nil
}

func checkoutHash(repo *gogit.Repository, hash plumbing.Hash) error {
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree access failed: %w", err)
	}

	if err := wt.Checkout(&gogit.CheckoutOptions{Hash: hash, Force: true}); err != nil {
		return fmt.Errorf("checkout failed: %w", err)
	}
	return nil
}

func commitMessage(repo *gogit.Repository, hash plumbing.Hash) (string, error) {
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return "", fmt.Errorf("commit lookup failed: %w", err)
	}
	return strings.TrimSpace(commit.Message), nil
}

func readComposeEntries(repo *gogit.Repository, hash plumbing.Hash, deployDir string) ([]ComposeEntry, error) {
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	rootTree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get tree: %w", err)
	}

	tree := rootTree
	if deployDir != "" {
		deployDir = strings.Trim(deployDir, "/")
		tree, err = rootTree.Tree(deployDir)
		if err != nil {
			return nil, fmt.Errorf("deploy dir %q not found: %w", deployDir, err)
		}
	}

	var entries []ComposeEntry
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

	return entries, nil
}
