package guard

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// walkStep is one command walkCommands reached, with the directory git works
// on when it runs there ("" when only a shell can tell).
type walkStep struct {
	tokens []string
	dir    string
	// configEnv marks a command an assignment earlier in the command may
	// reach with a variable that changes where git pushes (gitConfigEnv).
	configEnv bool
	// unread marks a command taken from code readShell could not follow,
	// read fail-closed (walkUnread).
	unread bool
	// targets are the command's redirection targets as tokens, heredoc
	// delimiters included. A command from the token walk keeps them in
	// tokens instead.
	targets []string
	// prose marks a command taken from a comment or a heredoc body no shell
	// runs (walkProse): its words are only text.
	prose bool
}

// walkCommands is walkSegments with the whole step. It reads the command
// with readShell, so quotes, comments and heredoc bodies hide nothing and
// split nothing, and it also walks the code a command runs out of sight: a
// ( ) subshell, a $(...) or backtick substitution, a process substitution,
// the script of a shell's -c flag and the text of eval. Those run in a
// subshell, so they start in the directory the walk has reached and a cd
// inside them moves nothing outside; eval runs in this shell, so a move
// inside it leaves the directory unknown. Comments and heredoc bodies no
// shell runs are walked the old token way as prose, and a body a shell may
// run is walked as code (walkProse). A code text readShell cannot follow
// gets the full walk up to the line it fails on and a fail-closed read of
// the rest (walkLines); a body read as code that it cannot follow, or one
// with a space oddSpace reports, is walked the token way (walkTokens).
func walkCommands(command, cwd string, fn func(walkStep)) {
	command = resolveLiteralVars(command)
	w := shellWalk{dir: cwd, cdpath: os.Getenv("CDPATH") != ""}
	w.walkRead(command, true, fn)
}

// walkRead walks one text. code says the shell runs it as commands; with
// code false it is a heredoc body a shell may run (walkProse), and one the
// reader cannot follow is walked the token way. Code nested past
// maxShellNesting is read fail-closed.
func (w *shellWalk) walkRead(text string, code bool, fn func(walkStep)) {
	if code && w.depth > maxShellNesting {
		w.walkUnread(text, fn)
		return
	}
	cmds, readTo, ok := readShellLines(text)
	switch {
	case !ok && code:
		w.walkLines(text, readTo, fn)
	case !ok || strings.IndexFunc(text, oddSpace) >= 0:
		w.walkTokens(text, false, fn)
	default:
		w.walkCmds(text, cmds, fn)
	}
}

// walkLines walks a code text the reader could follow only up to readTo,
// the end of the last whole line before the one it failed on. bash runs
// those lines before it meets the syntax error, so they get the full walk;
// the rest is read fail-closed.
func (w *shellWalk) walkLines(text string, readTo int, fn func(walkStep)) {
	head := text[:readTo]
	cmds, ok := readShell(head)
	switch {
	case !ok:
		w.walkUnread(text, fn)
		return
	case strings.IndexFunc(head, oddSpace) >= 0:
		w.walkTokens(head, false, fn)
	default:
		w.walkCmds(head, cmds, fn)
	}
	w.walkUnread(text[readTo:], fn)
}

// walkUnread walks code the reader could not follow, fail-closed: quotes and
// backslashes dropped and every ( ) $( ) backtick and brace read as a
// separator, so a git word hidden in `(git`, `$(git`, `"g"it` or `p\ush`
// still reaches fn, marked unread.
func (w *shellWalk) walkUnread(text string, fn func(walkStep)) {
	loose := strings.NewReplacer(
		"\\\n", "", `'`, "", `"`, "", `\`, "",
		"(", ";", ")", ";", "`", ";", "{", ";", "}", ";",
	).Replace(text)
	w.walkTokens(loose, true, fn)
}

