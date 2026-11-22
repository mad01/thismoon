package guard

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// This file holds file-discard's reading of a build target a command runs:
// a make target (readMake), an npm, yarn or pnpm script (readNpm, readYarn,
// readPnpm) and a just recipe (readJust). belt reads the target's body from
// its build file and walks it the way the shell runs it (walkCommands),
// from the folder the tool runs it in, so the rm, find and language deletes
// in it take the same ladder as the command's own. The targets a target
// runs first are read too. A body from a build file git holds committed is
// trusted (discardJudge.unknown).

// buildBody is one body a build target runs: what runs it, for a denial;
// its shell text; the folder it runs in; and the file it is written in, for
// the trust rule. unread says why belt cannot read the body, "" when it
// can. lang is the script language the tool runs the body in, as with a
// make SHELL or a just shell of python3 (langDeleteCalls), "" for a shell.
type buildBody struct {
	what, text, dir, file string
	unread                string
	lang                  string
}

// buildRun is what one build command runs: its bodies in order, the build
// file it reads them from with every file that file pulls in, and their
// text. unread says why belt cannot tell which targets run, "" when it
// can. file is "" for a build file no file holds, as one make reads from
// standard input. packages is the folder of a run across workspaces, whose
// every package.json the trust rule needs committed (committedPackages).
type buildRun struct {
	file     string
	files    []string
	text     string
	bodies   []buildBody
	unread   string
	packages string
}

// buildReader reads a build command's words past its name, with the
// variables the command sets in front of it (env), run from dir, into what
// it runs, reading its files through v. ok is false for a command that runs
// no target (`npm install`, `make --version`) or whose build file belt
// cannot find.
type buildReader func(args []shWord, env map[string]string, dir string, v *buildView) (run buildRun, ok bool)

// buildView is how a build tool finds its files when it runs: as the steps
// of the command before it leave them (discardJudge.newBody), else as they
// are on disk. blind names a file a step changes in a way belt cannot read:
// reading it fails, and the run is one belt cannot read.
type buildView struct {
	j     *discardJudge
	blind string
}

// read returns the text of the build file at p, as readBuildFile does.
func (v *buildView) read(p string) (string, bool) {
	switch text, state := v.j.newBody(p); state {
	case bodyWritten:
		return text, len(text) <= maxScriptSize
	case bodyBlind:
		v.blind = cmp.Or(v.blind, p)
		return "", false
	}
	return readBuildFile(p)
}

