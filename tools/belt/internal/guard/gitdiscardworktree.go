package guard

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
)

// errNoWorktree is the cause when belt cannot name the worktree a remove
// deletes.
var errNoWorktree = errors.New("belt cannot tell which worktree it removes")

// listedWorktree is one entry of `git worktree list --porcelain -z`.
type listedWorktree struct {
	path string
	// main marks the first entry, the main worktree, which git never
	// removes.
	main bool
}

// worktreeRemove is a `worktree remove` as belt reads it, once per
// invocation: the listed worktrees it may delete, and whether --force is
// on.
type worktreeRemove struct {
	targets []listedWorktree
	forced  bool
}

// removedWorktrees reads `git worktree remove [-f]... <name>`, run in
// c.dir: the worktrees removeTargets places it on, and whether it is
// forced, as it is when a command in front of git may hand it a -f (fed).
// The name gets the expansions the shell gives an unquoted ~ or a quoted
// path (expandCdTarget). A name belt cannot read, a list git cannot give and
// a name that matches no listed worktree are refused.
func (g *GitDiscard) removedWorktrees(c gitInvocation, fed bool) (worktreeRemove, error) {
	if len(c.args) == 0 || c.args[0] != "remove" {
		return worktreeRemove{}, errNoWorktree
	}
	w, err := readGitWords(c.args[1:], worktreeRemoveOptions)
	names := slices.Concat(w.operands, w.paths)
	if err != nil || len(names) != 1 {
		return worktreeRemove{}, errNoWorktree
	}
	name := expandCdTarget(names[0])
	if name == "" || strings.ContainsRune(name, '\\') {
		return worktreeRemove{}, errNoWorktree
	}
	listed, err := g.worktrees(c.dir)
	if err != nil {
		return worktreeRemove{}, errNoWorktree
	}
	targets := removeTargets(c.dir, name, listed)
	if len(targets) == 0 {
		return worktreeRemove{}, errNoWorktree
	}
	return worktreeRemove{targets: targets, forced: fed || w.on("force")}, nil
}

// removeTargets returns the listed worktrees a remove of name, run in dir,
// may pick. git takes the one worktree whose path ends in name at a
// directory boundary, else the one at name as a path, both paths resolved
// through their symlinks, and compares them as the file system does, with
// or without case (find_worktree). git starts a relative name from the
// physical directory, so a `..` there leaves the target of a symlinked dir,
// not the link. The name therefore counts from dir as written and from dir
// with its symlinks resolved. Every match of any kind counts, without case
// and with or without symlinks, so the list holds whichever git picks.
func removeTargets(dir, name string, listed []listedWorktree) []listedWorktree {
	ats := []string{name}
	if !filepath.IsAbs(name) {
		ats = []string{dir + "/" + name}
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			ats = append(ats, real+"/"+name)
		}
	}
	var picked []listedWorktree
	for _, wt := range listed {
		at := func(p string) bool { return samePath(p, wt.path) }
		if endsInDir(wt.path, name) || slices.ContainsFunc(ats, at) {
			picked = append(picked, wt)
		}
	}
	return picked
}

// endsInDir reports whether path ends in suffix at a directory boundary,
// without case.
func endsInDir(path, suffix string) bool {
	start := len(path) - len(suffix)
	if suffix == "" || start < 0 || !strings.EqualFold(path[start:], suffix) {
		return false
	}
	return start == 0 || path[start-1] == '/'
}

// samePath reports whether a and b name one directory, without case, as
// written or with their symlinks resolved.
func samePath(a, b string) bool {
	for _, x := range pathForms(a) {
		for _, y := range pathForms(b) {
			if strings.EqualFold(x, y) {
				return true
			}
		}
	}
	return false
}

// pathForms returns p cleaned, and with its symlinks resolved when it
// resolves. The resolved form follows each symlink before the `..` after
// it, as the file system does.
func pathForms(p string) []string {
	forms := []string{filepath.Clean(p)}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		forms = append(forms, real)
	}
	return forms
}

// keepsWorktreeCommits refuses a remove of a linked worktree whose HEAD and
// HEAD reflog are the last refs that hold some commits (removeStranded),
// detached or on a branch. git removes such a worktree without --force and
// without a warning, and its admin dir, which holds that HEAD and reflog,
// goes with it. git refuses to remove the main worktree, so it loses
// nothing.
func (g *GitDiscard) keepsWorktreeCommits(ctx context.Context, dir string, wt listedWorktree) error {
	if wt.main {
		return nil
	}
	lost, err := g.stranded(ctx, dir, wt)
	if err != nil {
		return err
	}
	return lost.refusal("it deletes that worktree's HEAD and HEAD reflog")
}

// errNoPrune is the cause when belt cannot read a prune's words.
var errNoPrune = errors.New("belt cannot read which worktrees it prunes")

// judgePrune refuses a `worktree prune` that deletes the admin dir of a
// worktree whose HEAD and HEAD reflog are the last refs that hold some
// commits (pruneStranded). git prunes each worktree whose folder is gone,
// with its HEAD and HEAD reflog, without a warning. The dry run belt asks
// takes the prune's own expire words (pruneExpiry).
func (g *GitDiscard) judgePrune(c gitInvocation, fed bool) error {
	expire, err := pruneExpiry(c.args[1:], fed)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), proofBudget)
	defer cancel()
	lost, err := g.pruned(ctx, c.dir, expire)
	if err != nil {
		return err
	}
	return lost.refusal("it prunes each worktree whose folder is gone, with its HEAD and HEAD reflog")
}

// pruneExpiry reads a prune's words (worktreePruneOptions) and returns its
// --expire and --no-expire words for the dry run. A value the shell may
// still glob or unquote (asWritten), or a prune a command in front of git
// may hand more words (fed), gives none: the dry run then names every
// worktree a prune with no --expire deletes, the most any prune deletes.
// Words belt cannot read are refused, and so is a value the shell may still
// expand ($, a backtick, a subshell), which readGitWords refuses before
// asWritten sees it.
func pruneExpiry(args []string, fed bool) ([]string, error) {
	w, err := readGitWords(args, worktreePruneOptions)
	if err != nil {
		return nil, errNoPrune
	}
	var expire []string
	for _, o := range w.opts {
		if o.name != "expire" {
			continue
		}
		if fed || !asWritten(o.value) {
			return nil, nil
		}
		expire = append(expire, o.asWord())
	}
	return expire, nil
}

// gitWorktrees lists the worktrees of the repository at dir, the main one
// first, as `git worktree list --porcelain -z` prints them: each entry
// starts with a `worktree <path>` field.
func gitWorktrees(dir string) ([]listedWorktree, error) {
	ctx, cancel := context.WithTimeout(context.Background(), proofBudget)
	defer cancel()
	out, err := runProofGit(ctx, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var list []listedWorktree
	for _, f := range nulFields(out) {
		if p, ok := strings.CutPrefix(f, "worktree "); ok {
			list = append(list, listedWorktree{path: p, main: len(list) == 0})
		}
	}
	return list, nil
}
