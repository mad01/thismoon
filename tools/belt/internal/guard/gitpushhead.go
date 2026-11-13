package guard

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/gitloc"
)

// pushHeads is what the git checkouts and switches before a push, in its
// directory, leave as its current branch (headTracker).
type pushHeads struct {
	// switched are the branches the checkouts and switches in the push's
	// directory may have made current, in order.
	switched []pushHead
	// sure marks a push that runs only after the last of them succeeded,
	// with nothing around them that may move HEAD another way, so the
	// branch git reads before the command no longer counts.
	sure bool
	// verify is the branch a sure git checkout <name> needs to exist for git
	// to read <name> as a branch and not as a path, "" when nothing needs it.
	verify string
	// why says what kept the last switch from being sure, for the denial to
	// name (unsureSwitchDenial).
	why string
}

// pushHead is one branch a push may find checked out: the one git reads in
// its directory (fromRepo), or one a switch moved to, with name "" when the
// guard could not read where.
type pushHead struct {
	name     string
	fromRepo bool
	// move is the checkout or switch that moved to it, as written, for the
	// denial to name.
	move string
}

// candidates returns the branches a push may find checked out, the latest
// switch first, so a denial names the switch nearest the push.
func (h pushHeads) candidates() []pushHead {
	n := len(h.switched)
	if h.sure && n > 0 {
		return h.switched[n-1:]
	}
	heads := slices.Clone(h.switched)
	slices.Reverse(heads)
	return append(heads, pushHead{fromRepo: true})
}

// verified keeps a sure git checkout <name> sure only when hasBranch finds
// the local branch <name>; git then reads <name> as that branch.
func (h pushHeads) verified(hasBranch func(string) bool) pushHeads {
	if h.sure && h.verify != "" && !hasBranch(h.verify) {
		h.sure = false
		h.why = fmt.Sprintf("belt found no local branch %s, so git may read it as a path", h.verify)
	}
	return h
}

// headTracker follows the git checkouts and switches of one command in the
// order the walk meets them. A switch moves the repository's HEAD, not the
// shell, so one in a subshell, a substitution or a -c script counts as much
// as a plain one. It counts only for a push in the same known directory
// (knownDir): a switch in another directory may be in another repository, so
// it adds nothing there, and only stops a switch before it from being sure.
type headTracker struct {
	moves []headMove
	// steady marks that every step since the last move leaves HEAD alone
	// (keepsHead); breaker is the first one that may not, as written.
	steady  bool
	breaker string
	// changed are the moves and the steps keepsHead does not vouch for, as
	// written, so a push can tell whether one came after it (gitCommandsAt).
	changed []string
	// defines is the first step that defines a shell function, as written,
	// "" when none does (settle).
	defines string
}

// headMove is one checkout or switch the walk met.
type headMove struct {
	dir  string // knownDir, "" when unknown
	head pushHead
	// chain is the && run (walkStep.chain) a later push must share for the
	// move to be sure, 0 when it cannot be.
	chain  int
	verify string // pushHeads.verify
}

// see takes one step of the walk. A comment or a heredoc body no shell runs
// moves nothing.
func (t *headTracker) see(s walkStep) {
	if s.prose {
		return
	}
	c, isGit := segmentGitCommand(s.tokens, s.dir)
	name, kind := "", noMove
	if isGit {
		name, kind = branchMove(c)
	}
	step := stepText(s.tokens)
	if s.defines && t.defines == "" {
		t.defines = step
	}
	switch {
	case kind != noMove:
		t.moves = append(t.moves, newHeadMove(s, c, name, kind))
		t.steady, t.breaker = true, ""
	case keepsHead(s, c, isGit):
		return
	case t.steady:
		t.steady, t.breaker = false, step
	}
	t.changed = append(t.changed, step)
}

