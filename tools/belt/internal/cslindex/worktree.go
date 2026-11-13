package cslindex

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mad01/thismoon/kit/cslask"
)

// LinkedWorktree returns the root of the linked git worktree p sits in and
// the root of its main checkout, read from files alone. ok is false for a
// main clone, a submodule and a path outside any checkout.
func LinkedWorktree(p string) (root, main string, ok bool) {
	root = RepoRoot(p)
	if root == "" {
		return "", "", false
	}
	main, ok = mainCheckout(root)
	if !ok {
		return "", "", false
	}
	return root, main, true
}

// mainCheckout returns the main checkout of the linked worktree rooted at
// root. Its .git must be a pointer file whose git dir holds a commondir
// file, and commondir leads to the main checkout's .git. A main clone's .git
// is a directory, and a submodule's pointer leads to a git dir with no
// commondir, so neither is a linked worktree. csl's own worktree lookup
// reads the same files.
func mainCheckout(root string) (string, bool) {
	data, ok := SmallFile(filepath.Join(root, ".git")) // a directory fails here
	if !ok {
		return "", false
	}
	gitDir, ok := strings.CutPrefix(strings.TrimSpace(data), "gitdir:")
	if !ok {
		return "", false
	}
	gitDir = strings.TrimSpace(gitDir)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	common, ok := SmallFile(filepath.Join(gitDir, "commondir"))
	if !ok {
		return "", false
	}
	commonDir := strings.TrimSpace(common)
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitDir, commonDir)
	}
	commonDir = filepath.Clean(commonDir)
	if filepath.Base(commonDir) != ".git" {
		return commonDir, true // a bare clone is its own git dir
	}
	return filepath.Dir(commonDir), true
}

// maxSmallFile bounds the git bookkeeping files belt reads: HEAD, a .git
// file, commondir and gitdir each hold one short line.
const maxSmallFile = 4096

// SmallFile returns the content of a regular file, not a symlink, of at
// most maxSmallFile bytes. A hook runs on every tool call, so anything else
// fails before it is opened: a FIFO would block the open until a writer
// came, and a symlink could lead to a device read without end. The read
// stops at the bound too, should the file grow after the check. The
// guard's git discovery reads its files through it as well.
func SmallFile(path string) (string, bool) {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxSmallFile {
		return "", false
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxSmallFile+1))
	if err != nil || len(b) > maxSmallFile {
		return "", false
	}
	return string(b), true
}

// Located is a file csl can answer for, and how a csl call reaches it.
type Located struct {
	Root string // the working tree the file sits in
	Rel  string // the file's slash path below Root
	Repo string // csl's org/repo name; for a worktree, its main checkout's
	// Worktree marks Root as a linked worktree, which csl searches from an
	// index of its own.
	Worktree bool
}

// AskTooLong says why a file past csl_ask's line ceiling gets no csl_ask
// call: the large-read guard and the read-size hint both say it.
var AskTooLong = fmt.Sprintf(
	"csl_ask reads at most %d lines in one call, so it cannot answer from this file",
	cslask.MaxLinesCeiling,
)

// askScope is the argument that points a csl_ask call at the file's tree:
// repo for a main checkout, and the worktree root for a linked worktree, so
// csl answers from the worktree's index and not its main checkout's. csl
// reads repo as a regex over repo names, so it is escaped and anchored at
// both ends: unanchored, it also matches every repo its name prefixes.
func (l Located) askScope() string {
	if l.Worktree {
		return fmt.Sprintf("worktree: %q", l.Root)
	}
	return fmt.Sprintf("repo: %q", "^"+regexp.QuoteMeta(l.Repo)+"$")
}

// AskCall is the csl_ask call scoped to the file, for a file of lines lines
// as csl_ask counts them. zoekt reads f: as a regex over the path below the
// tree's root, so the path is escaped and anchored too: unanchored, it also
// matches a same-named file deeper in the tree. ZoektTerm quotes a path
// holding a space as one term. csl_ask reads a file whole or skips it, so
// the call raises max_lines past the default (cslask.MaxLines). ok is false
// past the ceiling, where no call can read the file; say AskTooLong then.
func (l Located) AskCall(lines int) (call string, ok bool) {
	maxLines, ok := cslask.MaxLines(lines)
	if !ok {
		return "", false
	}
	query := "f:" + ZoektTerm("^"+regexp.QuoteMeta(l.Rel)+"$")
	call = fmt.Sprintf(
		`csl_ask(question: "<what you need from it>", queries: [%q], %s`,
		query, l.askScope(),
	)
	if maxLines > 0 {
		call += fmt.Sprintf(", max_lines: %d", maxLines)
	}
	return call + ")", true
}

// Resolve resolves file to the tree csl knows it by and its path below
// that tree's root, ok only when csl has a shard for the repo. It follows a
// linked worktree to its main checkout: a file in a worktree whose main
// checkout has a shard resolves to the worktree. The main index never holds
// a worktree, but csl builds one an index of its own on the first call that
// names it, so the main checkout's shard is what says csl covers it. A
// checkout is named the way csl names its shard, by its directories
// (RepoName).
func Resolve(file string) (Located, bool) {
	root := RepoRoot(file)
	if root == "" {
		return Located{}, false
	}
	name := RepoName(root)
	main, worktree := mainCheckout(root)
	if worktree {
		name = RepoName(main)
	}
	repo, rel, ok := locateIn(root, name, file)
	if !ok {
		return Located{}, false
	}
	return Located{Root: root, Rel: rel, Repo: repo, Worktree: worktree}, true
}