// exists reports whether the tool finds a file at p: one a step before it
// writes (discardJudge.writes), or a regular file on disk.
func (v *buildView) exists(p string) bool {
	if v.j.writes(p) {
		return true
	}
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// buildReaders are the build tools file-discard reads, by command name.
var buildReaders = map[string]buildReader{
	"make": readMake, "gmake": readMake,
	"npm": readNpm, "yarn": readYarn, "pnpm": readPnpm,
	"just": readJust,
}

// maxBuildDepth bounds how deep build targets run one another (a recipe
// that runs make again), and maxBuildBodies how many bodies one command's
// build targets may hand belt in all.
const (
	maxBuildDepth  = 4
	maxBuildBodies = 2048
)

// overBodies is the loss for build targets past maxBuildBodies bodies.
func overBodies(file string) *fileLoss {
	return &fileLoss{why: fmt.Sprintf("belt reads at most %d build target bodies in one command, "+
		"and the targets `%s` runs hold more", maxBuildBodies, file)}
}

// buildTargets judges the build targets a step runs: the command word a
// buildReaders tool, past wrappers and assignments (runAt). A build file
// belt cannot place, as from an unknown folder, adds nothing. One a step
// before it changes in a way belt cannot read (buildView.blind) is not what
// runs, so belt cannot tell what the targets delete.
func (j *discardJudge) buildTargets(s walkStep) *fileLoss {
	at := runAt(s.tokens)
	if at >= len(s.tokens) || len(s.words) != len(s.tokens) || s.cwd == "" {
		return nil
	}
	read, ok := buildReaders[runName(s.words[at].shWord)]
	if !ok {
		return nil
	}
	env := map[string]string{}
	for _, w := range s.words[:at] {
		if name, value, ok := w.assignment(); ok {
			env[name] = "${__env_" + name + "}"
			if v, lit := (shWord{parts: value}).literal(); lit {
				env[name] = v
			}
		}
	}
	args := make([]shWord, 0, len(s.words)-at-1)
	for _, w := range s.words[at+1:] {
		args = append(args, w.shWord)
	}
	tool, v := runName(s.words[at].shWord), &buildView{j: j}
	run, ok := read(args, env, s.cwd, v)
	switch {
	case v.blind != "":
		return &fileLoss{why: "this command changes `" + v.blind + "` before " + tool + " reads it, in a way " +
			"belt cannot read, so it cannot tell what the target deletes: run the change as a command of its own first"}
	case !ok:
		return nil
	}
	return j.runBuild(tool, run)
}

// runBuild judges what one build command runs. One belt cannot tell is a
// delete whose path it cannot read (unreadRun), and so is one past the
// call's time budget. A build file a git command earlier in the command
// may change holds what git holds, which belt does not read, so the run
// denies when what belt read of it (bodyText) or the text of that step
// names a delete (namesDelete).
func (j *discardJudge) runBuild(tool string, run buildRun) *fileLoss {
	for _, f := range run.files {
		if _, state := j.newBody(f); state == bodyGit {
			if by, _ := j.touched(f); namesDelete(run.bodyText() + "\n" + by) {
				return &fileLoss{why: "this command changes `" + f + "` before " + tool +
					" reads it, so belt cannot tell what the target deletes: run the change as a command " +
					"of its own first"}
			}
			return nil
		}
	}
	if run.unread != "" {
		return j.unreadRun(run)
	}
	if j.depth >= maxBuildDepth {
		return j.unknown(&fileLoss{why: fmt.Sprintf("belt reads build targets only %d deep, and `%s` "+
			"runs one deeper", maxBuildDepth, tool)})
	}
	j.depth++
	defer func() { j.depth-- }()
	for _, b := range run.bodies {
		if j.builds++; j.builds > maxBuildBodies {
			return j.unknown(overBodies(run.file))
		}
		if time.Now().After(j.deadline) {
			return j.unknown(&fileLoss{why: "belt ran out of time reading the build targets `" + tool + "` runs"})
		}
		if loss := j.walkBody(b, run.files); loss != nil {
			return loss
		}
	}
	return nil
}

// unreadRun is the loss for a build run belt cannot read: a delete whose
// path it cannot read, which the trust rule passes only when every build
// file the run reads is committed code, and for a run across workspaces
// every package.json under its folder too (committedPackages). A run whose
// build file no file holds is never trusted.
func (j *discardJudge) unreadRun(run buildRun) *fileLoss {
	loss := &fileLoss{why: run.unread}
	if run.file == "" {
		return loss
	}
	for _, f := range run.files {
		if !j.committedCode(f) {
			return j.inFile(f, func() *fileLoss { return j.unknown(loss) })
		}
	}
	if run.packages != "" && !j.committedPackages(run.packages) {
		loss.why += ", and belt trusts such a run only when git holds every package.json under `" +
			run.packages + "` committed"
		return loss
	}
	return j.inFile(run.file, func() *fileLoss { return j.unknown(loss) })
}

// bodyText is the text of a run's bodies, or the whole build file's when
// belt could not read them all.
func (r buildRun) bodyText() string {
	if r.unread != "" {
		return r.text
	}
	var b strings.Builder
	for _, body := range r.bodies {
		if body.unread != "" {
			return r.text
		}
		b.WriteString(body.text + "\n")
	}
	return b.String()
}

// walkBody walks one build target body as the code of its file, its values
// coming from every file its build run reads (inCode), in a shell of its
// own: the mktemp variables and the -c scripts judged with a consumer's
// input start empty there. A body in a script language (buildBody.lang) is
// judged as a script of that language (scriptLoss). A loss in it, and each
// drop it leaves for git, names the body (buildBody.what).
func (j *discardJudge) walkBody(b buildBody, files []string) *fileLoss {
	if b.unread != "" {
		return j.inCode(b.file, files, func() *fileLoss { return j.unknown(&fileLoss{via: b.what, why: b.unread}) })
	}
	covered, vars := j.covered, j.scan.vars
	j.covered, j.scan.vars = map[int]bool{}, map[string]tempVar{}
	defer func() { j.covered, j.scan.vars = covered, vars }()
	queued := make(map[*repoDrops]int, len(j.repos))
	for _, r := range j.repos {
		queued[r] = len(r.drops)
	}
	var loss *fileLoss
	_ = j.inCode(b.file, files, func() *fileLoss {
		if b.lang != "" {
			sc := langScript{lang: b.lang, runner: b.what, text: b.text, ok: true, file: b.file}
			loss = j.scriptLoss(sc, walkStep{cwd: b.dir, cwdSure: true, pwd: b.dir})
			return nil
		}
		walkCommands(b.text, b.dir, func(s walkStep) {
			if loss == nil {
				loss = j.step(s)
			}
		})
		return nil
	})
	for _, r := range j.repos {
		for _, d := range r.drops[queued[r]:] {
			if d.call != nil && d.call.via == "" {
				d.call.via = b.what
			}
		}
	}
	if loss != nil && loss.where() == "" {
		loss.via = b.what
		if loss.call != nil {
			loss.call.via = b.what
		}
	}
	return loss
}

// namesDelete reports whether text names a loss command (namesLossText)
// or a recursive delete of a script language (langDeleteText).
func namesDelete(text string) bool {
	return namesLossText(text) || langDeleteText.MatchString(text)
}

// readBuildFile reads a build file up to maxScriptSize. ok is false for a
// path that is no regular file or that belt cannot read, and for one past
// the cap.
func readBuildFile(p string) (string, bool) {
	// A FIFO would block the open, so only a regular file is opened.
	if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
		return "", false
	}
	f, err := os.Open(p)
	if err != nil {
		return "", false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxScriptSize+1))
	if err != nil || len(b) > maxScriptSize {
		return "", false
	}
	return string(b), true
}

// bodyWhat names a build target body for a denial: the tool's kind of
// target, its name, the file, and the line it runs.
func bodyWhat(kind, name, file, line string) string {
	return fmt.Sprintf("the %s `%s` in %s runs `%s`", kind, name, file, shortLine(line))
}

// shortLine writes a body's text on one line, cut to 80 bytes.
func shortLine(line string) string {
	line = strings.Join(strings.Fields(line), " ")
	const maxLine = 80
	if len(line) > maxLine {
		line = line[:maxLine] + "..."
	}
	return line
}

// joinDir places p under dir unless it is absolute.
func joinDir(dir, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(dir, p)
}

// quoteWord writes a word for a shell to read back as itself.
func quoteWord(v string) string {
	const plain = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_./+,:@%=-"
	if v != "" && strings.Trim(v, plain) == "" {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// argText writes a word the command hands a build tool for the body that
// receives it: a literal as itself, quoted, and anything else as written.
func argText(w shWord) string {
	if v, ok := w.literal(); ok {
		return quoteWord(v)
	}
	return w.raw
}