// newHeadMove records one checkout or switch. It can be sure only when the
// command plainly runs git (runsGit), is not itself after || or a pipe, and
// && follows it. A git checkout <name> is sure only once the push's repository
// shows <name> is a branch (pushHeads.verify), so one the guard cannot read
// never is.
func newHeadMove(s walkStep, c gitInvocation, name string, kind moveKind) headMove {
	m := headMove{
		dir:  knownDir(s, c),
		head: pushHead{name: name, move: stepText(append([]string{"git", c.sub}, c.args...))},
	}
	if runsGit(s.tokens) && s.chain != 0 && s.after == jointAnd && sequential(s.before) {
		m.chain = s.chain
	}
	if kind == mayMove {
		m.verify = name
		if name == "" {
			m.chain = 0
		}
	}
	return m
}

// at returns the heads a git command c at step s may find. Only the moves in
// its own known directory count. The push is sure when the last move of the
// command is one of them, every step since leaves HEAD alone, and && alone
// joins that move to the push.
func (t *headTracker) at(s walkStep, c gitInvocation) pushHeads {
	dir := knownDir(s, c)
	if dir == "" || len(t.moves) == 0 {
		return pushHeads{}
	}
	var h pushHeads
	for _, m := range t.moves {
		if m.dir == dir {
			h.switched = append(h.switched, m.head)
		}
	}
	last := t.moves[len(t.moves)-1]
	h.verify = last.verify
	switch {
	case last.dir != dir:
		h.why = fmt.Sprintf("`%s` in another directory comes after it", last.head.move)
	case !t.steady:
		h.why = fmt.Sprintf(
			"`%s` runs between the switch and the push, and %s",
			t.breaker,
			keepsHeadRule,
		)
	case last.chain == 0 || last.chain != s.chain:
		h.why = "the push does not run only after the switch succeeded: the switch must run as plain " +
			"git, without !, not after || or in a pipeline, and && alone must join it to the push"
	default:
		h.sure = true
	}
	return h
}

// settle returns h, the heads of a git command the walk met when changed held
// seen steps, with what the whole command shows: a function definition
// anywhere in it may stand in for git or for a step keepsHead vouches for, and
// a step after the command may move the branch (afterPush). Either leaves the
// command unsure.
func (t *headTracker) settle(h pushHeads, seen int) pushHeads {
	switch {
	case !h.sure:
	case t.defines != "":
		h.sure = false
		h.why = fmt.Sprintf(
			"`%s` defines a shell function in the same command, which may stand in for git "+
				"or any other command",
			t.defines,
		)
	case seen < len(t.changed):
		h.afterPush(t.changed[seen])
	}
	return h
}

// afterPush makes h unsure because step, which keepsHead does not vouch for,
// comes after the push in the same command and may move the branch. After a &
// or in a pipeline it runs beside the push; after && it runs once the push is
// done, but one list holds for every step after the push.
func (h *pushHeads) afterPush(step string) {
	h.sure = false
	h.why = fmt.Sprintf(
		"`%s` comes after the push in the same command and may move the branch, and %s; "+
			"run that step in a command of its own",
		step,
		keepsHeadRule,
	)
}

// keepsHeadRule names the steps keepsHead vouches for, for the denials.
const keepsHeadRule = "belt counts only git add, commit, fetch, pull, merge, status, log, diff " +
	"and rebase with at most one upstream (with no -c, no --output and no path in .git), a read " +
	"of the current branch and read-only commands as leaving HEAD alone"

// stepText returns a step's words as one line for a denial, cut short.
func stepText(tokens []string) string {
	const maxStepText = 80
	text := strings.Join(tokens, " ")
	if len(text) > maxStepText {
		return text[:maxStepText] + "..."
	}
	return text
}

// knownDir returns the directory a git command works on when both the walk
// and git can tell it: the same -C target or the same cd state give the same
// string. It is "" when the shell's directory is unknown or git may use
// another repository there (walkStep.dir), even when a -C names a path.
func knownDir(s walkStep, c gitInvocation) string {
	if s.dir == "" {
		return ""
	}
	return c.dir
}

// runsGit reports whether a command's words run the git in them: nothing but
// NAME=value assignments and wrapperCommands words (timeout with its
// duration) in front of the git word. So git is not the argument of another
// command (echo git, : git, xargs git), and no ! in front turns a failed git
// into success for the && after it.
func runsGit(tokens []string) bool {
	pre, ok := beforeGit(tokens)
	for i := 0; ok && i < len(pre); i++ {
		switch name := filepath.Base(pre[i]); {
		case isAssignment(pre[i]), wrapperCommands[name]:
		case wrappersWithArg[name] && i+1 < len(pre):
			i++
		default:
			return false
		}
	}
	return ok
}

