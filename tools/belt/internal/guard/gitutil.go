package guard

import (
	"context"
	"os"
	"path"
	"slices"
	"strings"
	"unicode"

	"github.com/mad01/thismoon/kit/gitloc"
)

// gitInvocation is one parsed git invocation from a shell command: the
// directory it runs in, the subcommand, and the tokens after it. dir is ""
// when that directory cannot be told without a shell (see parseGitCmd).
// configured marks a -c or --config-env option, or an environment that may
// change git's config (gitCommandsAt); unread marks an invocation taken from
// code readShell could not follow (walkStep); moved marks a -C, which runs
// git somewhere other than the shell's directory. heads are the branches the
// checkouts and switches before it may leave checked out (headTracker).
type gitInvocation struct {
	dir        string
	sub        string
	args       []string
	configured bool
	unread     bool
	moved      bool
	heads      pushHeads
	// opaque names text a command runs that the walk could not read
	// (walkStep.opaque); such a command counts as a possible push.
	opaque opacity
}

// gitCommandsAt extracts bare `git <sub>` invocations from a shell command,
// compound commands included, each with the directory it runs in. The walk
// starts in cwd, the session working directory, and tracks `cd`/`pushd`
// across segments, so a bare invocation after `cd <repo> &&` is placed in
// that repo and a `git -C` target resolves against it (parseGitCmd). A
// variable the command sets to a literal path (`S=~/repo; git -C $S push`)
// is substituted first, when nothing could change it (resolveLiteralVars).
// An invocation's dir is "" when a cd or -C target before it needs a shell
// to resolve, or git may use another repository (walkSegments); each caller
// decides what unknown means, and no git-backed resolver fills the gap with
// belt's own process directory (gitOutput). Wrapped invocations count:
// `(git push)`, `$(git push)`, `sh -c 'git push'`, `eval "git push"` and a
// push split over continued lines (walkCommands), while a quoted string
// that only holds the words is one word, never a bare `git`.
//
// A step anywhere in the command that can change where a push goes
// (retargetsPush) marks every invocation configured: the resolvers read the
// config before the command runs, and a step after a push may run beside it.
// A git checkout or switch before an invocation changes the branch it may
// find checked out (headTracker), read before the invocation itself counts as
// a step. A step after it may run beside it, behind a & or in a pipeline, so
// one that keepsHead does not vouch for leaves it unsure (pushHeads.afterPush),
// and so does a shell function defined anywhere in the command
// (headTracker.settle).
//
// For sub push, a command that runs text the walk could not read
// (walkStep.opaque) counts as one more push, marked opaque, outside comments
// and text bodies. The commit guards fail open, so for them it counts as
// nothing.
func gitCommandsAt(command, cwd, sub string) []gitInvocation {
	var cmds []gitInvocation
	var seen []int // len(headTracker.changed) after each invocation
	retargets := false
	var heads headTracker
	walkCommands(command, cwd, func(s walkStep) {
		retargets = retargets || retargetsPush(s)
		c, ok := segmentGitCommand(s.tokens, s.dir)
		if ok = ok && c.sub == sub; ok {
			c.heads = heads.at(s, c)
		}
		heads.see(s)
		if s.opaque.what != "" && !s.prose && sub == "push" {
			cmds = append(cmds, gitInvocation{dir: s.dir, sub: sub, opaque: s.opaque})
			seen = append(seen, len(heads.changed))
		}
		if !ok {
			return
		}
		pre, _ := beforeGit(s.tokens)
		c.configured = c.configured || s.configEnv || overridesConfig(pre)
		c.unread = s.unread
		cmds = append(cmds, c)
		seen = append(seen, len(heads.changed))
	})
	for i := range cmds {
		cmds[i].configured = cmds[i].configured || retargets
		cmds[i].heads = heads.settle(cmds[i].heads, seen[i])
	}
	return cmds
}

// walkSegments calls fn for every command of a compound shell command that
// does not move the shell (`cd`, `pushd`, `popd`), with the command's tokens
// and the directory git works on when it runs there: cwd to begin with, then
// whatever the moves before it resolved to (shellWalk.move), and "" once one
// of them could not be resolved statically or git may use another repository
// (shellWalk.gitDir). A move that may carry another command (`cd /x & git
// push`) is passed to fn too, with an unknown directory. The commands come
// from readShell, nested ones included (walkCommands), with the command's
// variables substituted first when resolveLiteralVars can vouch for every
// one, so a cd, a `git -C`, or any other argument sees what the shell would
// pass; otherwise they are the command as written. A CDPATH in belt's own
// environment counts: belt runs with the session's environment, so an
// exported CDPATH reaches the Bash tool's shell too. One the shell sets
// without exporting is invisible here, and then zsh's rule of trying the
// current directory first is what keeps a relative target right.
func walkSegments(command, cwd string, fn func(tokens []string, dir string)) {
	walkCommands(command, cwd, func(s walkStep) { fn(s.tokens, s.dir) })
}

