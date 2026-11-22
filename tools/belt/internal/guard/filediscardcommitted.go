package guard

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/cslindex"
)

// This file holds file-discard's trust in committed code (a user decision
// of 2026-11-22). Code read from a file git holds committed, a script file
// or a build target's Makefile, package.json or justfile, is trusted: a
// delete in it whose path belt cannot read passes, and a literal path in it
// still takes the ladder. A build target's body is trusted only when every
// file its build run reads is committed too: an include, an import, a
// .env. A file the same command writes or edits before it runs it is not
// trusted, and neither is one outside any work tree.

// unknown returns loss, a delete whose path belt cannot read, unless the
// code being judged comes from a committed file (committedCode), and so
// does every file its values come from (codeFrom), where it passes.
func (j *discardJudge) unknown(loss *fileLoss) *fileLoss {
	if j.codeFile == "" || loss == nil {
		return loss
	}
	const trust = ", and belt trusts such a delete only in code git holds committed"
	if !j.committedCode(j.codeFile) {
		loss.why += trust + ", which `" + filepath.Base(j.codeFile) + "` is not"
		return loss
	}
	for _, f := range j.codeFrom {
		if !j.committedCode(f) {
			loss.why += trust + ", and `" + filepath.Base(j.codeFile) + "` reads `" + filepath.Base(f) +
				"`, which is not"
			return loss
		}
	}
	return nil
}

// inFile judges code read from file (codeFile) with judge, then restores
// the file it was judging. "" marks code the command holds itself. Code
// read again from the file being judged, as a script a build target runs
// inline, keeps the files its values come from (codeFrom).
func (j *discardJudge) inFile(file string, judge func() *fileLoss) *fileLoss {
	from := []string{file}
	if file == j.codeFile {
		from = j.codeFrom
	}
	return j.inCode(file, from, judge)
}

// inCode judges code read from file with judge, its values coming from the
// files from too, as a build target's body takes them from every file its
// build run reads (an include, an import, a .env); then it restores what
// it was judging.
func (j *discardJudge) inCode(file string, from []string, judge func() *fileLoss) *fileLoss {
	prevFile, prevFrom := j.codeFile, j.codeFrom
	j.codeFile, j.codeFrom = file, from
	defer func() { j.codeFile, j.codeFrom = prevFile, prevFrom }()
	return judge()
}

// committedCode reports whether git holds the file at p as committed: it
// lies in a work tree, git tracks it, git status lists nothing for it
// (no edit, staged or not, and not untracked or ignored), and no earlier
// step of the command wrote or edited it (touched). What git answers is
// cached per path; the touches are checked on every call, since a step
// between two runs of the file may edit it.
func (j *discardJudge) committedCode(p string) bool {
	if _, touched := j.touched(p); touched || gitEnvRedirected() {
		return false
	}
	ok, seen := j.committed[p]
	if !seen {
		ok = j.gitHolds(p)
		j.committed[p] = ok
	}
	return ok
}

// gitHolds reports whether the file at p is tracked in its work tree and
// git status lists nothing for it.
func (j *discardJudge) gitHolds(p string) bool {
	real := physical(p)
	if spelled, ok := j.spelled(real); ok {
		real = spelled
	}
	top := cslindex.RepoRoot(real)
	if top == "" {
		return false
	}
	rel, err := filepath.Rel(top, real)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || rel == ".git" ||
		strings.HasPrefix(rel, ".git/") {
		return false
	}
	ctx, cancel := context.WithDeadline(context.Background(), j.deadline)
	defer cancel()
	status, err := j.g.status(ctx, top, []string{rel})
	if err != nil || len(status) > 0 {
		return false
	}
	tracked, err := j.g.tracked(ctx, top, []string{rel})
	return err == nil && len(tracked) > 0
}

// committedPackages reports whether git holds every package.json under the
// folder dir committed, for a run across workspaces: git status lists none
// changed, staged or untracked, and no step before this one may write one
// (touch.names). An ignored folder such as node_modules holds no
// workspace, so the ignored entries do not count.
func (j *discardJudge) committedPackages(dir string) bool {
	const name = "package.json"
	if gitEnvRedirected() || slices.ContainsFunc(j.touches, func(t touch) bool {
		return t.all || t.path == "" && strings.Contains(strings.ToLower(t.text), name) ||
			strings.EqualFold(path.Base(t.path), name) && holds(dir, t.path)
	}) {
		return false
	}
	real := physical(dir)
	top := cslindex.RepoRoot(real)
	rel, err := filepath.Rel(top, real)
	if top == "" || err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	ctx, cancel := context.WithDeadline(context.Background(), j.deadline)
	defer cancel()
	out, err := j.g.status(ctx, top, []string{rel})
	entries, readErr := readStatus(out)
	if err != nil || readErr != nil {
		return false
	}
	return !slices.ContainsFunc(entries, func(e statusEntry) bool {
		return e.code != "!!" && path.Base(e.path) == name
	})
}

