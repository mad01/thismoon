package record

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// toolBash is the shell tool. With the harness setting bashEditDiffEnabled
// on, its PostToolUse tool_response carries bashEditDiff: the files the
// command changed under the repository, up to five with their hunks in
// files, and every one by absolute path in changedFiles. The harness calls
// the list best effort and its shape public beta, so every field is read
// on its own and one of the wrong type reads as absent.
const toolBash = "Bash"

// recordBash records the files a Bash command changed that git still
// reports changed after it, one edit each, and counts the rest on the turn
// as skipped. A file the budget runs out on before its disk read is
// counted, not written: without the read a Bash edit has no after content,
// and without its placement it would land under an absolute path in no
// worktree.
func (rec *recorder) recordBash() error {
	targets := rec.bashTargets()
	if len(targets) == 0 {
		return nil
	}
	kept, skipped := rec.authored(targets)
	for i, t := range kept {
		if rec.spent() {
			skipped.Timeout += len(kept) - i
			break
		}
		env, chunks, cut := rec.captureEdit(t)
		if cut {
			skipped.Timeout++
			continue
		}
		if err := rec.writeEdit(env, chunks); err != nil {
			return err
		}
	}
	return rec.reportSkipped(skipped)
}

// bashTargets lists one target per file the command's diff names: the files
// with hunks first, in the harness's order, then every other changed path
// by path alone. A Bash call without a diff names none.
func (rec *recorder) bashTargets() []target {
	diff := rec.p.response.obj("bashEditDiff")
	if diff == nil {
		return nil
	}
	var out []target
	seen := map[string]bool{}
	for _, f := range objects(diff.array("files")) {
		name := f.str("filePath")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, target{
			name: name, index: len(out), kind: bashKind(f), hunks: f.array("hunks"),
		})
	}
	for _, name := range strs(diff.array("changedFiles")) {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, target{
			name: name, index: len(out), kind: store.EditUpdate, pathOnly: true,
		})
	}
	return out
}

// verdict is what the clean check made of one target. The zero value is
// unreached: the budget or a git deadline ran out before the check could
// say.
type verdict int

const (
	unreached verdict = iota
	keep
	clean
	// failed is a target whose status git could not give: it failed or
	// said too much, or the target is gone and no worktree holds it.
	failed
)

// authored keeps the targets the command left changed and counts the
// rest. The harness lists every file a command changed under the session's
// repository, including the ones a pull, checkout, reset or stash brought
// back to their committed state, and the command authored no Edit of
// those. git status runs once per worktree over that worktree's targets: a
// file it reports modified, added, deleted, renamed, untracked or ignored
// is kept, and one it does not report is clean. A file outside any
// worktree is kept unchecked, since git cannot say, unless it is gone:
// nothing of it is left to record but an absolute path, so it is failed.
//
// The rule has a known cost: a file a command edits and commits in one go
// (sed ... && git commit) is clean afterwards and gets no edit. git blame
// names that commit. The head-moved report at Stop ties the commit to the
// turn only when the session had already recorded an edit in that
// worktree, since only such a worktree is touched. An untracked file the
// command deleted is gone from status too and counts as clean; when the
// session had touched it, the next snapshot lists it as deleted.
func (rec *recorder) authored(targets []target) ([]target, store.SkippedFiles) {
	groups, verdicts := rec.placeTargets(targets)
	for _, g := range groups {
		rec.checkStatus(g, verdicts)
	}
	var kept []target
	var skipped store.SkippedFiles
	for i, v := range verdicts {
		switch v {
		case keep:
			kept = append(kept, targets[i])
		case clean:
			skipped.Clean++
		case failed:
			skipped.Failed++
		case unreached:
			skipped.Timeout++
		}
	}
	return kept, skipped
}

// worktreeTargets is the targets one worktree holds: their positions in
// the target list and their worktree-relative paths.
type worktreeTargets struct {
	worktree string
	index    []int
	paths    []string
}

// placeTargets places every target the budget reaches, once, and keeps
// the placement on the target for its capture. It groups the targets
// inside a worktree by it, in the order each worktree first appears. A
// target outside any worktree gets its verdict here; one git could not
// place in time stays unreached.
func (rec *recorder) placeTargets(targets []target) ([]*worktreeTargets, []verdict) {
	verdicts := make([]verdict, len(targets))
	var groups []*worktreeTargets
	byWorktree := map[string]*worktreeTargets{}
	for i := range targets {
		if rec.spent() {
			break
		}
		pl, answered := rec.placeBash(targets[i].name)
		if !answered {
			continue
		}
		targets[i].where = &pl
		wt := pl.loc.Worktree
		if wt == "" {
			verdicts[i] = keep
			if gone(pl.abs) {
				verdicts[i] = failed
			}
			continue
		}
		g, ok := byWorktree[wt]
		if !ok {
			g = &worktreeTargets{worktree: wt}
			byWorktree[wt] = g
			groups = append(groups, g)
		}
		g.index = append(g.index, i)
		g.paths = append(g.paths, pl.rel)
	}
	return groups, verdicts
}

