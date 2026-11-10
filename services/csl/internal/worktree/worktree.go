// Package worktree gives a linked git worktree a lexical index of its own.
// The main index holds each repo's main checkout only, so a search from a
// worktree would answer from the default branch. A call that names a
// worktree gets that worktree's index instead: built on demand under
// search.WorktreeIndexRoot, rebuilt when the worktree's content changes,
// and removed once the worktree is gone. Fleet-wide searches never open it.
//
// A worktree's results carry the name finder gives every linked worktree,
// org/repo@branch (org/repo@<worktree id> on a detached HEAD), so a reader
// can tell the branch copy from the main checkout.
package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mad01/thismoon/kit/repofind"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// Root returns the root of the linked worktree that holds path, walking up
// to the nearest .git entry. ok is false when that entry belongs to a main
// clone or a submodule, or when no checkout holds path at all.
func Root(path string) (root string, ok bool) {
	dir := filepath.Clean(path)
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			if finder.IsLinkedWorktree(dir) {
				return dir, true
			}
			return "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Resolve turns a worktree argument into the worktree's repo. A path
// (absolute, ~-prefixed, or starting with ".") names the worktree or any
// directory inside it. Anything else is a name, matched like a repo name
// (case-insensitive regex, exactly one match) against the linked worktrees
// of the discovered repos. A worktree whose remote host index.hosts leaves
// out is refused, as its main checkout would be.
func Resolve(cfg *config.Config, arg string) (finder.Repo, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return finder.Repo{}, errors.New(
			"worktree is empty; pass a worktree path or its org/repo@branch name",
		)
	}
	if isPath(arg) {
		return fromPath(cfg, arg)
	}
	return byName(cfg, arg)
}

// isPath reports whether a worktree argument is a path rather than a name.
func isPath(arg string) bool {
	return filepath.IsAbs(repofind.ExpandHome(arg)) || strings.HasPrefix(arg, ".")
}

// fromPath resolves the linked worktree that holds a path.
func fromPath(cfg *config.Config, arg string) (finder.Repo, error) {
	abs, err := filepath.Abs(repofind.ExpandHome(arg))
	if err != nil {
		return finder.Repo{}, fmt.Errorf("worktree %q: %w", arg, err)
	}
	root, ok := Root(abs)
	if !ok {
		return finder.Repo{}, fmt.Errorf(
			"worktree %q: not inside a linked git worktree "+
				"(git worktree list shows them); for a main checkout pass repo instead",
			arg,
		)
	}
	return inspect(cfg, root)
}

// byName resolves a worktree name among the linked worktrees of the
// discovered repos.
func byName(cfg *config.Config, arg string) (finder.Repo, error) {
	repos, err := cfg.DiscoverRepos()
	if err != nil {
		return finder.Repo{}, fmt.Errorf("walk repos: %w", err)
	}
	wts := List(repos)
	if len(wts) == 0 {
		return finder.Repo{}, fmt.Errorf(
			"worktree %q: the discovered repos have no linked worktrees; pass the worktree's path",
			arg,
		)
	}
	wt, err := finder.MatchOne(wts, arg)
	if err != nil {
		return finder.Repo{}, fmt.Errorf("worktree: %w", err)
	}
	return wt, nil
}

// inspect reads a worktree's identity and refuses one whose main checkout
// the config drops from the index.
func inspect(cfg *config.Config, root string) (finder.Repo, error) {
	wt, err := finder.Inspect(root)
	if err != nil {
		return finder.Repo{}, fmt.Errorf("inspect worktree %s: %w", root, err)
	}
	if d, ok := dropped(cfg, wt); ok {
		return finder.Repo{}, fmt.Errorf(
			"worktree %s: %s, so csl does not index it",
			root,
			d.Reason(),
		)
	}
	return wt, nil
}

// dropped reports whether the config drops the worktree from the index, by
// the rules discovery applies to its main checkout: index.hosts, then
// hooks.post_merge.exclude on the main checkout's path and org/repo name or
// on the worktree's own. The drop words its reason the way `csl repo
// --skipped` and csl doctor do.
func dropped(cfg *config.Config, wt finder.Repo) (finder.Dropped, bool) {
	if cfg == nil {
		return finder.Dropped{}, false
	}
	if len(cfg.Index.Hosts) > 0 && !slices.Contains(cfg.Index.Hosts, wt.Host) {
		kind := finder.DropHost
		if wt.Remote == "" {
			kind = finder.DropNoRemote
		}
		return finder.Dropped{Repo: wt, Kind: kind}, true
	}
	mainPath, _ := finder.MainCheckout(wt.Path)
	excluded := cfg.Hooks.PostMerge.IsExcluded(mainPath, MainName(wt)) ||
		cfg.Hooks.PostMerge.IsExcluded(wt.Path, wt.Name)
	if excluded {
		return finder.Dropped{Repo: wt, Kind: finder.DropExcluded}, true
	}
	return finder.Dropped{}, false
}

// MainName is the org/repo name of the worktree's main checkout: its own
// name without the @branch suffix.
func MainName(wt finder.Repo) string {
	name, _, _ := strings.Cut(wt.Name, "@")
	return name
}

// SelectsOnly reports whether filter, a repo filter read as --repo reads it
// (case-insensitive regex), selects the worktree's repo and nothing else: it
// matches the worktree's org/repo@branch name or its main checkout's
// org/repo, and no discovered repo but that main checkout and the worktree
// itself. discover runs only when the filter matches the worktree at all,
// since a walk of the configured dirs is the expensive part.
func SelectsOnly(
	filter string,
	wt finder.Repo,
	discover func() ([]finder.Repo, error),
) (bool, error) {
	re, err := finder.CompileMatcher(filter)
	if err != nil {
		return false, err
	}
	main := MainName(wt)
	if !re.MatchString(wt.Name) && !re.MatchString(main) {
		return false, nil
	}
	repos, err := discover()
	if err != nil {
		return false, fmt.Errorf("walk repos: %w", err)
	}
	for _, r := range repos {
		if r.Name == main || r.Path == wt.Path {
			continue
		}
		if re.MatchString(r.Name) {
			return false, nil
		}
	}
	return true, nil
}

// List returns the linked worktrees of the given repos, in repo order. A
// repo that is itself a linked worktree contributes none.
func List(repos []finder.Repo) []finder.Repo {
	var wts []finder.Repo
	for _, r := range repos {
		for _, root := range finder.LinkedWorktrees(r.Path) {
			wt, err := finder.Inspect(root)
			if err != nil {
				continue
			}
			wts = append(wts, wt)
		}
	}
	return wts
}

// FromWorkingDir returns the linked worktree that holds dir, for the CLI's
// --repo rule. ok is false when dir is not inside one, or when the config
// drops the worktree's repo from the index: the caller did not name the
// worktree, so it gets the main index instead of an error.
func FromWorkingDir(cfg *config.Config, dir string) (finder.Repo, bool) {
	root, ok := Root(dir)
	if !ok {
		return finder.Repo{}, false
	}
	wt, err := finder.Inspect(root)
	if err != nil {
		return finder.Repo{}, false
	}
	if _, drop := dropped(cfg, wt); drop {
		return finder.Repo{}, false
	}
	return wt, true
}
