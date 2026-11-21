package guard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// This file holds the file-discard checks git answers: one git status per
// work tree, the proof of tracked files, and the check that the command
// gets each path a feeder prints whole.

// inRepos runs the last step for every work tree the command drops paths
// in, under one budget and one path cap for the whole call. A repository a
// walk finds under a narrowed drop is queued as a work tree of its own
// (nestedTop) and judged in turn.
func (j *discardJudge) inRepos() *fileLoss {
	if len(j.repos) == 0 {
		return nil
	}
	if gitEnvRedirected() {
		return &fileLoss{call: j.repos[0].drops[0].call, why: notListed(
			"a GIT_DIR-style variable is set").Error()}
	}
	ctx, cancel := context.WithDeadline(context.Background(), j.deadline)
	defer cancel()
	proved := 0
	for i := 0; i < len(j.repos); i++ {
		r := j.repos[i]
		r.judged = true
		if loss := j.inRepo(ctx, r, &proved); loss != nil {
			return loss
		}
	}
	return nil
}

// statusEntry is one entry of git status --porcelain=v1: its two-letter
// code and its path, a folder with a trailing slash.
type statusEntry struct{ code, path string }

// readStatus splits git status -z output into its entries.
func readStatus(out []byte) ([]statusEntry, error) {
	var entries []statusEntry
	for _, f := range nulFields(out) {
		if len(f) < 4 {
			return nil, errors.New("git status output")
		}
		entries = append(entries, statusEntry{code: f[:2], path: f[3:]})
	}
	return entries, nil
}

// inRepo judges one work tree's drops with one git status, each entry by
// the first drop that loses it (repoDrops.loser). No entry passes. An
// untracked file denies; an untracked folder, a nested repository, is
// judged by its own state (hiddenWork). Every path a feeder prints must
// reach the command whole (namesLoss), and every tracked entry must hold
// what a commit that counts holds (prove). An ignored entry passes, but a
// folder the command reaches into may still hold work status cannot show.
func (j *discardJudge) inRepo(ctx context.Context, r *repoDrops, proved *int) *fileLoss {
	pathspecs := make([]string, len(r.drops))
	for i, d := range r.drops {
		pathspecs[i] = d.rel
	}
	out, err := j.g.status(ctx, r.top, pathspecs)
	if err != nil {
		return &fileLoss{call: r.drops[0].call, why: notListed(err).Error()}
	}
	entries, err := readStatus(out)
	if err != nil {
		return &fileLoss{call: r.drops[0].call, why: notListed(err).Error()}
	}
	var tracked []string
	var hidden []hiddenDir
	for _, e := range entries {
		d, lost := r.loser(e.code, e.path)
		dir, isDir := strings.CutSuffix(e.path, "/")
		switch {
		case !lost:
		case e.code == "??" && isDir:
			hidden = append(hidden, hiddenDir{rel: dir, nested: true})
		case e.code == "??":
			return untrackedLoss(d.call, filepath.Join(r.top, e.path))
		case e.code == "!!" && isDir:
			hidden = append(hidden, hiddenDir{rel: dir})
		case e.code != "!!":
			tracked = append(tracked, e.path)
		}
	}
	if loss := j.namesLoss(ctx, r, entries); loss != nil {
		return loss
	}
	if *proved += len(tracked); *proved > j.g.maxPaths {
		return &fileLoss{call: r.drops[0].call, why: notListed(
			fmt.Sprintf("more than %d paths", j.g.maxPaths)).Error()}
	}
	if err := j.g.prove(ctx, r.top, tracked); err != nil {
		return &fileLoss{call: r.drops[0].call, why: "this deletes uncommitted work in " + r.top + ": " +
			err.Error()}
	}
	return j.hiddenWork(ctx, r, hidden)
}

// loser is the first drop that loses the status entry p (pathFilter.loses),
// the first drop when none holds p.
func (r *repoDrops) loser(code, p string) (treeDrop, bool) {
	rel := strings.TrimSuffix(p, "/")
	held := false
	for _, d := range r.drops {
		if !holds(d.rel, rel) {
			continue
		}
		held = true
		if d.filter.loses(r.top, d.rel, code, p) {
			return d, true
		}
	}
	return r.drops[0], !held
}

// holds reports whether the path at rel, relative to the top of a work
// tree ("." for the top), is p or holds it.
func holds(rel, p string) bool {
	return rel == "." || p == rel || strings.HasPrefix(p, rel+"/")
}

// below returns p, a path relative to the top that rel holds, relative to
// rel instead: "" for rel itself.
func below(rel, p string) string {
	if rel == "." {
		return p
	}
	return strings.TrimPrefix(strings.TrimPrefix(p, rel), "/")
}