// walkCmds walks the commands readShell read from text.
func (w *shellWalk) walkCmds(text string, cmds []shellCmd, fn func(walkStep)) {
	for _, c := range cmds {
		if !w.prose {
			w.join(c.before)
		}
		w.walkNested(text, c, fn)
		if tokens := c.tokens(); len(tokens) > 0 {
			w.walkWords(c, tokens, fn)
		}
		w.heredoc = w.heredoc || len(c.docs) > 0
		w.walkProse(text, c, fn)
	}
}

// walkNested walks what a command runs in a subshell, each from the
// directory the walk has reached. When the command may run the text a
// substitution prints (runsText), that text is code, so a heredoc body
// printed there is read as code too (shellWalk.feeds).
func (w *shellWalk) walkNested(text string, c shellCmd, fn func(walkStep)) {
	feeds := w.feeds || runsText(c)
	for _, s := range append(c.pieces(), c.subshells...) {
		inner := w.child(w.dir)
		inner.feeds = feeds
		inner.walkRead(text[s.from:s.to], true, fn)
	}
}

// walkProse walks a command's heredoc bodies and comments in this shell, as
// lines that might be commands: a git word there reaches fn and a cd there
// leaves the directory unknown, so a body a shell does run (bash <<EOF) can
// never place a push. A body a shell may run (docRuns) is read as code. A
// comment and any other body are read the token way (walkTokens), as the git
// guards always read them, with their steps marked prose: a quoted body
// expands nothing, so a backtick or $( ) in it is text. The shell runs the
// $( ) and backticks of an unquoted body itself before the command reads it,
// a ' or # in front of them included, since both are literal in a body, so
// those bodies are walked as code wherever the body goes.
func (w *shellWalk) walkProse(text string, c shellCmd, fn func(walkStep)) {
	prose := w.prose
	w.prose = true
	runs := w.feeds || docRuns(c)
	asProse := func(s walkStep) {
		s.prose = true
		fn(s)
	}
	for _, d := range c.docs {
		if runs {
			w.walkRead(text[d.body.from:d.body.to], false, fn)
		} else {
			w.walkTokens(text[d.body.from:d.body.to], false, asProse)
		}
		if d.quoted {
			continue
		}
		for _, s := range bodySubstitutions(text, d.body) {
			inner := w.child(w.dir)
			inner.feeds = runs
			inner.walkRead(text[s.from:s.to], true, fn)
		}
	}
	for _, s := range c.comments {
		w.walkTokens(text[s.from:s.to], false, asProse)
	}
	w.prose = prose
}

// docRuns reports whether a command may run the heredoc bodies it reads:
// every command but cat, tee, head, tail, git commit and a gh pr, issue,
// release, gist or api call, and one of those too when a pipe or a process
// substitution may hand what it prints to something that runs it.
func docRuns(c shellCmd) bool {
	words := stripWrappers(c.tokens())
	if len(words) == 0 || c.after == jointPipe || handsOn(c) {
		return true
	}
	switch filepath.Base(words[0]) {
	case "cat", "tee", "head", "tail":
		return false
	case "gh":
		return len(words) < 2 || !ghTextGroups[words[1]]
	case "git":
		g, ok := segmentGitCommand(words, "")
		return !ok || g.sub != "commit"
	}
	return true
}

// ghTextGroups are the gh command groups whose heredoc input is a body, a
// note or a request, never a script.
var ghTextGroups = wordSet("pr issue release gist api")

// runsText reports whether a command may run the text a substitution in it
// prints: a shell's -c script, eval, source or ., an interpreter, or a pipe
// or process substitution that hands the output on.
func runsText(c shellCmd) bool {
	tokens := c.tokens()
	switch {
	case c.after == jointPipe, handsOn(c), inlineScriptAt(tokens) >= 0, evalAt(tokens) >= 0:
		return true
	}
	words := stripWrappers(tokens)
	if len(words) == 0 {
		return false
	}
	_, interpreter := isInterpreter(filepath.Base(words[0]))
	return interpreter || words[0] == "source" || words[0] == "."
}

