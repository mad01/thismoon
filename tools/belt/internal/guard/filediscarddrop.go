package guard

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/mad01/thismoon/tools/belt/internal/cslindex"
)

// This file holds the file-discard ladder for one path: an operand resolved
// without a shell, a glob expanded, and the cheap rungs before git.

// operand judges one operand. A variable a mktemp earlier in the command set
// passes while the operand stays inside what mktemp made (rmScan.target).
// What is left once the variable is empty, as after a failed mktemp or one
// set in a subshell, is judged as a path of its own: `"$D"/*` then names
// `/*`. Any other word must name a path without a shell (staticPath).
func (j *discardJudge) operand(call *lossCall, op lossOperand, s walkStep) *fileLoss {
	parts := op.word.parts
	if len(parts) == 0 || parts[0].kind != partVar || parts[0].text == "HOME" {
		return j.pathOperand(call, op, op.word, s)
	}
	if parts[0].text == "PWD" {
		return j.pwdOperand(call, op, s)
	}
	if _, known := j.scan.vars[parts[0].text]; !known {
		return j.unknown(unresolvable(call, op.word))
	}
	if _, ok := j.scan.target(op.word); !ok {
		return j.unknown(unresolvable(call, op.word))
	}
	rest := shWord{raw: op.word.raw, parts: parts[1:]}
	if emptyWord(rest) {
		return nil
	}
	loss := j.pathOperand(call, op, rest, s)
	if loss != nil {
		loss.why = "if `$" + parts[0].text + "` is empty, as after a failed mktemp, " + loss.why
		loss.userOnly = false
	}
	return loss
}

// pwdOperand judges an operand that starts with $PWD, the shell's directory
// where the walk can tell it (walkStep.pwd). Unquoted, a directory the
// shell may split or glob is not placed.
func (j *discardJudge) pwdOperand(call *lossCall, op lossOperand, s walkStep) *fileLoss {
	first := op.word.parts[0]
	if s.pwd == "" || !first.quoted && strings.ContainsAny(s.pwd, " \t\n*?[") {
		return j.unknown(unresolvable(call, op.word))
	}
	parts := append([]wordPart{{kind: partLit, text: s.pwd, quoted: true}}, op.word.parts[1:]...)
	return j.pathOperand(call, op, shWord{raw: op.word.raw, parts: parts}, s)
}

// pathOperand judges an operand word that names a path without a shell
// (rmScan.staticGlobs), a relative one from the shell's directory where the
// step runs. That directory must be sure: after `cd x;` or `cd x ||` the
// step may run where the shell was before (walkStep.cwdSure). A feeder
// prints a root as written, so a command that cuts what it prints must
// take the written text whole.
func (j *discardJudge) pathOperand(call *lossCall, op lossOperand, w shWord, s walkStep) *fileLoss {
	lit, active, ok := j.scan.staticGlobs(w)
	if !ok {
		return j.unknown(unresolvable(call, op.word))
	}
	if loss := writtenCut(call, op.filter, lit, active); loss != nil {
		return loss
	}
	if !path.IsAbs(lit) {
		switch {
		case s.cwd == "":
			return j.unknown(unresolvable(call, op.word))
		case !s.cwdSure:
			return j.unknown(&fileLoss{call: call, why: "the cd before it may not have run, so belt cannot " +
				"tell which directory `" + op.word.raw + "` is in: name the path in full, or " +
				"join the cd to the delete with &&"})
		}
		base := strings.TrimSuffix(s.cwd, "/") + "/"
		lit, active = base+lit, append(make([]bool, len(base)), active...)
	}
	slash := strings.HasSuffix(lit, "/")
	follows := op.follows || slash
	if slices.Contains(active, true) {
		return j.globDrop(call, op, lit, active, follows, slash)
	}
	return j.drop(call, op.tree, op.filter, kernelPath(lit, follows))
}

// writtenCut is the loss for a root a feeder prints as written when the
// command cuts it into other paths: a byte the command cuts at, outside the
// glob characters the shell expands.
func writtenCut(call *lossCall, f *pathFilter, lit string, active []bool) *fileLoss {
	if !f.splits() {
		return nil
	}
	for i := 0; i < len(lit); i++ {
		if !active[i] && strings.IndexByte(f.split, lit[i]) >= 0 {
			return cutLoss(call, f, lit)
		}
	}
	return nil
}

