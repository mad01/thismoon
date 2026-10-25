// Package gitloc places a directory in git: the working tree it belongs to,
// the main checkout that tree hangs off, the branch and commit checked out
// there, and the repo's canonical identity. A loom session works in a linked
// worktree under ~/.worktrees/<repo>/<slug> while the main checkout stays on
// the default branch, so the working tree and the main checkout are two
// different answers for one repo.
//
// Every function here degrades to a zero value and never errors or guesses.
// The callers are hooks and recorders that must not judge the session's repo
// in place of the one a command targets, and must not stall a session on a
// slow filesystem: every git call is bounded by the caller's context.
package gitloc

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// pipeGrace bounds how long a git call waits for its stdout pipe to close
// after git itself has exited or been killed.
const pipeGrace = 100 * time.Millisecond

// Location is where a directory sits in git.
type Location struct {
	// Toplevel is the root of the working tree the directory belongs to.
	Toplevel string
	// Canonical is the root of the main checkout. It equals Toplevel
	// outside a linked worktree; for a linked one it is derived from the
	// shared git dir (see mainCheckout, including its known limits).
	Canonical string
	// Branch is the checked-out branch, "HEAD" when detached.
	Branch string
	// Head is the full object name of the checked-out commit.
	Head string
	// Repo is the canonical host/owner/repo identity from the origin
	// remote, "" without one. Every worktree of a repo shares it.
	Repo string
	// Linked reports a linked worktree (git worktree add), as opposed to
	// the main checkout.
	Linked bool
}

// locationArgs asks git for every Location field but Repo in one call. The
// paths come back absolute and physical: without --path-format, git prints
// the common dir relative to the directory it ran in. --abbrev-ref applies
// to the revisions after it, so the first HEAD prints the commit and the
// second the branch.
var locationArgs = []string{
	"rev-parse", "--path-format=absolute",
	"--show-toplevel", "--git-dir", "--git-common-dir",
	"HEAD", "--abbrev-ref", "HEAD",
}

// locationLines is how many lines locationArgs prints: toplevel, git dir,
// common dir, commit, branch.
const locationLines = 5

// LocationAt resolves where dir sits in git, with two git processes: the
// rev-parse call for the tree, the main checkout, the commit and the branch,
// and the origin lookup for Repo. Both are killed when ctx is done. A git
// field degrades to its zero value when git cannot say: dir is "" or outside
// a repo, the repo has no commit yet (HEAD does not resolve, so the whole
// rev-parse call fails), the git is too old to know --path-format, or ctx
// ran out. Repo comes from its own call, so an origin can still resolve when
// the rest is empty.
func LocationAt(ctx context.Context, dir string) Location {
	loc := parseLocation(Output(ctx, dir, locationArgs...))
	loc.Repo = CanonicalRepo(Output(ctx, dir, "remote", "get-url", "origin"))
	return loc
}

// parseLocation reads the locationArgs output. A linked worktree is the one
// layout where the per-tree git dir differs from the shared common dir.
// Comparing the two, rather than the common dir's parent against the
// toplevel, keeps a submodule or a --separate-git-dir checkout (whose git
// dir sits outside the tree) from reading as linked. Output of the wrong
// shape, including the flag echoed back by a git that predates
// --path-format, yields the zero Location.
func parseLocation(out string) Location {
	lines := strings.Split(out, "\n")
	if len(lines) != locationLines {
		return Location{}
	}
	top, gitDir, common, head, branch := lines[0], lines[1], lines[2], lines[3], lines[4]
	if !filepath.IsAbs(top) || !filepath.IsAbs(gitDir) || !filepath.IsAbs(common) {
		return Location{}
	}
	loc := Location{Toplevel: top, Canonical: top, Branch: branch, Head: head}
	if filepath.Clean(gitDir) != filepath.Clean(common) {
		loc.Linked = true
		loc.Canonical = mainCheckout(common)
	}
	return loc
}

// mainCheckout names the main checkout a linked worktree hangs off: the
// parent of the shared .git directory. A common dir with any other name is
// taken for a bare repo, which has no checkout to name.
//
// The name is the whole test, so two layouts read wrong, both rare and both
// outside the loom layout: a bare repo cloned into proj/.git reads as a
// checkout at proj, and a worktree of a --separate-git-dir checkout (whose
// git dir has its own name) reads as having no main checkout. Telling them
// apart would take another git call on every record.
func mainCheckout(common string) string {
	common = filepath.Clean(common)
	if filepath.Base(common) != ".git" {
		return ""
	}
	return filepath.Dir(common)
}

// CanonicalRepo normalizes a git remote URL to "host/owner/repo"
// (github.com/mad01/dotfiles for both the SSH and HTTPS forms). It returns
// "" for anything it cannot resolve, an empty remote or a bare local path,
// so a caller matching an allowlist fails closed.
func CanonicalRepo(remote string) string {
	remote = strings.TrimSpace(remote)
	remote = strings.TrimSuffix(remote, ".git")
	switch {
	case strings.Contains(remote, "://"):
		// scheme://[user@]host[:port]/owner/repo
		remote = remote[strings.Index(remote, "://")+3:]
		if i := strings.LastIndex(remote, "@"); i >= 0 {
			remote = remote[i+1:]
		}
	case strings.Contains(remote, "@") && strings.Contains(remote, ":"):
		// scp-like: [user@]host:owner/repo
		remote = remote[strings.Index(remote, "@")+1:]
		remote = strings.Replace(remote, ":", "/", 1)
	default:
		return ""
	}
	// Drop a :port from the host segment.
	if i := strings.Index(remote, "/"); i >= 0 {
		if j := strings.Index(remote[:i], ":"); j >= 0 {
			remote = remote[:j] + remote[i:]
		}
	}
	// Need at least host/owner/repo.
	if strings.Count(remote, "/") < 2 {
		return ""
	}
	return remote
}

// Output runs one git command in dir and returns its trimmed stdout, or ""
// on any failure, including ctx ending first: git is killed when ctx is done.
// An empty dir means the caller could not tell where the command runs, and
// running git in the process's own directory instead would answer for the
// wrong repo, so it returns "" without running anything. A child git leaves
// behind (a wrapper's background job, say) inherits the stdout pipe, so
// killing git alone does not end the read. WaitDelay closes the pipe
// pipeGrace after git exits or is killed, which keeps ctx's deadline the
// real bound.
func Output(ctx context.Context, dir string, args ...string) string {
	if dir == "" {
		return ""
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.WaitDelay = pipeGrace
	out, err := cmd.Output()
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		// ErrWaitDelay means git exited 0 and only the child held the pipe
		// past the grace: what git printed before exiting is its answer.
		return ""
	}
	return strings.TrimSpace(string(out))
}