// handsOn reports whether a command's words or redirections hold a >( )
// process substitution, which hands what the command writes to a command.
func handsOn(c shellCmd) bool {
	for _, words := range [][]placedWord{c.words, c.targets} {
		if slices.ContainsFunc(
			words,
			func(w placedWord) bool { return strings.Contains(w.raw, ">(") },
		) {
			return true
		}
	}
	return false
}

// walkWords handles one command's words: a cd moves the walk, anything else
// reaches fn, and a shell -c script or an eval text is walked in turn.
func (w *shellWalk) walkWords(c shellCmd, tokens []string, fn func(walkStep)) {
	w.cdpath = w.cdpath || mentionsName(strings.Join(tokens, " "), cdpathNames)
	if target, ok := parseCd(tokens); ok {
		if target == "-" {
			fn(walkStep{tokens: tokens, configEnv: w.configEnv})
		}
		raw := make([]string, 0, len(c.words))
		for _, word := range c.words {
			raw = append(raw, word.raw)
		}
		w.move(target, strings.Join(raw, " "), c.before, c.after, w.prose)
		return
	}
	w.noteGitEnv(tokens)
	dir := w.gitDir(tokens)
	targets := make([]string, 0, len(c.targets))
	for _, t := range c.targets {
		targets = append(targets, t.token())
	}
	fn(walkStep{tokens: tokens, dir: dir, configEnv: w.configEnv, targets: targets})
	if at := inlineScriptAt(tokens); at >= 0 {
		w.childFor(dir, tokens[:at]).walkRead(c.words[at].script(), true, fn)
	}
	if at := evalAt(tokens); at >= 0 {
		w.walkEval(c, tokens, at, fn)
	}
}

// walkEval walks the text eval runs. eval runs it in this shell, so the
// variables it sets stay set, and a directory it moves to is unknown here.
func (w *shellWalk) walkEval(c shellCmd, tokens []string, at int, fn func(walkStep)) {
	script := make([]string, 0, len(c.words)-at)
	for _, word := range c.words[at+1:] {
		script = append(script, word.script())
	}
	inner := w.childFor(w.dir, tokens[:at])
	inner.walkRead(strings.Join(script, " "), true, fn)
	w.gitEnv = w.gitEnv || inner.gitEnv
	w.configEnv = w.configEnv || inner.configEnv
	w.cdpath = w.cdpath || inner.cdpath
	if inner.dir != w.dir || inner.andOnly {
		w.dir, w.andOnly = "", false
	}
}

// child starts the walk of a subshell in dir. It inherits what the shell
// has set so far; what it sets itself stays inside it.
func (w *shellWalk) child(dir string) *shellWalk {
	return &shellWalk{
		dir: dir, cdpath: w.cdpath, gitEnv: w.gitEnv, configEnv: w.configEnv, heredoc: w.heredoc,
		feeds: w.feeds, depth: w.depth + 1,
	}
}

// childFor is child for a shell or eval that pre, the words in front of its
// script, may run with an environment of its own (`GIT_DIR=/r bash -c`).
func (w *shellWalk) childFor(dir string, pre []string) *shellWalk {
	inner := w.child(dir)
	inner.gitEnv = inner.gitEnv || mentionsName(strings.Join(pre, " "), gitRepoEnvNames)
	inner.configEnv = inner.configEnv || overridesConfig(pre)
	return inner
}

// shellCFlag is a shell option word that holds -c, alone or in a bundle
// (`bash -lc`, `sh -ec`): the shell then runs its first operand as a script.
var shellCFlag = regexp.MustCompile(`^-[A-Za-z]*c[A-Za-z]*$`)