// shellWalk is what walkSegments carries from one segment to the next.
type shellWalk struct {
	dir string // the shell's directory; "" once it cannot be told
	// andOnly marks a dir that a move after && set: it holds only while the
	// commands after the move stay joined by &&, since the move may not have
	// run.
	andOnly   bool
	cdpath    bool // a relative cd target may be looked up in CDPATH
	gitEnv    bool // GIT_DIR or a sibling may be set for the rest of the command
	configEnv bool // a gitConfigEnv variable may be set for the rest of the command
	heredoc   bool // a heredoc began, so a later line may be its body
	prose     bool // walking a heredoc body or comment (walkProse)
	// feeds marks text whose output a shell or interpreter may run (a
	// substitution in a -c script or piped on), so every heredoc body in it
	// is read as code (walkProse).
	feeds bool
	depth int // how many subshells, -c scripts and evals deep the walk is
	// chain is the run of commands joined by && the walk is in (walkStep.chain),
	// and chains counts the runs one walkCommands call has seen, nested walks
	// included, so no two runs share a number.
	chain  int
	chains *int
	// mem is what the walk has learned that a later command may run
	// (walkMemory), shared by every walk of one command.
	mem *walkMemory
	// expanding are the git aliases whose bodies the walk is inside, so it
	// never reads one inside itself (aliasFeeds).
	expanding []string
	// positional marks a script run with arguments of its own (a -c script,
	// a file or a source with words after it, a ! alias body): a command
	// there that runs one of them as code is opaque (runsPositional).
	positional bool
}

// joint is the separator in front of a command or segment.
type joint int

const (
	jointList joint = iota // the start or end of the command, ; or a newline
	jointAnd               // &&
	jointOr                // ||
	jointPipe              // | or |&
	jointBg                // a lone &, which runs the command before it in the background
)

// jointBefore reads back the separator in front of segment i of command;
// jointList for the first segment and for i past the last.
func jointBefore(command string, segs []segment, i int) joint {
	if i == 0 || i >= len(segs) {
		return jointList
	}
	gap := command[segs[i-1].off+len(segs[i-1].text) : segs[i].off]
	switch {
	case strings.Contains(gap, "||"):
		return jointOr
	case strings.Contains(gap, "&&"):
		return jointAnd
	case strings.Contains(gap, "|"):
		return jointPipe
	}
	return jointList
}

// join takes the separator in front of the next segment: anything but &&
// ends the run of commands an andOnly directory holds for.
func (w *shellWalk) join(before joint) {
	if w.andOnly && before != jointAnd {
		w.dir, w.andOnly = "", false
	}
}

// move applies one cd, pushd, or popd to the walk. On top of what parseCd
// and resolveDir cannot place, the destination is unknown when the shell may
// not run the move in this shell or at all (after ||, in a pipeline, which
// bash runs in subshells, or sent to the background with &), may not read it
// as a command (inside a string, a comment, or a heredoc body the segment
// split cut up), splits its words elsewhere (a space oddSpace reports, which
// strings.Fields splits at and the shell does not), or may look its target
// up in CDPATH (searchesCdpath).
func (w *shellWalk) move(target, text string, before, after joint, inText bool) {
	if before == jointOr || before == jointPipe || after == jointPipe || after == jointBg ||
		inText || w.heredoc ||
		strings.IndexFunc(text, oddSpace) >= 0 || (w.cdpath && searchesCdpath(target)) {
		target = "-"
	}
	w.dir = resolveDir(w.dir, target)
	if before == jointAnd {
		w.andOnly = true
	}
}

// gitRepoEnvNames are the environment variables that point git at another
// repository or work tree (git help git, ENVIRONMENT). The rest only change
// objects, the index, config, or how far discovery reaches.
var gitRepoEnvNames = wordSet("GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR")

// cdpathNames are the variables a cd target is looked up in: bash and zsh
// read CDPATH, zsh ties its cdpath array to it.
var cdpathNames = wordSet("CDPATH cdpath")

// noteGitEnv remembers a gitRepoEnvNames variable named in a segment with no
// git word in it (`export GIT_DIR=...`, `GIT_DIR=...;`): it may reach every
// git after it. One that prefixes a git word is that invocation's alone. An
// assignment of a gitConfigEnv variable there is remembered the same way.
func (w *shellWalk) noteGitEnv(tokens []string) {
	pre, hasGit := beforeGit(tokens)
	if hasGit {
		return
	}
	w.gitEnv = w.gitEnv || mentionsName(strings.Join(pre, " "), gitRepoEnvNames)
	w.configEnv = w.configEnv || slices.ContainsFunc(pre, assignsGitConfigEnv)
}

// gitDir returns the directory a git invocation in this segment works on:
// the shell's, or "" when git may use another repository there. That is so
// after a noteGitEnv variable, when one prefixes the git word (`GIT_DIR=/r
// git`, `env GIT_DIR=/r git`), and when a -C or --chdir before it (`env -C`)
// runs git elsewhere.
func (w *shellWalk) gitDir(tokens []string) string {
	pre, _ := beforeGit(tokens)
	if w.gitEnv || mentionsName(strings.Join(pre, " "), gitRepoEnvNames) {
		return ""
	}
	for _, tok := range pre {
		if tok == "-C" || tok == "--chdir" || strings.HasPrefix(tok, "--chdir=") {
			return ""
		}
	}
	return w.dir
}