// emptyWord reports whether a word holds nothing but empty literal text.
func emptyWord(w shWord) bool {
	v, ok := w.literal()
	return ok && v == ""
}

// globEscape writes s as a path.Match pattern: a byte active marks stays a
// glob character, and every other metacharacter is escaped. A shell's `[!`
// negation is Go's `[^`. A nil active escapes them all.
func globEscape(s string, active []bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		on := active != nil && active[i]
		switch {
		case !on && strings.IndexByte(`*?[\`, c) >= 0:
			b.WriteByte('\\')
		case on && c == '[' && i+1 < len(s) && s[i+1] == '!':
			b.WriteString("[^")
			i++
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// kernelPath makes p physical the way the kernel walks it: every symlink
// on the way to the last part, and the last part too when the command writes
// through it or p ends in a slash. A `..` last part is resolved from the
// physical parent, as the kernel does.
func kernelPath(p string, follows bool) string {
	if follows {
		return physical(p)
	}
	dir, base := filepath.Split(p)
	if dir == "" {
		return filepath.Clean(p)
	}
	return filepath.Join(physical(dir), base)
}

// globDrop judges a glob operand the way bash and zsh expand it by default
// (splitGlob, globWalk). Its literal prefix folder takes the first rung:
// the root, the home directory or a folder holding it denies, unless the
// call is a replace, which writes only the files it matches. The prefix's
// own entries in a temp or cache folder pass, unless the command follows them
// through a symlink, as a trailing slash makes BSD rm do. Each match is
// then judged as a path of its own; with a trailing slash only folders
// match. Past maxPaths matches or globDirs folders, a glob whose matches
// stay in the prefix (leavesPrefix) is judged as the prefix, one tree, and
// any other is denied. A glob with no match drops the word as written,
// which bash hands to the command.
func (j *discardJudge) globDrop(
	call *lossCall, op lossOperand, lit string, active []bool, follows, slash bool,
) *fileLoss {
	n := len(strings.TrimRight(lit, "/"))
	prefix, parts, err := splitGlob(lit[:n], active[:n])
	if err != nil {
		return &fileLoss{call: call, why: "belt cannot tell which paths `" + op.word.raw +
			"` names: " + err.Error()}
	}
	dir := physical(prefix)
	if c, ok := j.spelled(dir); ok {
		dir = c
	}
	if loss := j.homeLoss(call, dir); loss != nil && !call.replaces() {
		return loss
	}
	if !follows && len(parts) == 1 && !parts[0].recursive && (j.inTemp(dir+"/x") || j.inCache(dir+"/x")) &&
		!op.filter.splits() {
		return nil
	}
	walk := globWalk{
		maxDirs: j.g.globDirs - j.globDirs, maxMatches: j.g.maxPaths, deadline: j.deadline,
	}
	err = walk.expand(dir, parts)
	j.globDirs += walk.dirs
	switch {
	case err != nil && leavesPrefix(parts, follows):
		return &fileLoss{call: call, why: fmt.Sprintf("belt cannot list every path `%s` names: "+
			"it matches more than %d paths or reads more than %d folders", op.word.raw,
			j.g.maxPaths, j.g.globDirs)}
	case err != nil:
		return j.drop(call, true, op.filter, dir)
	case len(walk.matches) == 0:
		return j.drop(call, op.tree, op.filter, kernelPath(lit, follows))
	}
	for _, m := range walk.matches {
		if slash && !isDir(m) {
			continue
		}
		if loss := cutLoss(call, op.filter, strings.TrimPrefix(m, dir)); loss != nil {
			return loss
		}
		if loss := j.drop(call, op.tree, op.filter, kernelPath(m, follows)); loss != nil {
			return loss
		}
	}
	return nil
}

