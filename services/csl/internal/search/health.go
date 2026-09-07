package search

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/semaphore"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// Actions a git-health sweep can recommend for a repo, most urgent first.
const (
	GitActionCommitOrStash = "commit_or_stash"
	GitActionDiverged      = "diverged"
	GitActionPush          = "push_recommended"
	GitActionPull          = "pull_recommended"
	GitActionNoUpstream    = "no_upstream"
	GitActionDetached      = "detached_head"
	GitActionError         = "error"
	GitActionReady         = "ready"
)

// GitHealth is one repo's git working-state summary in a fleet sweep: what
// uncommitted or unpushed work it holds and what to do about it. Ahead/Behind
// compare against the last-fetched upstream ref — the sweep never fetches.
type GitHealth struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Host           string `json:"host,omitempty"`
	Branch         string `json:"branch,omitempty"`
	Dirty          bool   `json:"dirty"`
	ModifiedFiles  int    `json:"modified_files"`
	UntrackedFiles int    `json:"untracked_files"`
	Ahead          int    `json:"ahead"`
	Behind         int    `json:"behind"`
	HasUpstream    bool   `json:"has_upstream"`
	Action         string `json:"action"`
	Error          string `json:"error,omitempty"`
}

// NeedsAttention reports whether the repo holds work worth looking at before,
// say, a machine switch: anything but a clean, synced checkout.
func (h GitHealth) NeedsAttention() bool { return h.Action != GitActionReady }

// deriveGitAction maps a repo's git state to the recommended action. Pure, so
// the priority ladder is unit testable.
func deriveGitAction(h GitHealth) string {
	switch {
	case h.Error != "":
		return GitActionError
	case h.Branch == "HEAD":
		return GitActionDetached
	case h.Dirty:
		return GitActionCommitOrStash
	case !h.HasUpstream:
		return GitActionNoUpstream
	case h.Ahead > 0 && h.Behind > 0:
		return GitActionDiverged
	case h.Ahead > 0:
		return GitActionPush
	case h.Behind > 0:
		return GitActionPull
	default:
		return GitActionReady
	}
}

// AheadBehind reports how many commits HEAD is ahead of and behind its
// configured upstream. It reads the local upstream ref — no fetch happens, so
// behind is relative to the last fetch. hasUpstream is false (with no error)
// whenever git cannot make the comparison: no upstream configured, detached
// HEAD, or not a git repo. Git exits non-zero for all of those, and telling
// the stderr texts apart across git versions buys nothing here.
func AheadBehind(repoPath string) (ahead, behind int, hasUpstream bool, err error) {
	out, err := gitOutput(repoPath, "rev-list", "--count", "--left-right", "@{u}...HEAD")
	if err != nil {
		if _, ok := errors.AsType[*exec.ExitError](err); ok {
			return 0, 0, false, nil
		}
		return 0, 0, false, fmt.Errorf("git rev-list in %s: %w", repoPath, err)
	}

	left, right, ok := strings.Cut(out, "\t")
	if !ok {
		return 0, 0, false, fmt.Errorf("git rev-list in %s: unexpected output %q", repoPath, out)
	}
	behind, err = strconv.Atoi(strings.TrimSpace(left))
	if err != nil {
		return 0, 0, false, fmt.Errorf("git rev-list in %s: unexpected output %q", repoPath, out)
	}
	ahead, err = strconv.Atoi(strings.TrimSpace(right))
	if err != nil {
		return 0, 0, false, fmt.Errorf("git rev-list in %s: unexpected output %q", repoPath, out)
	}
	return ahead, behind, true, nil
}

// gitHealthConcurrency bounds parallel git subprocesses during a sweep. Each
// repo costs up to three (the fingerprint is TTL-cached, dirty counts and
// ahead/behind are not). Raised for fleets with many checkouts, where a lower
// bound makes a cold sweep drag.
const gitHealthConcurrency = 16

// GitHealthSweep gathers GitHealth for every repo concurrently, preserving
// input order. Per-repo git failures land in that entry's Error field instead
// of failing the sweep, so one broken checkout cannot hide the rest.
func GitHealthSweep(ctx context.Context, repos []finder.Repo) []GitHealth {
	sem := semaphore.NewWeighted(gitHealthConcurrency)
	out := make([]GitHealth, len(repos))

	var wg sync.WaitGroup
	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r finder.Repo) {
			defer wg.Done()
			if err := sem.Acquire(ctx, 1); err != nil {
				out[idx] = GitHealth{
					Name: r.Name, Path: r.Path, Host: r.Host,
					Error: err.Error(), Action: GitActionError,
				}
				return
			}
			defer sem.Release(1)
			out[idx] = gitHealthOne(r)
		}(i, repo)
	}
	wg.Wait()

	return out
}

// gitHealthOne gathers one repo's git state and derives its action.
func gitHealthOne(r finder.Repo) GitHealth {
	h := GitHealth{Name: r.Name, Path: r.Path, Host: r.Host}

	fp, err := cachedFingerprint(r.Path)
	if err != nil {
		h.Error = err.Error()
		h.Action = deriveGitAction(h)
		return h
	}
	h.Branch = fp.Branch
	h.Dirty = fp.Dirty

	if h.Dirty {
		if mod, untracked, err := DirtyInfo(r.Path); err == nil {
			h.ModifiedFiles = mod
			h.UntrackedFiles = untracked
		}
	}

	ahead, behind, hasUpstream, err := AheadBehind(r.Path)
	if err != nil {
		h.Error = err.Error()
	}
	h.Ahead, h.Behind, h.HasUpstream = ahead, behind, hasUpstream

	h.Action = deriveGitAction(h)
	return h
}