// beforeGit returns the tokens in front of the first git word, or all of
// them when there is none.
func beforeGit(tokens []string) (pre []string, hasGit bool) {
	if i := slices.IndexFunc(tokens, isGitWord); i >= 0 {
		return tokens[:i], true
	}
	return tokens, false
}

// isGitWord reports whether a token names git: the bare word or a path to it
// (/usr/bin/git, ~/bin/git).
func isGitWord(tok string) bool {
	return tok == "git" || strings.HasSuffix(tok, "/git")
}

// mentionsName reports whether text names one of names as a word of its
// own, once quotes and backslashes are dropped (`"GIT_DIR"=`, `GIT_\DIR=`).
func mentionsName(text string, names map[string]bool) bool {
	text = unquoteWord(text)
	notName := func(r rune) bool { return r > unicode.MaxASCII || !isNameByte(byte(r)) }
	return slices.ContainsFunc(
		strings.FieldsFunc(text, notName),
		func(w string) bool { return names[w] },
	)
}

// searchesCdpath reports whether the shell may look a cd target up in
// CDPATH: a relative path whose first component is not . or ...
func searchesCdpath(target string) bool {
	t := expandCdTarget(target)
	if t == "" || path.IsAbs(t) {
		return false
	}
	first, _, _ := strings.Cut(t, "/")
	return first != "." && first != ".."
}

// inText reports whether offset off of command lies inside a quoted string
// or a comment, as far as nextQuote can tell without escapes, so a segment
// that starts there is not a command. A comment runs from a # that starts a
// word to the end of its line; its quotes count for nothing.
func inText(command string, off int) bool {
	var quote byte
	for i := 0; i < off; i++ {
		c := command[i]
		if quote == 0 && c == '#' && (i == 0 || strings.IndexByte(" \t\n;&|", command[i-1]) >= 0) {
			end := strings.IndexByte(command[i:], '\n')
			if end < 0 || i+end >= off {
				return true
			}
			i += end
			continue
		}
		quote = nextQuote(quote, c)
	}
	return quote != 0
}

// opensHeredoc reports whether text holds a heredoc operator (<<, not a
// herestring's <<<), so the lines after it may be its body.
func opensHeredoc(text string) bool {
	return strings.Contains(strings.ReplaceAll(text, "<<<", ""), "<<")
}

// moveWords are the builtins that move the shell. zsh has chdir as another
// name for cd; bash has none.
var moveWords = wordSet("cd chdir pushd popd")

// movePrefixes are the words that may stand before cd and change whether,
// how, or in which shell it runs: precommand modifiers (zsh runs an external
// cd after `command`, bash the builtin), reserved words, and time.
var movePrefixes = wordSet(`
	command exec noglob nocorrect - time ! { if then elif else while until do coproc
`)

// parseCd reports whether a segment's tokens move the shell to another
// directory and, if so, returns the target as written. A bare cd goes home,
// so it returns "~". "-", the marker `cd -` uses, stands for a destination
// only the shell knows; resolveDir turns it into unknown. Besides cd -,
// that is popd and a bare pushd, since belt keeps no directory stack; chdir,
// which only zsh has; anything but `builtin` before the word (moveWord); and
// whatever cdTarget cannot place.
func parseCd(tokens []string) (string, bool) {
	name, args, plain := moveWord(tokens)
	switch {
	case name == "":
		return "", false
	case !plain || name == "chdir" || name == "popd":
		return "-", true
	}
	return cdTarget(name == "pushd", args), true
}

// moveWord finds a moveWords word at the start of a segment, after any
// `builtin`, movePrefixes word, or NAME=value assignment (`HOME=/x cd`), and
// returns it with the tokens after it. A run of two or more dots counts as
// one: the Bash tool's shell aliases `..` to `cd ..`. plain is false when
// anything but builtin came first, or the word is such a run.
func moveWord(tokens []string) (name string, args []string, plain bool) {
	plain = true
	for i, tok := range tokens {
		switch {
		case moveWords[tok]:
			return tok, tokens[i+1:], plain
		case len(tok) > 1 && strings.Trim(tok, ".") == "":
			return tok, tokens[i+1:], false
		case tok == "builtin":
		case movePrefixes[tok] || isAssignment(tok) || (!plain && strings.HasPrefix(tok, "-")):
			plain = false // the last case takes a prefix's own flags, as in `time -p`
		default:
			return "", nil, false
		}
	}
	return "", nil, false
}

// isAssignment reports whether tok is a NAME=value or NAME+=value word.
func isAssignment(tok string) bool {
	name, _, ok := strings.Cut(tok, "=")
	return ok && isShellName(strings.TrimSuffix(name, "+"))
}

