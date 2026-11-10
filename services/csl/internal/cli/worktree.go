package cli

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/worktree"
)

// workingDir is os.Getwd. Tests point it away from the checkout they run
// in, which may itself be a linked worktree.
var workingDir = os.Getwd

// worktreeFlags is the --worktree / --no-worktree pair csl search, count,
// read, outline and ask carry: one value per command, since each binds its
// own flag set.
type worktreeFlags struct {
	arg string
	off bool
}

// register adds the pair to cmd.
func (f *worktreeFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.arg, "worktree", "",
		"answer from this linked git worktree (a path in it, . for the one you are in, or its org/repo@branch name) instead of the main index")
	cmd.Flags().BoolVar(&f.off, "no-worktree", false,
		"use the main index even when --repo selects only the repo of the worktree you are in")
	cmd.MarkFlagsMutuallyExclusive("worktree", "no-worktree")
}

// worktreeChoice is the linked worktree a command answers from.
type worktreeChoice struct {
	repo finder.Repo
	// byRepo is set when the repo filter picked the worktree, not
	// --worktree, so the command says on stderr which checkout answers.
	byRepo bool
	// filter is the repo filter that picked it, quoted in that note.
	filter string
}

// choose returns the worktree the command answers from, or nil for the main
// index. The working directory never narrows a query on its own, so a
// fleet-wide search run from inside a worktree still covers the fleet.
// --worktree names a worktree, a relative path resolving against the
// working directory, so "." is the worktree you are in. Otherwise the
// worktree holding the working directory answers only when repo, the
// command's repo filter, selects its repo and no other discovered repo
// (worktree.SelectsOnly), unless --no-worktree is set.
func (f worktreeFlags) choose(cfg *config.Config, repo string) (*worktreeChoice, error) {
	if f.arg != "" {
		wt, err := worktree.Resolve(cfg, f.resolvedArg())
		if err != nil {
			return nil, fmt.Errorf("--worktree: %w", err)
		}
		return &worktreeChoice{repo: wt}, nil
	}
	if f.off || strings.TrimSpace(repo) == "" {
		return nil, nil
	}
	dir, err := workingDir()
	if err != nil {
		return nil, nil // a working directory that is gone sits in no worktree
	}
	wt, ok := worktree.FromWorkingDir(cfg, dir)
	if !ok {
		return nil, nil
	}
	only, err := worktree.SelectsOnly(repo, wt, cfg.DiscoverRepos)
	if err != nil {
		return nil, fmt.Errorf("--repo: %w", err)
	}
	if !only {
		return nil, nil
	}
	return &worktreeChoice{repo: wt, byRepo: true, filter: repo}, nil
}

// resolvedArg is the --worktree value with a relative path ("." or "./x")
// joined to the working directory, which tests can point elsewhere.
func (f worktreeFlags) resolvedArg() string {
	arg := strings.TrimSpace(f.arg)
	if !strings.HasPrefix(arg, ".") {
		return arg
	}
	dir, err := workingDir()
	if err != nil {
		return arg
	}
	return filepath.Join(dir, arg)
}

// announce prepares the process for answering from the worktree. It mutes
// the standard logger, since a worktree is searched in-process and zoekt
// logs every shard it builds or loads, which would otherwise land on stderr
// on every call. When the repo filter picked the worktree it also tells the
// reader on w which checkout answers and how to get the main one; a
// worktree --worktree named needs no note.
func (c *worktreeChoice) announce(w io.Writer) {
	log.SetOutput(io.Discard)
	if !c.byRepo {
		return
	}
	fmt.Fprintf(
		w,
		"worktree: %q matches only %s, so its linked worktree %s at %s answers; --no-worktree answers from the main checkout\n",
		c.filter,
		worktree.MainName(c.repo),
		c.repo.Name,
		c.repo.Path,
	)
}

// noteStale prints the index's stale note on w, when it has one.
func noteStale(w io.Writer, idx worktree.Index) {
	if note := idx.StaleNote(); note != "" {
		fmt.Fprintf(w, "worktree: %s\n", note)
	}
}
