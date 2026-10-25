package record

import (
	"encoding/json"

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
// reports changed after it, one edit each, and counts the rest on the turn.
// A file the budget runs out on before its disk read is counted, not
// written: without the read a Bash edit has no after content, and without
// its placement it would land under an absolute path in no worktree.
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
		if err := rec.recordEdit(t); err != nil {
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
// unreached: the budget or git ran out before the check could say.
type verdict int

const (
	unreached verdict = iota
	keep
	clean
)

// authored keeps the targets the command left changed and counts the
// rest. The harness lists every file a command changed under the session's
// repository, including the ones a pull, checkout, reset or stash brought
// back to their committed state, and those are not changes the command
// made. git status runs once per worktree over that worktree's targets: a
// file it reports modified, added, deleted, renamed, untracked or ignored
// is kept, and one it does not report is clean. A file outside any
// worktree is kept unchecked, since git cannot say.
//
// The rule has a known cost: a file a command edits and commits in one go
// (sed ... && git commit) is clean afterwards and gets no edit. git blame
// names that commit, and the head-moved report at Stop ties the commit to
// the turn. An untracked file the command deleted is gone from status too
// and counts as clean; when the session had touched it, the next snapshot
// lists it as deleted.
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

// placeTargets places every target the budget reaches and groups the ones
// inside a worktree by it, in the order each worktree first appears. A
// target outside any worktree gets its keep verdict here; one git could
// not place in time stays unreached.
func (rec *recorder) placeTargets(targets []target) ([]*worktreeTargets, []verdict) {
	verdicts := make([]verdict, len(targets))
	var groups []*worktreeTargets
	byWorktree := map[string]*worktreeTargets{}
	for i, t := range targets {
		if rec.spent() {
			break
		}
		pl, answered := rec.place(t.name)
		if !answered {
			continue
		}
		wt := pl.loc.Worktree
		if wt == "" {
			verdicts[i] = keep
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

// checkStatus sets the verdict of each of a worktree's targets from one
// git status over their paths. When the budget is spent or git gives no
// status, they stay unreached.
func (rec *recorder) checkStatus(g *worktreeTargets, verdicts []verdict) {
	if rec.spent() {
		return
	}
	changed, ok := rec.changedPaths(g.worktree, g.paths)
	if !ok {
		return
	}
	for j, i := range g.index {
		verdicts[i] = clean
		if changed[g.paths[j]] {
			verdicts[i] = keep
		}
	}
}

// changedPaths runs git status in wt limited to paths, taken literally,
// and returns every path it reports: untracked files one by one, ignored
// files too, and a rename's original, which the rename deleted. It takes
// no index lock.
func (rec *recorder) changedPaths(wt string, paths []string) (map[string]bool, bool) {
	ctx, cancel := rec.gitCtx()
	defer cancel()
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

// reportSkipped spools the count of files the hook left out, when it left
// any, for serve to put on the turn, and logs it.
func (rec *recorder) reportSkipped(s store.SkippedFiles) error {
	if s.Total() == 0 {
		return nil
	}
	s.Tool, s.ToolUseID = rec.p.tool, rec.p.toolUseID
	rec.opts.Logf("%s %s: left %d changed files unrecorded: %d clean after it, %d past the budget",
		s.Tool, s.ToolUseID, s.Total(), s.Clean, s.Timeout)
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