// cdTarget reads the target of a plain cd or pushd from the tokens after
// it, skipping redirections, the -L, -e, and -@ flags, and one --. It returns
// "~" for a bare cd and "-" for a bare pushd and for whatever only the shell
// can place: -P, whose physical mode resolves symlinks before `..` where the
// walk cleans as text; any other flag (zsh's -q and -s, a bundle like -LP);
// a stack entry (+1, -1, which zsh reads as one even after --); and a second
// word (zsh's `cd old new`, or a `&` that sends the cd to a subshell).
func cdTarget(pushd bool, args []string) string {
	target, flagsDone := "", false
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if redir, operandNext := redirection(tok); redir {
			if operandNext {
				i++
			}
			continue
		}
		bare := strings.TrimLeft(tok, `"'`)
		switch {
		case target != "":
			return "-"
		case !flagsDone && tok == "--":
			flagsDone = true
		case !flagsDone && (tok == "-L" || tok == "-e" || tok == "-@"):
		case strings.HasPrefix(bare, "-") || strings.HasPrefix(bare, "+"):
			return "-"
		default:
			target = tok
		}
	}
	switch {
	case target != "":
		return target
	case pushd:
		return "-"
	}
	return "~"
}

// redirection reports whether tok is a redirection (`>f`, `2>&1`, `&>>log`,
// `<in`) and whether its operand is the next token rather than part of tok.
func redirection(tok string) (redir, operandNext bool) {
	rest := strings.TrimLeft(tok, "0123456789")
	for _, op := range []string{"&>>", "&>", ">>", ">&", "<&", ">|", "<>", ">", "<"} {
		if after, ok := strings.CutPrefix(rest, op); ok {
			return true, after == ""
		}
	}
	return false, false
}

// segmentGitCommand parses the first bare `git` in one segment's tokens: one
// invocation per segment.
func segmentGitCommand(tokens []string, cwd string) (gitInvocation, bool) {
	if i := slices.IndexFunc(tokens, isGitWord); i >= 0 {
		return parseGitCmd(tokens[i+1:], cwd)
	}
	return gitInvocation{}, false
}

// parseGitCmd parses the tokens after `git`: global flags first, then the
// subcommand and its arguments. cwd is the directory the shell is in when
// git runs. A -C target gets the expansions a cd target gets and is placed
// under cwd (placeDir), so `git -C ~/repo`, `git -C $HOME/repo`, and the
// absolute path all name one directory, and a target only a shell can expand
// leaves dir "" rather than a path git would never see. Unlike a cd target
// the path is not cleaned: git chdirs into it and the OS resolves `..`
// physically, so `link/..` is the link target's parent there, where a
// lexical clean would name the link's own parent and judge the wrong repo.
// Repeated -C flags chain the way git chains them. git takes -C only as a
// separate token, so there is no = form to parse. --git-dir and --work-tree
// point git at a tree only git itself can name, so either form leaves dir
// "" as well. -c and --config-env set configured, since either may name
// another push destination (remote.origin.pushurl, url.X.pushInsteadOf).
// Every other global flag before the subcommand is skipped, together with
// its value when it takes one (gitValueFlags), so `git --no-pager push` is
// still a push. ok is false when no subcommand follows the flags.
func parseGitCmd(tokens []string, cwd string) (gitInvocation, bool) {
	c := gitInvocation{dir: cwd}
	otherTree := false
	i := 0
	for i < len(tokens) {
		tok := tokens[i]
		switch {
		case tok == "-C" && i+1 < len(tokens):
			c.dir = placeDir(c.dir, tokens[i+1])
			c.moved = true
			i += 2
		case tok == "-c" || tok == "--config-env":
			c.configured = true
			i += 2
		case strings.HasPrefix(tok, "--config-env="):
			c.configured = true
			i++
		case tok == "--git-dir" || tok == "--work-tree":
			otherTree = true
			i += 2 // separate-value form consumes the path token too
		case strings.HasPrefix(tok, "--git-dir=") || strings.HasPrefix(tok, "--work-tree="):
			otherTree = true
			i++
		case gitValueFlags[tok]:
			i += 2
		case strings.HasPrefix(tok, "-"):
			i++ // a flag without a value (--no-pager, -p, --bare) or an = form
		default:
			if otherTree {
				c.dir = ""
			}
			c.sub = tok
			c.args = tokens[i+1:]
			return c, true
		}
	}
	return gitInvocation{}, false
}

// gitValueFlags are the global git flags that take their value as the next
// token. Their = forms are one token and skip like any other flag. -C and
// --git-dir/--work-tree take a value too but are handled on their own in
// parseGitCmd.
var gitValueFlags = map[string]bool{
	"-c": true, "--namespace": true, "--super-prefix": true,
	"--config-env": true, "--attr-source": true,
}

// unresolvableChars are the shell characters a directory target may still
// carry after belt's own expansion. Any of them means a shell would rewrite
// the target into something belt cannot see (a glob, a brace or a zsh glob
// qualifier, a redirection or a `&` glued on), so the destination is unknown.
const unresolvableChars = "$*?~`\"'[]{}()<>&|;"