// isDir reports whether p is a folder or a symlink to one.
func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// homeLoss is the loss for the root, the home directory or a directory
// that holds it, nil for any other path.
func (j *discardJudge) homeLoss(call *lossCall, p string) *fileLoss {
	what := ""
	for _, home := range []string{j.home, j.realHome} {
		switch {
		case p == "/":
			what = "the root of the file system"
		case home == "":
		case p == home:
			what = "the home directory"
		case strings.HasPrefix(home, p+"/"):
			what = "a directory that holds the home directory"
		}
	}
	if what == "" {
		return nil
	}
	return &fileLoss{call: call, why: "this " + call.verb() + " `" + p + "`, " + what, userOnly: true}
}

// inTemp reports whether p sits strictly inside a temp folder, the home
// directory read as given or as it lies on disk.
func (j *discardJudge) inTemp(p string) bool {
	return underTempRoot(p, j.home) || underTempRoot(p, j.realHome)
}

// cacheFolders are the folders under the home directory that hold only
// what a tool rebuilds; a delete at or under one passes. The list is code,
// not config.
var cacheFolders = []string{"Library/Developer/Xcode/DerivedData", "Library/Caches", ".cache"}

// inCache reports whether p, a path with its symlinks resolved, is a cache
// folder or lies under one, the home directory read as given or as it lies
// on disk. A cache folder counts by its own name only, so a path that
// resolves through a symlink into another folder is judged there.
func (j *discardJudge) inCache(p string) bool {
	for _, home := range []string{j.home, j.realHome} {
		if home == "" {
			continue
		}
		for _, c := range cacheFolders {
			if root := path.Join(home, c); p == root || strings.HasPrefix(p, root+"/") {
				return true
			}
		}
	}
	return false
}

// drop runs the verdict ladder on one physical path, first match wins: the
// root or the home directory denies; a temp path and a cache folder
// (inCache) pass; a missing path, an empty file or directory, a symlink
// taken as itself and a directory a non-recursive command leaves alone
// pass; a path in no work tree denies, and so do a work tree's top and
// git's own data. Anything else in a work tree waits for its git status
// (inRepos). From the work tree on, the path is spelled the way its folders
// list it (spelled), as git reads it. A filter whose tests match the path
// itself drops it whole (pathFilter.takes). A replace of a device, a pipe
// or a socket writes no file, and passes, and a replace of the files a
// feeder hands from a folder (tree) skips the home rung, as it writes only
// the files below it.
func (j *discardJudge) drop(call *lossCall, tree bool, filter *pathFilter, p string) *fileLoss {
	below := call.replaces() && tree
	if loss := j.homeLoss(call, p); loss != nil && !below {
		return loss
	}
	if j.inTemp(p) || j.inCache(p) {
		return j.tempCuts(call, tree, filter, p)
	}
	fi, err := os.Lstat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR):
		return nil
	case err != nil:
		return &fileLoss{call: call, why: fmt.Sprintf("belt cannot read `%s` (%v)", p, err)}
	case fi.Mode()&fs.ModeSymlink != 0, fi.Mode().IsRegular() && fi.Size() == 0:
		return nil
	case fi.IsDir() && (!tree || emptyDir(p)):
		return nil
	case call.replaces() && !tree && !fi.IsDir() && !fi.Mode().IsRegular():
		return nil
	}
	real, ok := j.spelled(p)
	switch {
	case !ok:
		return &fileLoss{call: call, why: "belt cannot find `" + p + "` under the name its folder lists"}
	case j.inTemp(real) || j.inCache(real):
		return j.tempCuts(call, tree, filter, real)
	}
	if loss := j.homeLoss(call, real); loss != nil && !below {
		return loss
	}
	if filter.takes(filepath.Base(real), fileType(fi)) {
		filter = filter.whole()
	}
	return j.inWorkTree(call, tree && fi.IsDir(), filter, real)
}

// tempCuts is the loss for a path a feeder prints under p, a temp folder
// the delete takes as a tree, that the command cuts into other paths: those
// land wherever it runs. nil when the command cuts nothing.
func (j *discardJudge) tempCuts(call *lossCall, tree bool, f *pathFilter, p string) *fileLoss {
	if !f.splits() || !tree || !isDir(p) {
		return nil
	}
	d := treeDrop{rel: ".", call: call, filter: f, prefix: p}
	var loss *fileLoss
	err := j.walkDirs(p, walkMode{skipGit: f.skipsGitData()}, func(dir string, entries []fs.DirEntry) walkNext {
		if loss = d.walkCuts(p, dir, entries); loss != nil {
			return walkStop
		}
		return walkOn
	})
	if err != nil {
		return &fileLoss{call: call, why: fmt.Sprintf("belt could not read every path the feeder "+
			"prints under `%s` (%v), and the command cuts what it prints", p, err)}
	}
	return loss
}