// touch is a file an earlier step of the command may have written or
// edited: by its path, by its base name in the step's text, or any file at
// all (a checkout or a patch). text is the step's text, for the test of
// whether what it wrote names a delete. git marks a git command, which
// leaves what git holds. body, set for a write to path, returns what the
// step leaves there, unread when belt cannot read it.
type touch struct {
	path string
	text string
	all  bool
	git  bool
	body func() script
}

// names reports whether a touch may change the file at the clean path
// clean, whose base name in lower case is base. Names compare in any case,
// as a case-insensitive disk reads them. A text names the base only as a
// word of its own, so `Unix Makefiles` does not name a Makefile.
func (t touch) names(clean, base string) bool {
	return t.all || strings.EqualFold(t.path, clean) ||
		t.path == "" && namesWord(strings.ToLower(t.text), base)
}

// namesWord reports whether text holds word with no name byte (isNameByte,
// or -) right before or after it.
func namesWord(text, word string) bool {
	nameAt := func(i int) bool { return i >= 0 && i < len(text) && (isNameByte(text[i]) || text[i] == '-') }
	for from := 0; word != ""; {
		at := strings.Index(text[from:], word)
		if at < 0 {
			return false
		}
		at += from
		if !nameAt(at-1) && !nameAt(at+len(word)) {
			return true
		}
		from = at + 1
	}
	return false
}

// touched reports whether an earlier step may have written or edited the
// file at p, and the text of the steps that did.
func (j *discardJudge) touched(p string) (string, bool) {
	clean, base := path.Clean(p), strings.ToLower(path.Base(p))
	var texts []string
	for _, t := range j.touches {
		if t.names(clean, base) {
			texts = append(texts, t.text)
		}
	}
	return strings.Join(texts, "\n"), len(texts) > 0
}

// bodyState is what the steps of a command leave in a file a later step
// reads (newBody).
type bodyState int

const (
	bodyOnDisk  bodyState = iota // no step touches it: it holds what it holds on disk
	bodyWritten                  // a step writes text belt read
	bodyBlind                    // a step changes it in a way belt cannot read
	bodyGit                      // a git command may change it to what git holds
)

// newBody returns what the steps before this one leave in the file at p,
// by the last step that touches it: the text a write leaves there when
// belt read it, what git holds after a git command, and blind after any
// other change.
func (j *discardJudge) newBody(p string) (string, bodyState) {
	clean, base := path.Clean(p), strings.ToLower(path.Base(p))
	text, state := "", bodyOnDisk
	for _, t := range j.touches {
		switch {
		case !t.names(clean, base):
		case t.git:
			text, state = "", bodyGit
		case t.body != nil && strings.EqualFold(t.path, clean):
			text, state = "", bodyBlind
			if s := t.body(); s.ok {
				text, state = s.text, bodyWritten
			}
		default:
			text, state = "", bodyBlind
		}
	}
	return text, state
}

// writes reports whether an earlier step writes the file at p by its path,
// spelled as p spells it: a redirection, tee, cp or mv. Such a file is
// there when a later step looks for it, whether it was on disk or not.
func (j *discardJudge) writes(p string) bool {
	clean := path.Clean(p)
	return slices.ContainsFunc(j.touches, func(t touch) bool { return t.body != nil && t.path == clean })
}

// fileText returns what the file at p holds when a step reads it: what the
// steps before it leave there (newBody), else what it holds on disk
// (readBuildFile).
func (j *discardJudge) fileText(p string) (string, bool) {
	switch text, state := j.newBody(p); state {
	case bodyWritten:
		return text, true
	case bodyBlind:
		return "", false
	}
	return readBuildFile(p)
}

// noteWrites records the files a step writes with a redirection, before
// the step runs: a shell opens them first. What the step writes there is
// what the walk read of it once the step ran (writtenBody).
func (j *discardJudge) noteWrites(s walkStep) {
	for _, w := range s.writes {
		if p := resolveDir(s.cwd, w); p != "" {
			j.touches = append(j.touches, touch{path: p, text: strings.Join(s.tokens, " "), body: writtenBody(s, p)})
		}
	}
}

// writtenBody returns what the walk read of the text a step wrote to the
// file at p (walkStep.written), asked for when a later step reads the
// file: the walk records it only after the step.
func writtenBody(s walkStep, p string) func() script {
	return func() script {
		if s.written == nil {
			return script{}
		}
		sc, _ := s.written(p)
		return sc
	}
}