// resolveDir applies a `cd` target to the current directory and returns the
// new one, or "" for a destination it cannot determine statically (see
// placeDir). The result is cleaned lexically, which is what a shell's cd does
// by default: `cd link/..` lands in the link's own parent, not in the link
// target's. An unknown directory is what makes a later push fail closed (see
// GitPushMain.checkPush) and a commit guard or hint stay quiet.
func resolveDir(cwd, target string) string {
	dir := placeDir(cwd, target)
	if dir == "" {
		return ""
	}
	return path.Clean(dir)
}

// placeDir expands a directory target and places it under cwd as written,
// without cleaning it: "" for `-` (a directory only the shell remembers), a
// target expandCdTarget rejects, or a relative target from an unknown cwd.
// A `git -C` target uses it directly, so git sees the path as typed.
func placeDir(cwd, target string) string {
	if target == "-" {
		return ""
	}
	target = expandCdTarget(target)
	switch {
	case target == "":
		return ""
	case path.IsAbs(target):
		return target
	case cwd == "":
		return ""
	}
	return strings.TrimSuffix(cwd, "/") + "/" + target
}

// expandCdTarget performs the expansions the shell would on a directory
// target, as far as they can be done without a shell: a surrounding quote
// pair is dropped, a leading $HOME or ${HOME} becomes the home directory
// unless single-quoted, and so does an unquoted leading ~. A target that
// still carries a shell metacharacter or a stray quote afterwards returns "".
func expandCdTarget(target string) string {
	quote, inner := unquote(target)
	if quote != '\'' {
		inner = expandHomeVar(inner)
	}
	if quote == 0 {
		inner = expandHome(inner)
	}
	if strings.ContainsAny(inner, unresolvableChars) {
		return ""
	}
	return inner
}

// expandHome rewrites a leading ~ (bare, or followed by /) to the home
// directory and leaves the rest of the path as written, so `~/link/..` keeps
// its `..` for git to resolve; config.ExpandHome would clean it away. Any
// other ~ form (~user) stays for the metacharacter check to reject, and so
// does the ~ itself when there is no home directory to expand it to.
func expandHome(target string) string {
	rest, ok := strings.CutPrefix(target, "~")
	if !ok || (rest != "" && rest[0] != '/') {
		return target
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return target
	}
	return home + rest
}

// unquote strips one pair of matching surrounding quotes and reports which
// quote it was, 0 for none.
func unquote(s string) (byte, string) {
	n := len(s)
	if n >= 2 && (s[0] == '"' || s[0] == '\'') && s[n-1] == s[0] {
		return s[0], s[1 : n-1]
	}
	return 0, s
}

// expandHomeVar rewrites a leading $HOME or ${HOME} to the home directory,
// through the same helper a leading ~ goes through. A longer variable
// ($HOMEBREW_PREFIX, say) is left alone for the metacharacter check to
// reject.
func expandHomeVar(target string) string {
	for _, v := range []string{"${HOME}", "$HOME"} {
		rest, ok := strings.CutPrefix(target, v)
		if ok && (rest == "" || rest[0] == '/') {
			return expandHome("~" + rest)
		}
	}
	return target
}

// resolveLiteralVars returns command with each variable reference replaced by
// the literal the command itself assigns that variable, as the shell would
// expand it: `S=~/repo; git -C $S push` reads as a push from ~/repo. It is all
// or nothing. When anything in the command could leave a variable empty or
// set it to something else by the time it is used, command comes back
// unchanged, every reference stays as written, and the guards fail closed on
// the directories they name. It refuses rather than parse shell structure;
// plainShell, literalAssignments, substituteVars, and inertCommand each say
// what they refuse. A variable set in a sourced file, a shell function, or an
// alias is invisible here, as everything outside the command text is.
func resolveLiteralVars(command string) string {
	if !strings.Contains(command, "$") || !plainShell(command) {
		return command
	}
	vars, ok := literalAssignments(command)
	if !ok || len(vars) == 0 {
		return command
	}
	resolved, ok := substituteVars(command, vars)
	if !ok || !inertCommand(resolved, vars) {
		return command
	}
	return resolved
}

// refusedChars are refused anywhere in a command: a backslash escapes or
// continues a line, a backtick substitutes, ( and ) open subshells, functions
// and substitutions, { and } (a ${NAME} reference aside) group commands, [
// opens a test or a subscript, and # starts a comment.
const refusedChars = "\\`(){}[#"

// unquotedGlobChars are refused outside quotes: a glob, zsh's extended ^
// included, can expand to a builtin's name.
const unquotedGlobChars = "*?^"

