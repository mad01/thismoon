package guard

import (
	"context"
	"path/filepath"
	"strings"
)

// Location is where a directory sits in git: the working tree it belongs
// to, the main checkout that tree hangs off, the branch checked out there,
// and the repo's canonical identity. A loom session works in a linked
// worktree under ~/.worktrees/<repo>/<slug> while the main checkout stays on
// the default branch, so the working tree and the main checkout are two
// different answers for one repo.
type Location struct {
	// Toplevel is the root of the working tree the directory belongs to.
	Toplevel string
	// Canonical is the root of the main checkout. It equals Toplevel
	// outside a linked worktree; for a linked one it is derived from the
	// shared git dir (see mainCheckout, including its known limits).
	Canonical string
	// Branch is the checked-out branch, "HEAD" when detached.
	Branch string
	// Repo is the canonical host/owner/repo identity from the origin
	// remote, "" without one. Every worktree of a repo shares it.
	Repo string
	// Linked reports a linked worktree (git worktree add), as opposed to
	// the main checkout.
	Linked bool
}

// locationArgs asks git for every Location field but Repo in one call. The
// paths come back absolute and physical: without --path-format, git prints
// the common dir relative to the directory it ran in.
var locationArgs = []string{
	"rev-parse", "--path-format=absolute",
	"--show-toplevel", "--git-dir", "--git-common-dir",
	"--abbrev-ref", "HEAD",
}

// locationLines is how many lines locationArgs prints: toplevel, git dir,
// common dir, branch.
const locationLines = 4

// LocationAt resolves where dir sits in git, with two git processes: the
// rev-parse call for the tree, the main checkout and the branch, and the
// origin lookup for Repo. Both are killed when ctx is done. A git field
// degrades to its zero value when git cannot say: dir is "" or outside a
// repo, the repo has no commit yet (HEAD does not resolve, so the whole
// rev-parse call fails), the git is too old to know --path-format, or ctx
// ran out. Repo comes from its own call, so an origin can still resolve
// when the rest is empty. Like every guard helper, it never errors and
// never guesses.
func LocationAt(ctx context.Context, dir string) Location {
	loc := parseLocation(gitOutputContext(ctx, dir, locationArgs...))
	loc.Repo = canonicalRepo(gitOutputContext(ctx, dir, "remote", "get-url", "origin"))
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
	top, gitDir, common, branch := lines[0], lines[1], lines[2], lines[3]
	if !filepath.IsAbs(top) || !filepath.IsAbs(gitDir) || !filepath.IsAbs(common) {
		return Location{}
	}
	loc := Location{Toplevel: top, Canonical: top, Branch: branch}
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