// headKeepers are the git subcommands that leave HEAD on its branch, past
// rebase, which rebaseKeepsHead reads more closely.
var headKeepers = wordSet("add commit fetch pull merge status log diff")

// keepsHead reports whether a step that is no checkout or switch leaves
// HEAD on its branch. That one list decides for the steps between a switch
// and the push and for those after the push: a plainly run git command
// gitKeepsHead vouches for, a whole currentBranchCommands read, or a
// readOnlyCommands command. Anything else may move HEAD: a git alias, git
// branch -m, a script, a test run. So may a step that writes into .git
// (retargetsPush).
func keepsHead(s walkStep, c gitInvocation, isGit bool) bool {
	switch {
	case retargetsPush(s):
		return false
	case currentBranchCommands[strings.Join(s.tokens, " ")]:
		return true
	case isGit && runsGit(s.tokens):
		return gitKeepsHead(s, c)
	}
	words := stripWrappers(s.tokens)
	return len(words) > 0 && readOnlyCommand(words)
}

// gitKeepsHead reports whether a plainly run git command c at step s leaves
// HEAD on its branch: a headKeepers subcommand, or a git rebase
// rebaseKeepsHead vouches for. Not with a -c override of its own
// (gitInvocation.configured), which may change what the subcommand runs, and
// not with a word that may point it into .git, where HEAD lives: a path there
// (gitConfigFile) or an --output option, which git log and git diff write to.
func gitKeepsHead(s walkStep, c gitInvocation) bool {
	intoGitDir := func(word string) bool { return outputOption(word) || gitConfigFile(word, s.dir) }
	switch {
	case c.configured || slices.ContainsFunc(s.tokens, intoGitDir):
		return false
	case c.sub == "rebase":
		return rebaseKeepsHead(c.args)
	}
	return headKeepers[c.sub]
}

// outputOption reports whether a word is git's --output option, with or
// without a value, or a prefix of it git may accept for it.
func outputOption(word string) bool {
	opt, _, _ := strings.Cut(word, "=")
	return strings.HasPrefix(opt, "--output") ||
		(len(opt) > 3 && strings.HasPrefix("--output", opt))
}

// gitHasBranch reports whether the repository at dir has the local branch
// name. It is the one git call the guard makes for a switch, bounded by
// branchLookupTimeout; an error or a timeout reads as no branch.
// for-each-ref matches its pattern as a path prefix, so the one line must be
// the ref itself.
func gitHasBranch(dir, name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), branchLookupTimeout)
	defer cancel()
	ref := "refs/heads/" + name
	return gitloc.Output(ctx, dir, "for-each-ref", "--format=%(refname)", ref) == ref
}

// branchLookupTimeout bounds gitHasBranch.
const branchLookupTimeout = 2 * time.Second

// rebaseMovers are the git rebase options that may leave another branch
// checked out: --root reads its one word as the branch to switch to, --exec
// and an interactive todo may run a checkout, and the others pick up a
// rebase begun on another branch.
var rebaseMovers = []string{
	"--root", "--exec", "--interactive", "--edit-todo", "--continue", "--skip", "--abort", "--quit",
}

// rebaseKeepsHead reports whether git rebase rebases the branch it starts
// on: at most one word that is not an option, the upstream, since a second
// one names a branch git switches to first, and no rebaseMovers option, -x
// or -i, alone, bundled, or as a prefix git accepts.
func rebaseKeepsHead(args []string) bool {
	words := 0
	for _, arg := range args {
		opt, _, _ := strings.Cut(arg, "=")
		switch {
		case !strings.HasPrefix(arg, "-"):
			words++
		case !strings.HasPrefix(arg, "--"):
			if strings.ContainsAny(arg, "xi") {
				return false
			}
		case len(opt) > 3 && slices.ContainsFunc(rebaseMovers, func(m string) bool {
			return strings.HasPrefix(m, opt)
		}):
			return false
		}
	}
	return words <= 1
}

// moveKind is what a git command does to the current branch.
type moveKind int