// plainShell reports whether command stays clear of the constructs that can
// make an assignment conditional, hide one from the segment split, or turn
// text into a command belt cannot see: refusedChars, unquotedGlobChars, a
// heredoc or herestring (<<), a character oddSpace reports, and an
// unbalanced quote.
func plainShell(command string) bool {
	if strings.Contains(command, "<<") || strings.IndexFunc(command, oddSpace) >= 0 {
		return false
	}
	var quote byte
	for i := 0; i < len(command); i++ {
		if _, n := varRef(command[i:]); n > 0 && quote != '\'' {
			i += n - 1 // a ${NAME} reference keeps its braces
			continue
		}
		c := command[i]
		if strings.IndexByte(refusedChars, c) >= 0 ||
			(quote == 0 && strings.IndexByte(unquotedGlobChars, c) >= 0) {
			return false
		}
		quote = nextQuote(quote, c)
	}
	return quote == 0
}

// oddSpace reports a control character or a space other than a blank, a tab
// or a newline. strings.Fields splits at some of them where the shell does
// not: zsh reads `X=1<no-break space>S=/a` as one assignment, to X.
func oddSpace(r rune) bool {
	return r != ' ' && r != '\t' && r != '\n' && (unicode.IsSpace(r) || unicode.IsControl(r))
}

// nextQuote returns the quote the shell is inside after reading c, given the
// one it was inside before: 0 outside quotes, else the quote that opened the
// string. There are no escapes to track, since plainShell refuses backslashes.
func nextQuote(quote, c byte) byte {
	switch {
	case quote == 0 && (c == '\'' || c == '"'):
		return c
	case quote == c:
		return 0
	}
	return quote
}

// literalVar is a variable a command assigns a plain literal: its value as the
// shell stores it, and the offset just past the segment that assigns it, where
// references to it may begin.
type literalVar struct {
	value string
	from  int
}

// literalAssignments returns the variables the command's assignment-only
// segments set (`S=~/repo`, `S="/abs"`, `A=/a B=/b`), keyed by name. ok is
// false, so nothing resolves, when any such segment could leave something
// other than one plain literal per name in this shell: one that may not run or
// runs in a subshell (unconditional), a value literalValue rejects, a name the
// shell manages itself (shellManagedNames), or a name assigned twice.
func literalAssignments(command string) (map[string]literalVar, bool) {
	segs := splitSegments(command)
	vars := map[string]literalVar{}
	for i, seg := range segs {
		tokens := strings.Fields(seg.text)
		if !assignsOnly(tokens) {
			continue
		}
		if !unconditional(command, segs, i) {
			return nil, false
		}
		for _, tok := range tokens {
			name, raw, _ := strings.Cut(tok, "=")
			value, ok := literalValue(raw)
			if _, seen := vars[name]; !ok || seen || shellManagedNames[name] {
				return nil, false
			}
			vars[name] = literalVar{value: value, from: seg.off + len(seg.text)}
		}
	}
	return vars, true
}

// assignsOnly reports whether every token is a NAME=value assignment, so the
// segment sets shell variables and runs no command.
func assignsOnly(tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	for _, tok := range tokens {
		name, _, ok := strings.Cut(tok, "=")
		if !ok || !isShellName(name) {
			return false
		}
	}
	return true
}

// unconditional reports whether segment i of command always runs, in the
// shell itself: nothing before it is quoted, so it is no piece of a string
// the split cut up; it starts the command or follows a ; or a newline, never
// &&, || or a pipe; and no pipe follows it. splitSegments drops the
// separators, so they are read back from the gaps between segments.
func unconditional(command string, segs []segment, i int) bool {
	seg := segs[i]
	start, end := 0, len(command)
	if i > 0 {
		start = segs[i-1].off + len(segs[i-1].text)
	}
	if i+1 < len(segs) {
		end = segs[i+1].off
	}
	return !strings.ContainsAny(command[:seg.off], `'"`) &&
		!strings.ContainsAny(command[start:seg.off], "&|") &&
		!strings.Contains(command[seg.off+len(seg.text):end], "|")
}

// plainValuePunct is the punctuation a literal value may hold besides
// letters and digits: enough for a path, and nothing an unquoted $S would
// split, glob, or hand to the shell as syntax.
const plainValuePunct = "/._-+@%:,"

// literalValue reads the value side of an assignment the way the shell
// stores it, where that needs no shell: the quote removal and the leading ~
// or $HOME expansion a cd target gets (expandCdTarget). A surrounding quote
// pair is the only quoting it accepts. ok is false for an empty value, for
// anything expandCdTarget rejects, and for a value with characters outside
// letters, digits, and plainValuePunct.
func literalValue(raw string) (string, bool) {
	value := expandCdTarget(raw)
	if value == "" {
		return "", false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) &&
			!strings.ContainsRune(plainValuePunct, r) {
			return "", false
		}
	}
	return value, true
}