// untrackedLoss is the loss for an untracked file: no commit holds it.
func untrackedLoss(call *lossCall, p string) *fileLoss {
	return &fileLoss{call: call, why: "this deletes `" + p + "`, which is untracked, so no commit holds it"}
}

// namesLoss is the loss for a path a feeder prints that the command cuts
// into other paths (pathFilter.split), which land wherever it runs: among
// the files git tracks under the drops that cut (git ls-files) and the
// ignored files git status lists. A walk reads the rest: an ignored folder
// or a nested repository (hiddenWork). A tracked path git prints in C
// quotes, as a git feeder without -z does, reaches the command changed.
func (j *discardJudge) namesLoss(ctx context.Context, r *repoDrops, entries []statusEntry) *fileLoss {
	var cutting []treeDrop
	var specs []string
	for _, d := range r.drops {
		if d.filter.splits() {
			cutting, specs = append(cutting, d), append(specs, d.rel)
		}
	}
	if len(cutting) == 0 {
		return nil
	}
	out, err := j.g.tracked(ctx, r.top, specs)
	if err != nil {
		return &fileLoss{call: cutting[0].call, why: notListed(err).Error()}
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		p, types, quoted, ok := readTracked(line)
		for _, d := range cutting {
			if loss := d.cutsAt(p, types, quoted || !ok); loss != nil {
				return loss
			}
		}
	}
	for _, e := range entries {
		if e.code != "!!" || strings.HasSuffix(e.path, "/") {
			continue
		}
		for _, d := range cutting {
			if !holds(d.rel, e.path) || !d.filter.loses(r.top, d.rel, e.code, e.path) {
				continue
			}
			if loss := d.cutsAt(e.path, "fl", false); loss != nil {
				return loss
			}
		}
	}
	return nil
}

// readTracked reads one line of git ls-files -s: the path, unquoted, the
// -type letter its mode gives (a symlink, a submodule's folder, else a
// file), and whether git printed it in C quotes. ok is false for a quoted
// path belt cannot unquote.
func readTracked(line string) (p, types string, quoted, ok bool) {
	meta, p, found := strings.Cut(line, "\t")
	if !found {
		return "", "", false, true
	}
	types = "f"
	switch {
	case strings.HasPrefix(meta, "120000"):
		types = "l"
	case strings.HasPrefix(meta, "160000"):
		types = "d"
	}
	if !strings.HasPrefix(p, `"`) {
		return p, types, false, true
	}
	u, err := strconv.Unquote(p)
	if err != nil {
		return p, types, true, false
	}
	return u, types, true, true
}

// cutsAt is the loss for p, a path relative to the top, when the drop's
// feeder prints a path to it that the command does not get whole, nil when
// it gets each one whole or the feeder prints nothing on the way to p.
// quoted marks a path git prints in C quotes. The prefix is checked where
// it is found (foundRepo), so only the path below it counts here.
func (d treeDrop) cutsAt(p, types string, quoted bool) *fileLoss {
	if p == "" || !holds(d.rel, p) {
		return nil
	}
	printed := d.filter.printed(below(d.rel, p), types)
	switch {
	case printed == "":
		return nil
	case quoted && d.filter.quoted:
		return &fileLoss{call: d.call, why: "git prints `" + filepath.Join(d.prefix, printed) +
			"` in C quotes, so the command gets another path", instead: wholePaths}
	case !d.filter.cuts(printed):
		return nil
	}
	return cutLoss(d.call, d.filter, filepath.Join(d.prefix, printed))
}

// wholePaths is the lossless form for a feed the command cuts.
const wholePaths = "To hand each path whole, print the paths NUL-separated (find -print0, " +
	"git ls-files -z, rg -0, grep --null) into xargs -0, or use find -exec ... {} +."

// cutLoss is the loss for a printed path the command cuts into other
// paths, nil when it takes p whole.
func cutLoss(call *lossCall, f *pathFilter, p string) *fileLoss {
	if !f.cuts(p) {
		return nil
	}
	c := p[strings.IndexAny(p, f.split)]
	return &fileLoss{call: call, why: "the feeder prints " + strconv.Quote(p) + ", which holds " +
		cutByteName(c) + ", and the command cuts it there into other paths", instead: wholePaths}
}

// cutByteName names a byte a command cuts a path at.
func cutByteName(c byte) string {
	switch c {
	case ' ':
		return "a space"
	case '\t':
		return "a tab"
	case '\n':
		return "a newline"
	case '\'', '"':
		return "a quote"
	case '\\':
		return "a backslash"
	case '*', '?', '[':
		return "a glob character"
	}
	return fmt.Sprintf("the byte %q", c)
}

