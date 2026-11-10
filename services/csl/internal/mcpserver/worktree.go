package mcpserver

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mad01/thismoon/services/csl/internal/lexical"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/worktree"
)

// worktreeParam is the parameter that points csl_search, csl_search_multi,
// csl_count, csl_read, csl_outline and csl_ask at one linked git worktree
// instead of the main checkouts. Without it a call never touches a worktree
// index, so fleet-wide results stay one entry per repo. Its description
// loads on six tools in every session, so it stays one short line; `csl
// docs` carries the rest.
type worktreeParam struct {
	Worktree string `json:"worktree,omitempty" jsonschema:"linked git worktree path or org/repo@branch to answer from instead of repo"`
}

// newSearchEnvFor is newSearchEnvIn, or the setup over one worktree's own
// index when wt names a worktree.
func newSearchEnvFor(cfg *config.Config, wt string) (searchEnv, error) {
	if strings.TrimSpace(wt) == "" {
		return newSearchEnvIn(cfg)
	}
	idx, err := ensureWorktreeIndex(cfg, wt)
	if err != nil {
		return searchEnv{}, err
	}
	return worktreeSearchEnv(idx), nil
}

// ensureWorktreeIndex resolves a worktree argument and brings its index up
// to date. The caller must Close the returned Index.
func ensureWorktreeIndex(cfg *config.Config, wt string) (worktree.Index, error) {
	repo, err := worktree.Resolve(cfg, wt)
	if err != nil {
		return worktree.Index{}, err
	}
	return worktree.Ensure(repo, cfg.AllowedHiddenDirs())
}

// worktreeSearchEnv is the search setup over one worktree index. It searches
// in-process, since the daemon serves the main index only, and the worktree
// is its only repo, so the zero-result hint counts it alone. It holds the
// index's lease until close.
func worktreeSearchEnv(idx worktree.Index) searchEnv {
	paths := map[string]string{idx.Repo.Name: idx.Repo.Path}
	return searchEnv{
		backend:   lexical.Local{IndexDir: idx.Dir, RepoNames: paths},
		repos:     []finder.Repo{idx.Repo},
		repoPaths: paths,
		indexDir:  idx.Dir,
		surface:   mcpSurface,
		worktree:  true,
		note:      idx.StaleNote(),
		release:   idx.Close,
	}
}

// resolveTargetIn returns the checkout a by-repo tool works in: the linked
// worktree wt names when it is set, else the one discovered repo that repo
// matches.
func resolveTargetIn(cfg *config.Config, repo, wt string) (finder.Repo, error) {
	if strings.TrimSpace(wt) != "" {
		return worktree.Resolve(cfg, wt)
	}
	if strings.TrimSpace(repo) == "" {
		return finder.Repo{}, errors.New("repo is required (or worktree, for a linked worktree)")
	}
	return resolveRepoIn(cfg, repo)
}

// resolveTarget is resolveTargetIn with the config loaded here.
func resolveTarget(repo, wt string) (finder.Repo, error) {
	cfg, err := config.Load()
	if err != nil {
		return finder.Repo{}, fmt.Errorf("load csl config: %w", err)
	}
	return resolveTargetIn(cfg, repo, wt)
}