// inlineScriptAt returns the index of the script word a shell interpreter
// (shellInterpreters, the script guard's list) runs with -c, or -1. The
// interpreter may sit anywhere in the command, behind sudo, env, xargs or
// any other word, as git may (segmentGitCommand). Its options come first:
// -o and +o take a value, and the script is the first word after them.
func inlineScriptAt(tokens []string) int {
	i := slices.IndexFunc(
		tokens,
		func(tok string) bool { return shellInterpreters[filepath.Base(tok)] },
	)
	if i < 0 {
		return -1
	}
	inline := false
	for j := i + 1; j < len(tokens); j++ {
		switch tok := tokens[j]; {
		case tok == "-o" || tok == "+o":
			j++
		case shellCFlag.MatchString(tok):
			inline = true
		case strings.HasPrefix(tok, "-") || strings.HasPrefix(tok, "+"):
		case inline:
			return j
		default:
			return -1
		}
	}
	return -1
}

// evalAt returns the index of an eval word that starts the command, behind
// any wrapper stripWrappers drops, or -1.
func evalAt(tokens []string) int {
	rest := stripWrappers(tokens)
	if len(rest) == 0 || rest[0] != "eval" {
		return -1
	}
	return len(tokens) - len(rest)
}

// walkTokens is the token walk every walker used before readShell: split at
// &&, ||, ;, | and newlines, quoted or not, and read each piece's words with
// strings.Fields. It walks a code text readShell cannot follow, so such a
// command gets at least the reading it always had.
func (w *shellWalk) walkTokens(command string, unread bool, fn func(walkStep)) {
	segs := splitSegments(command)
	for i, seg := range segs {
		before := jointBefore(command, segs, i)
		if !w.prose {
			w.join(before)
		}
		w.cdpath = w.cdpath || mentionsName(seg.text, cdpathNames)
		tokens := strings.Fields(seg.text)
		if target, ok := parseCd(tokens); ok {
			if target == "-" {
				fn(walkStep{tokens: tokens, configEnv: w.configEnv, unread: unread})
			}
			after := jointBefore(command, segs, i+1)
			w.move(target, seg.text, before, after, w.prose || inText(command, seg.off))
		} else {
			w.noteGitEnv(tokens)
			fn(walkStep{tokens: tokens, dir: w.gitDir(tokens), configEnv: w.configEnv, unread: unread})
		}
		w.heredoc = w.heredoc || opensHeredoc(seg.text)
	}
}

// gitConfigEnv reports whether a variable can change where a push goes
// without the command naming another remote: the GIT_CONFIG family (a whole
// config file, or keys like remote.origin.pushurl), the ssh and proxy
// commands git runs to reach the remote, GIT_EXEC_PATH, which picks the
// helpers that do the push, and HOME and XDG_CONFIG_HOME, which say where
// the global config with its url rewrites lives.
func gitConfigEnv(name string) bool {
	switch name {
	case "HOME", "XDG_CONFIG_HOME", "GIT_PROXY_COMMAND", "GIT_EXEC_PATH":
		return true
	}
	return strings.HasPrefix(name, "GIT_CONFIG") || strings.HasPrefix(name, "GIT_SSH")
}

// assignsGitConfigEnv reports whether tok assigns a gitConfigEnv variable
// (`GIT_CONFIG_GLOBAL=/x`, as a prefix or after export).
func assignsGitConfigEnv(tok string) bool {
	name, _, _ := strings.Cut(tok, "=")
	return isAssignment(tok) && gitConfigEnv(strings.TrimSuffix(name, "+"))
}

// overridesConfig reports whether the words in front of a git or shell word
// may run it with another config: an assignment of a gitConfigEnv variable,
// sudo, doas or su (another user's home and config), or env with an option
// (`env -i` drops HOME, `env -u` unsets a variable).
func overridesConfig(pre []string) bool {
	afterEnv := false
	for _, tok := range pre {
		switch {
		case assignsGitConfigEnv(tok), tok == "sudo", tok == "doas", tok == "su":
			return true
		case tok == "env":
			afterEnv = true
		case afterEnv && strings.HasPrefix(tok, "-"):
			return true
		}
	}
	return false
}