// discardStatus lists the changed, untracked and ignored files under the
// pathspecs in the work tree at top. core.fsmonitor is off, so git starts
// no monitor program of the repo's choosing inside the hook. An ignored
// folder is listed as one entry (--ignored=matching), never its content:
// ignored files count as build output. A changed file a flag hides from
// git status (flaggedEdits) is listed as changed in the work tree, " M".
func discardStatus(ctx context.Context, top string, pathspecs []string) ([]byte, error) {
	args := append([]string{
		"-c", "core.fsmonitor=false", "status", "--porcelain=v1", "-z", "--untracked-files=all",
		"--ignored=matching", "--no-renames", "--ignore-submodules=none", "--",
	}, pathspecs...)
	out, err := runProofGit(ctx, top, args...)
	if err == nil {
		out, err = withFlaggedEdits(ctx, top, pathspecs, out)
	}
	switch {
	case err == nil:
		return out, nil
	case ctx.Err() != nil:
		return nil, errors.New("git status timed out")
	}
	return nil, errors.New("git status failed")
}

// withFlaggedEdits appends to the git status -z output status a " M" entry
// for each file under the pathspecs in the work tree at top that a flag
// hides from git status and that status does not list already.
func withFlaggedEdits(ctx context.Context, top string, pathspecs []string, status []byte) ([]byte, error) {
	edits, err := flaggedEdits(ctx, top, nil, pathspecs)
	if err != nil || len(edits) == 0 {
		return status, err
	}
	entries, err := readStatus(status)
	if err != nil {
		return nil, err
	}
	for _, p := range edits {
		if !slices.ContainsFunc(entries, func(e statusEntry) bool { return e.path == p }) {
			status = append(status, " M "+p+"\x00"...)
		}
	}
	return status, nil
}

// flaggedEdits lists the files under the pathspecs, relative to dir, that
// the work tree git reads with at changed where git status does not look:
// a file marked assume-unchanged (a lowercase tag in git ls-files -v) or
// skip-worktree (S) that exists and does not hold what its index entry
// holds, compared as git object ids (worktreeMatches), so a filter or a
// line-ending conversion reads as a change. A skip-worktree file a sparse
// checkout leaves out is missing and loses nothing. A submodule's entry is
// left to the search for repositories, and an unmerged one to git status,
// which lists it whatever its flags. core.fsmonitor is off, as for git
// status.
func flaggedEdits(ctx context.Context, dir string, at, pathspecs []string) ([]string, error) {
	out, err := runProofGit(ctx, dir, slices.Concat(at, []string{"-c", "core.fsmonitor=false", "ls-files",
		"-v", "-s", "-z", "--"}, pathspecs)...)
	if err != nil {
		return nil, err
	}
	var edits []string
	for _, f := range nulFields(out) {
		meta, p, ok := strings.Cut(f, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 4 {
			return nil, errors.New("git ls-files printed an entry belt cannot read")
		}
		tag, mode, oid, stage := fields[0], fields[1], fields[2], fields[3]
		if !flagHides(tag) || mode == gitlinkMode || stage != "0" {
			continue
		}
		same, err := worktreeMatches(ctx, filepath.Join(dir, p), []treeEntry{{mode: mode, oid: oid, size: -1}})
		switch {
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case err != nil || !same:
			edits = append(edits, p)
		}
	}
	return edits, nil
}

// flagHides reports whether git ls-files -v tags an index entry with a
// flag that keeps git status from reading its file: a lowercase tag for
// assume-unchanged, S for skip-worktree.
func flagHides(tag string) bool {
	return tag == "S" || strings.ToUpper(tag) != tag
}

// listTracked lists the files git tracks under the pathspecs in the work
// tree at top, with their modes, as git ls-files prints them without -z:
// a path with odd bytes in C quotes, as the repository's core.quotePath
// says.
func listTracked(ctx context.Context, top string, pathspecs []string) ([]byte, error) {
	return runProofGit(ctx, top, append([]string{"ls-files", "-s", "--"}, pathspecs...)...)
}

// listWorktrees lists the work trees of the repository at top, the main
// one first, with git worktree list.
func listWorktrees(ctx context.Context, top string) ([]string, error) {
	out, err := runProofGit(ctx, top, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var trees []string
	for _, f := range nulFields(out) {
		if p, ok := strings.CutPrefix(f, "worktree "); ok {
			trees = append(trees, p)
		}
	}
	return trees, nil
}

// proveCommitted checks each tracked path (relative to top) against HEAD,
// its upstream and origin's default branch with git-discard's proof
// (proofRepo.checkDrops): the worktree file must equal one of them.
func proveCommitted(ctx context.Context, top string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	r := proofRepo{ctx: ctx, dir: top, top: top, maxPaths: maxProofPaths}
	r.shared = r.sharedCommits()
	drops := make([]drop, len(paths))
	for i, p := range paths {
		drops[i] = drop{path: p}
	}
	return r.checkDrops(drops)
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