// noteEdits records the files a step may edit, for the steps after it: a
// git command that changes the work tree and a patch may change any file;
// tee writes the files it names, and a plain cp or mv the file it copies
// to (copyTouch); any other command but a plain reader (onlyReads) may
// change each file its words or its heredocs name. A step belt could not
// read may change what any word names.
func (j *discardJudge) noteEdits(s walkStep) {
	at := runAt(s.tokens)
	if at >= len(s.tokens) {
		return
	}
	text := strings.Join(s.tokens[at:], " ")
	switch name := path.Base(s.tokens[at]); {
	case s.unread || s.words == nil:
	case name == "git" || name == "patch":
		if name == "patch" || gitWritesWorkTree(s.tokens[at+1:]) {
			j.touches = append(j.touches, touch{text: text, all: true, git: name == "git"})
		}
		return
	case onlyReads(name, s.tokens[at+1:]):
		return
	case name == "tee":
		j.noteTee(s, at)
		return
	case copyMovers[name]:
		if t, ok := j.copyTouch(s, at); ok {
			j.touches = append(j.touches, t)
			return
		}
		if copiers[name] && len(s.tokens) > at+2 {
			text = s.tokens[len(s.tokens)-1]
		}
	case copiers[name] && len(s.tokens) > at+2:
		text = s.tokens[len(s.tokens)-1]
	}
	if s.docText != "" {
		text += "\n" + s.docText
	}
	j.touches = append(j.touches, touch{text: text})
}

// noteTee records the files a tee writes: each word past its options, with
// what the walk read of what tee writes there (writtenBody). A word belt
// cannot place touches what it names.
func (j *discardJudge) noteTee(s walkStep, at int) {
	for _, w := range s.tokens[at+1:] {
		switch p := resolveDir(s.cwd, w); {
		case strings.HasPrefix(w, "-"):
		case p != "":
			j.touches = append(j.touches, touch{path: p, text: strings.Join(s.tokens, " "), body: writtenBody(s, p)})
		default:
			j.touches = append(j.touches, touch{text: w})
		}
	}
}

// copyMovers are the commands copyTouch reads: cp and mv.
var copyMovers = wordSet("cp gcp mv gmv")

// copyTouch is the touch of a cp or mv of one file onto another, with what
// it leaves there: the source's text as the command leaves it (fileText).
// A destination that is a folder on disk takes the source's name. ok is
// false for any other shape: an option other than -f, -p or -v, other than
// two operands, a word belt cannot read, or a folder belt cannot place.
func (j *discardJudge) copyTouch(s walkStep, at int) (touch, bool) {
	if len(s.words) != len(s.tokens) || s.cwd == "" {
		return touch{}, false
	}
	var ops []string
	for _, w := range s.words[at+1:] {
		v, lit := w.literal()
		switch {
		case !lit || v == "" || v == "-" || v == "--":
			return touch{}, false
		case v[0] == '-' && strings.Trim(v[1:], "fpv") != "":
			return touch{}, false
		case v[0] != '-':
			ops = append(ops, v)
		}
	}
	if len(ops) != 2 {
		return touch{}, false
	}
	src, dst := joinDir(s.cwd, ops[0]), joinDir(s.cwd, ops[1])
	if fi, err := os.Stat(dst); err == nil && fi.IsDir() || strings.HasSuffix(ops[1], "/") {
		dst = filepath.Join(dst, filepath.Base(src))
	}
	text, ok := j.fileText(src)
	body := script{text: text, ok: ok}
	return touch{path: dst, text: strings.Join(s.tokens[at:], " "), body: func() script { return body }}, true
}

// copiers write only their last operand, the file or folder they copy to.
var copiers = wordSet("cp gcp install ginstall ln gln")

// plainReaders are commands that read the files they name and change none.
var plainReaders = wordSet("cat head tail less more bat grep egrep fgrep rg ag ack wc ls stat file " +
	"diff cmp md5 md5sum shasum sha1sum sha256sum test [ [[ echo printf jq yq which type " +
	"realpath readlink basename dirname du tree true false")

// onlyReads reports whether a command only reads the files it names: one
// of plainReaders, or a sed without -i.
func onlyReads(name string, args []string) bool {
	if name == "sed" || name == "gsed" {
		for _, a := range args {
			if strings.HasPrefix(a, "-i") || strings.HasPrefix(a, "--in-place") ||
				strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "i") {
				return false
			}
		}
		return true
	}
	return plainReaders[name]
}

// worktreeGitCommands are the git commands that may change files in the
// work tree.
var worktreeGitCommands = wordSet("checkout switch restore reset stash merge pull rebase " +
	"cherry-pick revert apply am mv rm clean worktree submodule sparse-checkout")

// gitWritesWorkTree reports whether git's words, past its own options, name a
// command that may change files in the work tree. Words belt cannot place
// count as one that may.
func gitWritesWorkTree(args []string) bool {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-C" || a == "-c" || a == "--git-dir" || a == "--work-tree":
			i++
		case strings.HasPrefix(a, "-"):
		default:
			return worktreeGitCommands[a]
		}
	}
	return false
}