// placeBash is place for one file of a Bash call, which can list hundreds
// of files in a few worktrees. A file in a worktree the Run has located
// already is placed by its path, with no git call, unless a directory on
// the way holds a repository of its own. So only the first file of each
// worktree costs git calls.
func (rec *recorder) placeBash(name string) (placement, bool) {
	abs := Resolve(name, rec.p.cwd)
	if found, ok := rec.holder(abs); ok && !nestedRepo(found.Worktree, abs) {
		loc, rel := Placed(found, abs)
		return placement{abs: abs, rel: rel, loc: loc}, true
	}
	return rec.placeAbs(abs)
}

// holder is the location of the innermost worktree the Run has located
// that holds abs.
func (rec *recorder) holder(abs string) (store.Location, bool) {
	var best store.Location
	for _, loc := range rec.locs {
		if len(loc.Worktree) > len(best.Worktree) && holds(loc.Worktree, abs) {
			best = loc
		}
	}
	return best, best.Worktree != ""
}

// holds reports whether p is root or lies under it.
func holds(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && filepath.IsLocal(rel)
}

// nestedRepo reports whether a directory between root and abs, abs's own
// directory included, is a .git or holds one. A repository, submodule or
// linked worktree of its own sits there, and only git can place abs.
func nestedRepo(root, abs string) bool {
	rel, err := filepath.Rel(root, filepath.Dir(abs))
	if err != nil || rel == "." {
		return false
	}
	dir := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		dir = filepath.Join(dir, part)
		if part == ".git" {
			return true
		}
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
	}
	return false
}

// checkStatus sets the verdict of each of a worktree's targets from one
// git status over their paths. When the budget is spent first, or the
// status call runs past its deadline, they stay unreached; when git fails
// or says more than moondb.MaxStatusBytes, they are failed.
func (rec *recorder) checkStatus(g *worktreeTargets, verdicts []verdict) {
	if rec.spent() {
		return
	}
	ctx, cancel := rec.gitCtx()
	defer cancel()
	changed, ok := changedPaths(ctx, g.worktree, g.paths)
	for j, i := range g.index {
		switch {
		case !ok && ctx.Err() != nil:
			verdicts[i] = unreached
		case !ok:
			verdicts[i] = failed
		case changed[g.paths[j]]:
			verdicts[i] = keep
		default:
			verdicts[i] = clean
		}
	}
}

// changedPaths runs git status in wt limited to paths, taken literally,
// and returns every path it reports: untracked files one by one, ignored
// files too, and a rename's original, which the rename deleted. It takes
// no index lock.
func changedPaths(ctx context.Context, wt string, paths []string) (map[string]bool, bool) {
	// Claude Code takes these paths from git, so a tracked file arrives in
	// the index's case and the literal pathspec matches it.
	args := append([]string{
		"--no-optional-locks", "--literal-pathspecs", "status", "--porcelain=v2", "-z",
		"-uall", "--ignored=traditional", "--ignore-submodules=all", "--",
	}, paths...)
	out, ok := gitBytes(ctx, wt, moondb.MaxStatusBytes, args...)
	if !ok {
		return nil, false
	}
	changed := map[string]bool{}
	for _, e := range parseStatus(out) {
		changed[e.path] = true
		if e.orig != "" {
			changed[e.orig] = true
		}
	}
	return changed, true
}

// reportSkipped spools the count of files the hook skipped, when it
// skipped any, for serve to put on the turn, and logs it.
func (rec *recorder) reportSkipped(s store.SkippedFiles) error {
	if s.Total() == 0 {
		return nil
	}
	s.Tool, s.ToolUseID = rec.p.tool, rec.p.toolUseID
	rec.opts.Logf("%s %s: skipped %d changed files: "+
		"%d clean after the command, %d past the budget, %d status failed",
		s.Tool, s.ToolUseID, s.Total(), s.Clean, s.Timeout, s.Failed)
	env := rec.envelope(spool.KindSkipped, rec.p.toolUseID+":skipped")
	env.Skipped = &s
	return rec.spool.Write(env, nil)
}

// bashKind is what the diff says the command did to one file.
func bashKind(f fields) store.EditKind {
	switch {
	case f.flag("created"):
		return store.EditCreate
	case f.flag("deleted"):
		return store.EditDelete
	default:
		return store.EditUpdate
	}
}

// objects decodes the elements of a JSON array that are objects, one level
// deep; nil for a value that is not an array.
func objects(raw json.RawMessage) []fields {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var out []fields
	for _, item := range items {
		var f fields
		if json.Unmarshal(item, &f) == nil && f != nil {
			out = append(out, f)
		}
	}
	return out
}

// strs decodes the elements of a JSON array that are strings.
func strs(raw json.RawMessage) []string {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var out []string
	for _, item := range items {
		var s string
		if json.Unmarshal(item, &s) == nil {
			out = append(out, s)
		}
	}
	return out
}