// shellManagedNames are the variables the shell sets, ties, or reads on its
// own, so a later reference need not expand to the literal the command gave
// it: zsh 5.9's special parameters, the ones zsh modules and bash 5 set, PWD
// and OLDPWD, which every cd rewrites, and HOME, which decides what ~ means.
var shellManagedNames = wordSet(`
	_ ARGC argv cdpath CDPATH COLUMNS EGID EUID fignore FIGNORE fpath FPATH
	FUNCNEST GID histchars HISTCHARS HISTCMD HISTSIZE HOME IFS KEYBOARD_HACK
	LANG LC_ALL LINENO LINES mailpath MAILPATH manpath MANPATH module_path
	MODULE_PATH NULLCMD OPTARG OPTIND parameters path PATH pipestatus PPID
	prompt PROMPT PROMPT2 PROMPT3 PROMPT4 PS1 PS2 PS3 PS4 psvar PSVAR RANDOM
	READNULLCMD SAVEHIST SECONDS SHLVL SPROMPT status TERM TRY_BLOCK_ERROR
	TRY_BLOCK_INTERRUPT TTYIDLE UID USERNAME WORDCHARS zsh_eval_context
	ZSH_EVAL_CONTEXT ZSH_SUBSHELL
	PWD OLDPWD dirstack DIRSTACK REPLY reply MATCH MBEGIN MEND match mbegin
	mend ERRNO EPOCHSECONDS EPOCHREALTIME epochtime ZSH_ARGZERO
	BASHPID BASH_ARGC BASH_ARGV BASH_COMMAND BASH_LINENO BASH_REMATCH
	BASH_SOURCE BASH_SUBSHELL BASH_VERSINFO FUNCNAME GROUPS PIPESTATUS SRANDOM
	SHELLOPTS BASHOPTS POSIXLY_CORRECT GLOBIGNORE
`)

// substituteVars returns command with each $NAME or ${NAME} reference to a
// variable in vars replaced by its value. ok is false when any $ is anything
// else: a reference refAt rejects, one before or inside its variable's
// assignment, one to a variable the command does not assign, or a $ that
// starts no plain reference ($1, $?, ${S:-x}). $HOME stays as written for
// expandCdTarget to expand, as it did before variables resolved.
func substituteVars(command string, vars map[string]literalVar) (string, bool) {
	var b strings.Builder
	var quote byte
	for i := 0; i < len(command); {
		c := command[i]
		if c != '$' {
			b.WriteByte(c)
			quote = nextQuote(quote, c)
			i++
			continue
		}
		name, n := refAt(command, i, quote)
		v, ok := vars[name]
		switch {
		case n > 0 && name == "HOME":
			b.WriteString(command[i : i+n])
		case n == 0 || !ok || i < v.from:
			return "", false
		default:
			b.WriteString(v.value)
		}
		i += n
	}
	return b.String(), true
}

// refAt reads the variable reference starting at command[i], a $, and returns
// its name and length; n is 0 unless the reference expands to the plain value:
// it sits outside single quotes, not right after a ~ (zsh reads ~$S as text),
// and no : or [ follows it ($S:h and $S[1] are a zsh modifier and subscript).
func refAt(command string, i int, quote byte) (name string, n int) {
	name, n = varRef(command[i:])
	if n == 0 || quote == '\'' || (i > 0 && command[i-1] == '~') {
		return "", 0
	}
	if end := i + n; end < len(command) && (command[end] == ':' || command[end] == '[') {
		return "", 0
	}
	return name, n
}

// varRef reads a $NAME or ${NAME} reference at the start of s and returns the
// name and the reference's length in bytes; n is 0 when s does not start with
// one. ${NAME:-x} and the other operator forms are not plain references.
func varRef(s string) (name string, n int) {
	rest, ok := strings.CutPrefix(s, "$")
	if !ok {
		return "", 0
	}
	if inner, ok := strings.CutPrefix(rest, "{"); ok {
		name, _, closed := strings.Cut(inner, "}")
		if !closed || !isShellName(name) {
			return "", 0
		}
		return name, len("${}") + len(name)
	}
	end := 0
	for end < len(rest) && isNameByte(rest[end]) {
		end++
	}
	if !isShellName(rest[:end]) {
		return "", 0
	}
	return rest[:end], len("$") + end
}

// inertCommand reports whether nothing in the resolved command can set one of
// vars again, or run code that might: no word, quotes removed, is in
// shellDenyWords, no `.` has a file to source after it, and each name stands
// as a word of its own (nameWords) only where it is assigned. It reads the
// command after substitution, so a command word or a name put together from
// other variables (`$C$D $A$B`) is checked as the shell will run it.
func inertCommand(resolved string, vars map[string]literalVar) bool {
	for _, words := range simpleCommands(resolved) {
		for i, w := range words {
			if shellDenyWords[w] || (w == "." && i+1 < len(words)) {
				return false
			}
		}
	}
	for name := range vars {
		if nameWords(resolved, name) != 1 {
			return false
		}
	}
	return true
}