const (
	noMove      moveKind = iota // no branch change: a file checkout, or no checkout at all
	mayMove                     // git checkout <name>: <name> may be a path, which leaves the branch
	movesSurely                 // a switch, or a checkout that creates, tracks or detaches
)

// branchMove returns the branch a git checkout or git switch moves to, ""
// when the guard cannot read it, and how sure the move is.
func branchMove(c gitInvocation) (string, moveKind) {
	switch c.sub {
	case "switch":
		return switchMove(c.args)
	case "checkout":
		return checkoutMove(c.args)
	}
	return "", noMove
}

// switchMove reads git switch. It takes no paths, so every target is a
// branch; a -- or an option the guard does not know leaves it unreadable.
func switchMove(args []string) (string, moveKind) {
	m := readMoveArgs(args, switchFlags)
	switch {
	case m.unknown || m.dashdash:
		return "", movesSurely
	case m.created:
		return moveName(m.create), movesSurely
	case m.detach:
		return "", movesSurely
	case len(m.positional) == 0:
		return "", noMove
	case m.track:
		return trackedName(m.positional[0]), movesSurely
	}
	return moveName(m.positional[0]), movesSurely
}

// checkoutMove reads git checkout. A -- , a second operand, -p or
// --pathspec-from-file put it in path mode, which leaves the branch where it
// is. One operand may name a branch or a path, so the move is only maybe.
func checkoutMove(args []string) (string, moveKind) {
	m := readMoveArgs(args, checkoutFlags)
	switch {
	case m.unknown:
		return "", movesSurely
	case m.created:
		return moveName(m.create), movesSurely
	case m.paths || m.dashdash || (len(m.positional) != 1 && !m.detach):
		return "", noMove
	case m.detach:
		return "", movesSurely
	case m.track:
		return trackedName(m.positional[0]), movesSurely
	}
	return moveName(m.positional[0]), mayMove
}

// moveFlags are the options of git checkout or git switch.
type moveFlags struct {
	create map[string]bool // their value names a branch the command creates and moves to
	value  map[string]bool // other options that take a value
	paths  map[string]bool // put the command in path mode
	detach map[string]bool // detach HEAD
	plain  map[string]bool // other options without a value
}

var switchFlags = moveFlags{
	create: wordSet("-c -C --create --force-create --orphan"),
	value:  wordSet("--conflict"),
	paths:  map[string]bool{},
	detach: wordSet("-d --detach"),
	plain: wordSet(`
		-f --force --discard-changes -m --merge -q --quiet --progress --no-progress
		-t --track --no-track --guess --no-guess --ignore-other-worktrees
		--recurse-submodules --no-recurse-submodules
	`),
}

var checkoutFlags = moveFlags{
	create: wordSet("-b -B --orphan"),
	value:  wordSet("--conflict"),
	paths:  wordSet("-p --patch --pathspec-from-file"),
	detach: wordSet("--detach"),
	plain: wordSet(`
		-q --quiet --progress --no-progress -f --force --ours -2 --theirs -3 -m --merge
		-t --track --no-track --guess --no-guess -l --ignore-skip-worktree-bits
		--ignore-other-worktrees --overwrite-ignore --no-overwrite-ignore
		--recurse-submodules --no-recurse-submodules --overlay --no-overlay --pathspec-file-nul
	`),
}

// moveArgs are the arguments of a git checkout or git switch, read.
type moveArgs struct {
	create                                  string // the value of a create option
	positional                              []string
	created, detach, track, paths, dashdash bool
	// unknown marks an option the guard does not know, a prefix of a long
	// one or a bundle with a letter it does not know included, which may
	// create or move.
	unknown bool
}

// readMoveArgs reads args with flags. Reading stops at -- and at a path
// mode option, since nothing after them moves the branch.
func readMoveArgs(args []string, flags moveFlags) moveArgs {
	var m moveArgs
	for i := 0; i < len(args) && !m.paths; i++ {
		switch arg := args[i]; {
		case arg == "--":
			m.dashdash = true
			return m
		case arg == "-" || !strings.HasPrefix(arg, "-"):
			m.positional = append(m.positional, arg)
		case strings.HasPrefix(arg, "--"):
			i = m.readLong(args, i, flags)
		default:
			i = m.readShort(args, i, flags)
		}
	}
	return m
}