// emptyDir reports whether the directory at p holds no entry.
func emptyDir(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	return errors.Is(err, io.EOF)
}

// inWorkTree places a path that holds something in its work tree: none, the
// top or git's own data denies, and anything else waits for git status. dir
// marks a folder the call reaches into. A filter may take the top when the
// paths it hands are never git's own data: a feeder that skips .git, or
// find tests no entry of .git matches (gitDataLoss).
func (j *discardJudge) inWorkTree(call *lossCall, dir bool, filter *pathFilter, p string) *fileLoss {
	top := cslindex.RepoRoot(p)
	if top == "" {
		return &fileLoss{call: call, why: "this " + call.verb() + " `" + p + "`, which is in no git work " +
			"tree and no temp folder, so belt cannot tell whether it holds work no commit has"}
	}
	rel, err := filepath.Rel(top, p)
	switch {
	case err != nil:
		return &fileLoss{call: call, why: "belt cannot place `" + p + "` in its work tree"}
	case rel == "." && (filter == nil || filter.all):
		return topLoss(call, p)
	case rel == ".git" || strings.HasPrefix(rel, ".git/"):
		return &fileLoss{call: call, why: "this " + call.verb() + " `" + p + "`, which is git's own data"}
	case rel == "." && filter.hasTests():
		if loss := j.gitDataLoss(call, filepath.Join(p, ".git"), filter); loss != nil {
			return loss
		}
	case rel == "." && !filter.skipsGitData():
		return &fileLoss{call: call, why: "the paths it " + call.verb() + " may lie in `" + p +
			"/.git`, which is git's own data"}
	}
	j.queue(top, treeDrop{rel: rel, call: call, dir: dir && !filter.onlyTracked(), filter: filter})
	return nil
}

// gitDataLoss is the loss for find tests that match an entry of git's own
// data at gitDir, a work tree's .git or a bare repository, nil when none
// does.
func (j *discardJudge) gitDataLoss(call *lossCall, gitDir string, f *pathFilter) *fileLoss {
	hit, err := j.findMatch(gitDir, f)
	switch {
	case errors.Is(err, errWalkTooBig):
		return &fileLoss{call: call, why: fmt.Sprintf("belt could not check every entry of `%s` "+
			"against the find tests: it stops at %d folders and the time budget", gitDir, j.g.scanDirs)}
	case err != nil:
		return &fileLoss{call: call, why: fmt.Sprintf("belt could not read `%s` (%v)", gitDir, err)}
	case hit != "":
		return &fileLoss{call: call, why: "this " + call.verb() + " `" + hit + "`, which is git's own data"}
	}
	return nil
}

// topLoss is the loss for the top of a work tree. A delete of a linked
// worktree's top offers git's own command; a replace writes files in it,
// so no removal fits.
func topLoss(call *lossCall, top string) *fileLoss {
	loss := &fileLoss{call: call, why: "this " + call.verb() + " `" + top + "`, the top of a git work tree"}
	if !call.replaces() {
		loss.instead = worktreeRemoveHint(top)
	}
	return loss
}

// worktreeRemoveHint offers git's own command for a linked worktree, which
// refuses one with changes; "" for any other work tree.
func worktreeRemoveHint(top string) string {
	if _, _, linked := cslindex.LinkedWorktree(top); !linked {
		return ""
	}
	return "It is a linked worktree: remove it with `git worktree remove " + top +
		"`, which refuses a worktree with uncommitted changes."
}

// queue adds a drop to its work tree's list, a new one when the list was
// judged already.
func (j *discardJudge) queue(top string, d treeDrop) {
	for _, r := range j.repos {
		if r.top == top && !r.judged {
			r.drops = append(r.drops, d)
			return
		}
	}
	j.repos = append(j.repos, &repoDrops{top: top, drops: []treeDrop{d}})
}