// shellDenyWords are the zsh 5.9 (modules included) and bash 5 keywords and
// builtins that can run code, set a variable an argument names, or change how
// the rest of a command parses or expands. Builtins that do none of that (cd,
// echo, test, pwd) are left out, and so are precommand modifiers (builtin,
// command, noglob), whose next word is checked on its own. `.` is handled
// apart in inertCommand, since `git add .` is common and harmless.
var shellDenyWords = wordSet(`
	if then else elif fi for foreach while until do done case esac select
	repeat coproc function end
	source eval exec trap fc r sched emulate autoload functions unfunction
	alias unalias hash enable disable set setopt unsetopt shopt zmodload zle
	history
	read readarray mapfile vared getln getopts print printf let unset wait
	declare typeset local export readonly integer float private nameref
	compgen zparseopts zformat zregexparse zstyle
	sysread syswrite sysopen syserror zsystem strftime zstat stat zselect
	ztcp zsocket zpty zcurses pcre_match zgetattr zlistattr ztie zgdbmpath
`)

// simpleCommands splits a command plainShell accepted into its simple
// commands, at unquoted ;, &, | and newlines, and each of those into its
// words, at unquoted blanks, < and >, with the quotes removed. A redirection
// target counts as a word, which only makes inertCommand stricter.
func simpleCommands(command string) [][]string {
	var cmds [][]string
	for _, piece := range splitUnquoted(command, ";&|\n") {
		var words []string
		for _, w := range splitUnquoted(piece, " \t<>") {
			if w != "" {
				words = append(words, removeQuotes(w))
			}
		}
		cmds = append(cmds, words)
	}
	return cmds
}

// splitUnquoted splits s at each byte of seps that is outside quotes,
// dropping those bytes and keeping the quotes.
func splitUnquoted(s, seps string) []string {
	var parts []string
	var quote byte
	start := 0
	for i := 0; i < len(s); i++ {
		if quote == 0 && strings.IndexByte(seps, s[i]) >= 0 {
			parts = append(parts, s[start:i])
			start = i + 1
			continue
		}
		quote = nextQuote(quote, s[i])
	}
	return append(parts, s[start:])
}

// removeQuotes drops the quotes the shell removes from a word.
func removeQuotes(word string) string {
	var b strings.Builder
	var quote byte
	for i := 0; i < len(word); i++ {
		next := nextQuote(quote, word[i])
		if next == quote {
			b.WriteByte(word[i])
		}
		quote = next
	}
	return b.String()
}

// nameWords counts the runs of name characters in command that spell name.
// Every one counts, inside a path or an option too: zsh arithmetic, which
// builtins such as shift evaluate, assigns through `--S`, `S/=2`, and
// `a/S++` alike.
func nameWords(command, name string) int {
	count := 0
	for i := 0; i < len(command); {
		j := i
		for j < len(command) && isNameByte(command[j]) {
			j++
		}
		if j == i {
			j++
		} else if command[i:j] == name {
			count++
		}
		i = j
	}
	return count
}

// wordSet returns the whitespace-separated words as a set.
func wordSet(words string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(words) {
		set[w] = true
	}
	return set
}

// isShellName reports whether s is a shell variable name: a letter or _,
// then letters, digits, and _.
func isShellName(s string) bool {
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isNameByte(s[i]) {
			return false
		}
	}
	return true
}

// isNameByte reports whether c may appear in a shell variable name.
func isNameByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// gitCurrentBranch shells out to git; "" when dir is not a repo or git fails.
func gitCurrentBranch(dir string) string {
	return gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// gitRemoteURL shells out to git; "" when dir is outside a repo or the repo
// has no origin remote.
func gitRemoteURL(dir string) string {
	return gitOutput(dir, "remote", "get-url", "origin")
}

// gitUserEmail returns the effective user.email for the repo at dir (local
// config or global fallback, exactly what a commit there would record); ""
// when git has no identity configured or dir is not a repo.
func gitUserEmail(dir string) string {
	return gitOutput(dir, "config", "user.email")
}

// gitOutput runs one git command in dir and returns its trimmed stdout, or ""
// on any failure. A guard helper degrades to "unknown", never errors, and
// never guesses: an empty dir means the caller could not tell where the
// command runs, and running git in belt's own process directory instead would
// judge the session's repo in place of the target one. That is how a `cd` with
// a target only a shell can resolve once let a push into a non-exempt repo
// ride an exempt session cwd.
func gitOutput(dir string, args ...string) string {
	return gitloc.Output(context.Background(), dir, args...)
}

// GitCommitDirs returns the directory every `git commit` in a shell command
// runs in, placed as gitCommandsAt places it: a tracked `cd`/`pushd`, a `git
// -C` target, else cwd. An entry is "" when the directory cannot be told
// without a shell. Exported for the commit-policy and lint-policy hints,
// which watch the same invocations the commit guards check.
func GitCommitDirs(command, cwd string) []string {
	var dirs []string
	for _, c := range gitCommandsAt(command, cwd, "commit") {
		dirs = append(dirs, c.dir)
	}
	return dirs
}

// CanonicalRepoAt resolves the repo at dir to the canonical host/owner/repo
// identity allow_repos patterns match, or "" when there is no resolvable
// origin remote.
func CanonicalRepoAt(dir string) string {
	return gitloc.CanonicalRepo(gitRemoteURL(dir))
}