// readLong reads the long option at args[i] and returns the index of the
// last argument it took.
func (m *moveArgs) readLong(args []string, i int, flags moveFlags) int {
	opt, value, eq := strings.Cut(args[i], "=")
	switch {
	case flags.paths[opt]:
		m.paths = true
	case flags.create[opt] || flags.value[opt]:
		if !eq && i+1 < len(args) {
			i++
			value = args[i]
		}
		if flags.create[opt] {
			m.create, m.created = value, true
		}
	case flags.detach[opt]:
		m.detach = true
	case flags.plain[opt]:
		m.track = m.track || opt == "--track"
	default:
		m.unknown = true
	}
	return i
}

// readShort reads the short options at args[i], one or a bundle (-qc), as
// git's option parser does: an option that takes a value takes the rest of
// the bundle, or the next argument when nothing is left. -t takes an
// optional value only from the rest of the bundle, so it reads only last.
// A letter the guard does not know leaves the bundle unknown.
func (m *moveArgs) readShort(args []string, i int, flags moveFlags) int {
	letters := args[i][1:]
	for j := range len(letters) {
		opt, rest := "-"+letters[j:j+1], letters[j+1:]
		switch {
		case flags.paths[opt]:
			m.paths = true
			return i
		case flags.create[opt] || flags.value[opt]:
			if rest == "" && i+1 < len(args) {
				i++
				rest = args[i]
			}
			if flags.create[opt] {
				m.create, m.created = rest, true
			}
			return i
		case flags.detach[opt]:
			m.detach = true
		case flags.plain[opt] && (opt != "-t" || rest == ""):
			m.track = m.track || opt == "-t"
		default:
			m.unknown = true
			return i
		}
	}
	return i
}

// moveName returns the branch a checkout or switch target names, as git
// reads it (destBranch), or "" when the guard cannot read it: a word only a
// shell can expand, - for the previous branch, or a @{-N} form.
func moveName(word string) string {
	if word == "" || word == "-" || !plainRefspec(word) {
		return ""
	}
	return destBranch(word)
}

// trackedName returns the branch --track creates from a remote-tracking
// branch: the name after the remote, as git derives it (origin/main is
// main), or "" when the guard cannot read it.
func trackedName(word string) string {
	name := strings.TrimPrefix(strings.TrimPrefix(moveName(word), "refs/"), "remotes/")
	_, branch, ok := strings.Cut(name, "/")
	if !ok {
		return ""
	}
	return branch
}

// unreadSwitchDenial decides a push that needs the current branch after a
// checkout or switch the guard could not read.
func unreadSwitchDenial(move string) *Denial {
	return Reasonf(
		GitPushMainID,
		"cannot tell which branch this push leaves from: `%s` earlier in this command moves to a "+
			"branch belt cannot read (a variable, a substitution, -, --detach, or an option belt "+
			"does not know), so the push may land on main or master. Run the switch as its own "+
			"command first, or name the branch: git push origin <branch>.",
		move,
	)
}

// switchedDefaultDenial decides a push that may find a default branch
// checked out because a checkout or switch before it moved there.
func switchedDefaultDenial(branch, move string) *Denial {
	return Reasonf(
		GitPushMainID,
		"pushing to %q is blocked: `%s` earlier in this command may leave %s checked out when the "+
			"push runs, since belt cannot be sure a later step moved off it. Name the branch the "+
			"push sends (git push origin <branch>), or run the push as its own command.",
		branch,
		move,
		branch,
	)
}

// unsureSwitchDenial decides a push from a default branch git reads before
// the command, after a switch in its directory that could not be sure.
func unsureSwitchDenial(branch string, h pushHeads) *Denial {
	return Reasonf(
		GitPushMainID,
		"pushing to %q is blocked: this command starts on %s, and belt cannot be sure the push runs "+
			"on the branch `%s` moves to: %s. Push the branch by name (git push origin <branch>), "+
			"or run the push as its own command.",
		branch,
		branch,
		h.switched[len(h.switched)-1].move,
		h.why,
	)
}
