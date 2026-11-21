package guard

import (
	"context"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// FileDiscardID identifies the guard against shell deletes of work no commit
// holds.
const FileDiscardID = "file-discard"

// fileDiscardRule says when a delete passes.
const fileDiscardRule = "A delete passes only when every file it drops is inside a temp or cache folder, " +
	"missing or empty, ignored by git, or already in HEAD, its upstream or origin's default branch."

// FileDiscard denies a shell command that deletes files belt cannot prove
// are kept elsewhere: rm, unlink and shred, find -delete and -exec, rsync
// --delete, xargs, GNU parallel and fd -x. It is git-discard's rule for the
// commands git does not run. A delete passes when every path it drops is
// inside a temp or cache folder, missing or empty, ignored in a git work
// tree, or the same as HEAD, its upstream or origin's default branch there. rm-temp runs
// first, so an rm whose targets are all temp keeps its toss-bin --temp-only
// handover. `mode: soft` warns instead of denying.
type FileDiscard struct {
	cfg config.Config
	// home returns the home directory, "" when unknown. Injectable for tests.
	home func() string
	// lookPath finds toss-bin, which the deny offers only when it is on
	// PATH. Injectable for tests.
	lookPath func(file string) (string, error)
	// status returns `git status --porcelain=v1 -z` output for the
	// pathspecs in the work tree at top. Injectable for tests.
	status func(ctx context.Context, top string, pathspecs []string) ([]byte, error)
	// tracked returns `git ls-files -s` output for the pathspecs in the work
	// tree at top, paths quoted as the repository says. Injectable for tests.
	tracked func(ctx context.Context, top string, pathspecs []string) ([]byte, error)
	// prove returns nil when every tracked path (relative to top) holds what
	// HEAD, its upstream or origin's default branch holds, else the first
	// loss. Injectable for tests.
	prove func(ctx context.Context, top string, paths []string) error
	// worktrees lists the work trees of the repository at top, linked ones
	// included (git worktree list). Injectable for tests.
	worktrees func(ctx context.Context, top string) ([]string, error)
	// keeps returns nil when the repository a delete takes whole, one inside
	// an ignored or untracked folder, loses nothing: a clean tree, and every
	// commit of HEAD, its branches, its tags and its stash on a remote
	// branch. Injectable for tests.
	keeps func(ctx context.Context, repo string) error
	// emit sends the soft-mode warning to the events service. Injectable for
	// tests.
	emit emitFunc
	// budget bounds every git call, folder walk and file hash of one Check
	// together, and maxPaths caps the tracked paths it proves. globDirs
	// caps the folders its globs read, and scanDirs the folders it reads
	// looking for a repository inside an ignored folder.
	budget             time.Duration
	maxPaths           int
	globDirs, scanDirs int
}

// NewFileDiscard builds the guard with the real home, PATH, git and events
// backends.
func NewFileDiscard(cfg config.Config) *FileDiscard {
	return &FileDiscard{
		cfg:       cfg,
		home:      homeDir,
		lookPath:  exec.LookPath,
		status:    discardStatus,
		tracked:   listTracked,
		prove:     proveCommitted,
		worktrees: listWorktrees,
		keeps:     repoKeepsAll,
		emit:      notify.EmitEventSync,
		budget:    proofBudget,
		maxPaths:  maxProofPaths,
		globDirs:  maxGlobDirs,
		scanDirs:  maxScanDirs,
	}
}

func (g *FileDiscard) ID() string    { return FileDiscardID }
func (g *FileDiscard) Event() string { return EventBash }

func (g *FileDiscard) noteWarnings(note func()) { g.emit = noting(g.emit, note) }

// Check walks every command the shell runs (walkCommands) and judges each
// path an rm, unlink or shred drops. The cheap checks run during the walk;
// the git checks run after it, one git status per work tree.
func (g *FileDiscard) Check(in Input) *Denial {
	j := newDiscardJudge(g)
	var loss *fileLoss
	walkCommands(in.Command, in.Cwd, func(s walkStep) {
		if loss == nil {
			loss = j.step(s)
		}
	})
	if loss == nil {
		loss = j.inRepos()
	}
	if loss == nil || g.warnSoft(loss) {
		return nil
	}
	return Reasonf(FileDiscardID, "%s", g.reason(loss))
}

// warnSoft sends the soft-mode warning for loss and reports whether soft
// mode is on, so the caller allows instead of denying.
func (g *FileDiscard) warnSoft(loss *fileLoss) bool {
	if !g.cfg.Guards[FileDiscardID].Soft() {
		return false
	}
	g.emit(
		"belt",
		"warn",
		"file-discard (soft)",
		"belt["+FileDiscardID+"]: "+loss.why+". Allowed (soft mode).",
		map[string]string{"guard": FileDiscardID},
	)
	return true
}

// reason is what the model reads: the first loss, then the lossless form,
// then git-discard's tail. A delete of the root or the home directory is
// the user's alone.
func (g *FileDiscard) reason(loss *fileLoss) string {
	if loss.userOnly {
		return loss.why + ". " + gitGoAhead + ": only the user may run it, with `! <command>`."
	}
	parts := []string{loss.why + ".", fileDiscardRule}
	switch c := loss.call; {
	case loss.instead != "":
		parts = append(parts, loss.instead)
	case c == nil:
	case c.keep != "":
		parts = append(parts, c.keep)
	case c.toss != "" && g.hasTossBin():
		parts = append(parts, "To keep a copy, "+c.toss+".")
	case len(c.targets) > 0 && g.hasTossBin():
		parts = append(parts, "To keep a copy, move the paths to the trash instead: `"+
			TossBin+" --safe-mode -r "+strings.Join(c.targets, " ")+"`.")
	}
	return strings.Join(append(parts, gitDiscardAsk), " ")
}

func (g *FileDiscard) hasTossBin() bool {
	_, err := g.lookPath(TossBin)
	return err == nil
}

// lossCall is one command that deletes: an rm, unlink or shred with its
// operands as written (targets), for the toss-bin form, or a find, xargs
// or rsync. toss is the toss-bin form for a call whose operands toss-bin
// cannot take as they are, and keep a lossless form that needs no
// toss-bin, offered first.
type lossCall struct {
	targets []string
	toss    string
	keep    string
}

// fileLoss is the first thing a command may lose: why, the call that loses
// it (nil when belt could not read the text), and the lossless form to offer
// in place of the toss-bin one. userOnly marks a delete of the root or the
// home directory, which only the user may run.
type fileLoss struct {
	why      string
	call     *lossCall
	instead  string
	userOnly bool
}

// lossOperand is one path a loss command names: the word as written,
// whether the command drops the whole tree there, and whether it writes
// through a symlink in the last part instead of removing the link. filter
// narrows a tree to the paths a find or a feeder hands the command, nil
// for all of them.
type lossOperand struct {
	word    shWord
	tree    bool
	follows bool
	filter  *pathFilter
}

// treeDrop is one path inside a work tree that only git can judge, relative
// to the tree's top, with the call that drops it. dir marks a folder the
// call reaches into, which may hold a repository git status does not list,
// and filter the paths in it the call drops (nil for all). prefix is the
// path from the feeder's root to this tree's top, for a repository found
// inside the drop of another (nestedTop): the paths the feeder prints here
// start with it.
type treeDrop struct {
	rel    string
	call   *lossCall
	dir    bool
	filter *pathFilter
	prefix string
}

// repoDrops are the drops inside one work tree, judged by one git status.
// judged marks a list inRepos has judged, which takes no more drops.
type repoDrops struct {
	top    string
	drops  []treeDrop
	judged bool
}

// discardJudge walks one command's steps for file-discard: the variables
// mktemp set so far (rmScan), the home directory as given and as it lies on
// disk, and the drops left for git, per work tree in the order met. The
// call's deadline bounds its folder walks and git calls together;
// spellings remembers the paths spelled (spelled), walked the folders
// searched (scanHidden), and globDirs and scanned count the folders read
// so far. covered holds the ids of the steps whose -c script the consumer
// judge read with the consumer's input (consumedScript).
type discardJudge struct {
	g         *FileDiscard
	scan      rmScan
	home      string
	realHome  string
	repos     []*repoDrops
	deadline  time.Time
	spellings map[string]string
	walked    map[walkKey]bool
	covered   map[int]bool
	globDirs  int
	scanned   int
}

func newDiscardJudge(g *FileDiscard) *discardJudge {
	home := g.home()
	j := &discardJudge{
		g: g, scan: rmScan{home: home, vars: map[string]tempVar{}}, home: home,
		deadline: time.Now().Add(g.budget), spellings: map[string]string{}, walked: map[walkKey]bool{},
		covered: map[int]bool{},
	}
	if home != "" {
		j.realHome = physical(home)
	}
	return j
}

// step judges one command and returns its first loss, or nil. A prose step
// runs nothing, and a step of a -c script the consumer judge read with the
// consumer's input (covered) is judged already. A step belt could not read
// (walkStep.unread, or one from the token walk with no words) is a loss
// when it names a loss command anywhere, and so is code it runs that belt
// could not read, when the text that builds it names one (unreadLoss). git
// steps are git-discard's.
func (j *discardJudge) step(s walkStep) *fileLoss {
	switch {
	case s.prose, j.covered[s.scriptOf]:
		return nil
	case s.unread || s.words == nil:
		if namesLossCommand(s.tokens) {
			return &fileLoss{why: "belt could not read the shell text around `" +
				stepText(s.tokens) + "`, so it cannot tell what it deletes"}
		}
		return nil
	}
	if loss := unreadLoss(s); loss != nil {
		return loss
	}
	words := make([]shWord, len(s.words))
	for i, w := range s.words {
		words[i] = w.shWord
	}
	words = skipReserved(words)
	if j.scan.assign(words) {
		return nil
	}
	name, args, ok := lossCommand(skipAssignments(words))
	if !ok {
		return nil
	}
	switch name {
	case "find":
		return j.find(args, s)
	case "rsync":
		return j.rsync(args, s)
	case "xargs":
		return j.xargs(args, s)
	case "parallel":
		return j.parallel(words[:len(words)-len(args)-1], args, s)
	case "fd":
		return j.fd(args, s)
	}
	return j.lossCall(&lossCall{}, name, args, s, nil)
}

// lossCall judges one rm, unlink or shred: each operand, where an operand
// that is fd's mark stands for the paths a feeder hands it, and one that is
// a whole $( ) or backtick substitution for the paths that feeder prints.
func (j *discardJudge) lossCall(call *lossCall, name string, args []shWord, s walkStep, fd *pathFeed) *fileLoss {
	ops := lossOperands(name, args)
	for _, op := range ops {
		if fd == nil || op.word.raw != feedMark {
			call.targets = append(call.targets, op.word.raw)
		}
	}
	for _, op := range ops {
		if loss := j.callOperand(call, op, s, fd); loss != nil {
			return loss
		}
	}
	return nil
}

// callOperand judges one operand of a loss call (lossCall).
func (j *discardJudge) callOperand(call *lossCall, op lossOperand, s walkStep, fd *pathFeed) *fileLoss {
	v, lit := op.word.literal()
	switch {
	case fd != nil && lit && v == fd.mark:
		return j.fed(call, op, fd, s)
	case fd != nil && fd.mark != feedMark && strings.Contains(op.word.raw, fd.mark):
		return unresolvable(call, op.word)
	case fd != nil && fd.inMatchDir && (!lit || !path.IsAbs(v)):
		return &fileLoss{call: call, why: "`" + op.word.raw + "` is relative to the folder of " +
			"each match, so belt cannot tell which path it names"}
	}
	if body, ok := substBody(op.word); ok {
		return j.fed(call, op, substFeed(op.word, body), s)
	}
	return j.operand(call, op, s)
}

// lossCommands are the commands whose operands file-discard judges as
// deletes, by the name each runs as: GNU coreutils from Homebrew installs
// them with a g in front.
var lossCommands = map[string]string{
	"rm": "rm", "unlink": "unlink", "shred": "shred",
	"grm": "rm", "gunlink": "unlink", "gshred": "shred",
}

// shapeCommands are the commands whose own syntax says what they delete:
// find's -delete and -exec, rsync's --delete and the command xargs, GNU
// parallel or fd's -x and -X runs. GNU findutils from Homebrew installs
// find and xargs with a g in front, and Debian installs fd as fdfind.
var shapeCommands = map[string]string{
	"find": "find", "gfind": "find", "rsync": "rsync", "xargs": "xargs", "gxargs": "xargs",
	"parallel": "parallel", "fd": "fd", "fdfind": "fd",
}

// discardName is the command a word runs as for file-discard, "" for one
// it does not judge.
func discardName(w shWord) string {
	n := runName(w)
	if name, ok := lossCommands[n]; ok {
		return name
	}
	return shapeCommands[n]
}

// multicallTools run the applet their first word names (`busybox rm`), so
// file-discard reads past them as it reads past a wrapperOptions wrapper.
var multicallTools = wordSet("busybox toybox")

// runsNext reports whether a command name runs a command after it: a
// wrapper (isWrapper) or a multicall tool.
func runsNext(name string) bool {
	return isWrapper(name) || multicallTools[name]
}

// runName is the command a word runs: its last path part, "" when the word
// holds an expansion.
func runName(w shWord) string {
	v, ok := w.literal()
	if !ok || v == "" {
		return ""
	}
	return path.Base(v)
}

// lossCommand finds the loss command a step runs: its command word, or
// behind a wrapper the first loss-command word after it (`sudo -u x rm`). A
// git word first means git runs it, which git-discard judges.
func lossCommand(words []shWord) (name string, args []shWord, ok bool) {
	if len(words) == 0 {
		return "", nil, false
	}
	if name := discardName(words[0]); name != "" {
		return name, words[1:], true
	}
	if !runsNext(runName(words[0])) {
		return "", nil, false
	}
	for i, w := range words[1:] {
		if runName(w) == "git" {
			return "", nil, false
		}
		if name := discardName(w); name != "" {
			return name, words[i+2:], true
		}
	}
	return "", nil, false
}

// isRmFamily reports whether a loss command name is rm, unlink or shred.
func isRmFamily(name string) bool {
	return name == "rm" || name == "unlink" || name == "shred"
}

// namesLossCommand reports whether a token reads as a loss command, find's
// -delete or one of rsync's delete options anywhere in a step belt could
// not read.
func namesLossCommand(tokens []string) bool {
	for _, tok := range tokens {
		if _, ok := lossCommands[path.Base(tok)]; ok || tok == "-delete" ||
			strings.HasPrefix(tok, "--del") || strings.HasPrefix(tok, "--remove-s") {
			return true
		}
	}
	return false
}

// lossOperands reads a loss command's arguments into the paths it drops.
func lossOperands(name string, args []shWord) []lossOperand {
	switch name {
	case "rm":
		return rmOperands(args)
	case "shred":
		return shredOperands(args)
	}
	return unlinkOperands(args)
}

// rmOperands reads rm's arguments with the rm option list the script guard
// reads (rmLongOption, rmShortOptions). An option counts after an operand
// too, as GNU rm reads it, and `--` ends them. -r or -R makes every operand
// a tree; so does a long option rm does not know, since belt cannot tell.
func rmOperands(args []shWord) []lossOperand {
	tree, endOpts := false, false
	var words []shWord
	for _, w := range args {
		v, lit := w.literal()
		switch {
		case endOpts || !lit || len(v) < 2 || v[0] != '-':
			words = append(words, w)
		case v == "--":
			endOpts = true
		case strings.HasPrefix(v, "--"):
			name, known := rmLongOption(v[2:])
			tree = tree || !known || name == "recursive"
		default:
			for _, c := range v[1:] {
				tree = tree || rmShortOptions[c] == "recursive"
			}
		}
	}
	ops := make([]lossOperand, len(words))
	for i, w := range words {
		ops[i] = lossOperand{word: w, tree: tree}
	}
	return ops
}

// unlinkOperands reads unlink's arguments: every word is a file, after an
// optional `--`.
func unlinkOperands(args []shWord) []lossOperand {
	if len(args) > 0 {
		if v, ok := args[0].literal(); ok && v == "--" {
			args = args[1:]
		}
	}
	ops := make([]lossOperand, len(args))
	for i, w := range args {
		ops[i] = lossOperand{word: w}
	}
	return ops
}

// shredLongOptions are GNU shred's long options, and shredValueOptions the
// ones that take a value; shredValueShorts are the short ones that do.
var (
	shredLongOptions = []string{
		"exact", "force", "help", "iterations", "random-source", "remove", "size",
		"verbose", "version", "zero",
	}
	shredValueOptions = wordSet("iterations random-source size")
)

const shredValueShorts = "ns"

// shredOperands reads shred's arguments: every operand is a file whose
// content shred overwrites through a symlink, with -u or without. A value
// option's value is no operand.
func shredOperands(args []shWord) []lossOperand {
	var ops []lossOperand
	endOpts, value := false, false
	for _, w := range args {
		v, lit := w.literal()
		switch {
		case value:
			value = false
		case endOpts || !lit || len(v) < 2 || v[0] != '-':
			ops = append(ops, lossOperand{word: w, follows: true})
		case v == "--":
			endOpts = true
		case strings.HasPrefix(v, "--"):
			value = !strings.Contains(v, "=") && shredTakesValue(v[2:])
		default:
			i := strings.IndexAny(v[1:], shredValueShorts)
			value = i >= 0 && i == len(v)-2
		}
	}
	return ops
}

// shredTakesValue reports whether a long option, by its exact name or a
// unique prefix, takes the next word as its value.
func shredTakesValue(name string) bool {
	match := ""
	for _, o := range shredLongOptions {
		if o == name {
			return shredValueOptions[o]
		}
		if strings.HasPrefix(o, name) {
			if match != "" {
				return false
			}
			match = o
		}
	}
	return shredValueOptions[match]
}

// unresolvable is the loss for an operand belt cannot place on disk.
func unresolvable(call *lossCall, w shWord) *fileLoss {
	return &fileLoss{call: call, why: "belt cannot tell which path `" + w.raw + "` names"}
}
