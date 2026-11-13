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
	// defines marks a command that defines a shell function
	// (definesFunction), which may stand in for git or any later command.
	defines bool
	// opaque names text the command runs that the walk could not read and
	// that may hold a push (walkMemory.claim): a git alias body, text printed
	// into a shell, an interpreter script that names a push.
	opaque opacity
	// pipedConfig marks a command a pipeline hands the output of a command
	// that names a git config file to (pipeInto), as xargs takes file names.
	pipedConfig bool
	// before and after are the joints around the command. chain names the
	// run of commands joined by && it belongs to in its own shell, so a later
	// command with the same chain runs only if this one succeeded. A command
	// from the token walk or a heredoc body has chain 0, in no run.
	before, after joint
	chain         int
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
// with a space oddSpace reports, is walked the token way (walkTokens). It
// also reads what a command hands a shell to run beyond its own words
// (feedsOf): a git alias body, text printed into a shell, a file or a
// variable the command filled with text.
func walkCommands(command, cwd string, fn func(walkStep)) {
	w := shellWalk{
		dir: cwd, cdpath: os.Getenv("CDPATH") != "", chains: new(int), mem: newWalkMemory(),
	}
	w.walkRead(resolveLiteralVars(command), true, fn)
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

// walkCmds walks the commands readShell read from text. Each command's
// heredoc bodies are judged with the command a pipe hands them to, if any.
func (w *shellWalk) walkCmds(text string, cmds []shellCmd, fn func(walkStep)) {
	for i, c := range cmds {
		if !w.prose {
			w.join(c.before)
			w.joinChain(c.before)
		}
		w.walkNested(text, c, fn)
		if tokens := c.tokens(); len(tokens) > 0 {
			w.walkWords(text, c, tokens, w.pipeInto(cmds, i), fn)
		}
		w.heredoc = w.heredoc || len(c.docs) > 0
		var reader *shellCmd
		if c.after == jointPipe && i+1 < len(cmds) {
			reader = &cmds[i+1]
		}
		w.walkProse(text, c, reader, fn)
	}
}

// joinChain takes the separator in front of the next command: anything but
// && starts a new run of commands joined by && (walkStep.chain).
func (w *shellWalk) joinChain(before joint) {
	if before != jointAnd || w.chain == 0 {
		*w.chains++
		w.chain = *w.chains
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
// those bodies are walked as code wherever the body goes. reader is the
// command a pipe after c hands its output to, nil when there is none.
func (w *shellWalk) walkProse(text string, c shellCmd, reader *shellCmd, fn func(walkStep)) {
	prose := w.prose
	w.prose = true
	runs := w.feeds || docRuns(c, reader)
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
// every command but cat, tee, head, tail, a git command that reads them as
// its message (readsMessage) and a gh pr, issue, release, gist or api call,
// and one of those too when a process substitution may hand what it prints
// to something that runs it. A pipe may too, unless it runs from cat to
// reader, a gh call that reads its standard input as body text (readsBody).
func docRuns(c shellCmd, reader *shellCmd) bool {
	words := stripWrappers(c.tokens())
	switch {
	case len(words) == 0 || handsOn(c):
		return true
	case c.after == jointPipe:
		return filepath.Base(words[0]) != "cat" || reader == nil || !readsBody(*reader)
	}
	switch filepath.Base(words[0]) {
	case "cat", "tee", "head", "tail":
		return false
	case "gh":
		return len(words) < 2 || !ghTextGroups[words[1]]
	case "git":
		g, ok := segmentGitCommand(words, "")
		return !ok || !readsMessage(g)
	}
	return true
}

// ghTextGroups are the gh command groups whose heredoc input is a body, a
// note or a request, never a script.
var ghTextGroups = wordSet("pr issue release gist api")

// ghBodyReaders are the gh subcommands that take their body from standard
// input with --body-file - or -F -.
var ghBodyReaders = wordSet("pr/create pr/comment pr/edit issue/create issue/comment")

// readsBody reports whether a command is a ghBodyReaders call that reads its
// body from standard input and pipes nothing on, so the text a pipe hands it
// is a body no shell runs.
func readsBody(c shellCmd) bool {
	words := stripWrappers(c.tokens())
	if len(words) < 3 || filepath.Base(words[0]) != "gh" || c.after == jointPipe || handsOn(c) {
		return false
	}
	return ghBodyReaders[words[1]+"/"+words[2]] && readsStdinFile(words[3:], "--body-file", "-F")
}

// readsMessage reports whether a git command reads a heredoc on its standard
// input as a message: git commit always, and git tag, git merge and git
// notes add when -F - or --file=- names standard input as the message file.
func readsMessage(g gitInvocation) bool {
	switch g.sub {
	case "commit":
		return true
	case "tag", "merge":
		return readsStdinFile(g.args, "--file", "-F")
	case "notes":
		return len(g.args) > 0 && g.args[0] == "add" && readsStdinFile(g.args[1:], "--file", "-F")
	}
	return false
}

// readsStdinFile reports whether args name standard input as the file of
// the option long or its short form: `-F -`, `-F-`, `--file -`, `--file=-`.
func readsStdinFile(args []string, long, short string) bool {
	for i, arg := range args {
		switch {
		case arg == short+"-", arg == long+"=-":
			return true
		case (arg == short || arg == long) && i+1 < len(args) && args[i+1] == "-":
			return true
		}
	}
	return false
}

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

// pipeIn is what the pipeline in front of a command hands it.
type pipeIn struct {
	// from is the command whose output reaches it, past any cat or tee
	// (passesStdin) between them; nil outside a pipeline.
	from *shellCmd
	// namesConfig marks a pipeline in which a command in front of it names a
	// git config file (gitConfigFile).
	namesConfig bool
}

// pipeInto returns what the pipeline in front of command i of cmds hands it.
func (w *shellWalk) pipeInto(cmds []shellCmd, i int) pipeIn {
	var in pipeIn
	for j := i - 1; j >= 0 && cmds[j+1].before == jointPipe; j-- {
		in.namesConfig = in.namesConfig || namesGitConfig(cmds[j], w.dir)
		if in.from == nil && (!passesStdin(cmds[j]) || cmds[j].before != jointPipe) {
			in.from = &cmds[j]
		}
	}
	return in
}

// namesGitConfig reports whether a command's words or redirections name a
// git config file (gitConfigFile).
func namesGitConfig(c shellCmd, dir string) bool {
	names := func(w placedWord) bool { return gitConfigFile(w.token(), dir) }
	return slices.ContainsFunc(c.words, names) || slices.ContainsFunc(c.targets, names)
}

// walkWords handles one command's words, read from text: a cd moves the
// walk, anything else reaches fn, and a shell -c script or an eval text is
// walked in turn, then what the command feeds a shell (feedsOf). A function
// definition moves nothing, even one named cd.
func (w *shellWalk) walkWords(
	text string,
	c shellCmd,
	tokens []string,
	in pipeIn,
	fn func(walkStep),
) {
	w.cdpath = w.cdpath || mentionsName(strings.Join(tokens, " "), cdpathNames)
	defines := definesFunction(text, c, tokens)
	if target, ok := parseCd(tokens); ok && !defines {
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
	step := walkStep{
		tokens: tokens, dir: dir, configEnv: w.configEnv, targets: targets, defines: defines,
		pipedConfig: in.namesConfig,
	}
	if !w.prose {
		step.before, step.after, step.chain = c.before, c.after, w.chain
	}
	w.mem.noteAliasConfig(tokens)
	feeds := w.feedsOf(text, c, tokens, in, dir)
	opaque, walk := w.mem.claim(feeds)
	if opaque.what == "" && w.positional && runsPositional(c, tokens) {
		opaque = opacity{what: "a script argument the script runs as code", scoped: true}
	}
	step.opaque = opaque
	fn(step)
	w.remember(text, c, tokens, in, dir)
	if at := inlineScriptAt(tokens); at >= 0 {
		inner := w.childFor(dir, tokens[:at])
		inner.positional = at+1 < len(tokens)
		inner.walkRead(c.words[at].script(), true, fn)
	}
	if at := evalAt(tokens); at >= 0 {
		w.walkEval(c, tokens, at, fn)
	}
	if walk {
		w.walkFeeds(feeds, fn)
	}
}

// positionalRef is a reference to a script argument: $1 to $9, ${10}, $@
// or $*.
var positionalRef = regexp.MustCompile(`\$\{?[0-9@*]`)

// runsPositional reports whether a command runs a script argument as code:
// its command word is one, or eval or a shell's -c takes one as its text
// (`sh -c 'eval "$1"' _ 'git push'`). The walk cannot see the argument from
// inside the script, so the text is unread.
func runsPositional(c shellCmd, tokens []string) bool {
	refers := func(w placedWord) bool { return positionalRef.MatchString(w.raw) }
	head := len(tokens) - len(stripWrappers(tokens))
	switch {
	case head < len(c.words) && refers(c.words[head]):
		return true
	case evalAt(tokens) >= 0:
		return slices.ContainsFunc(c.words[evalAt(tokens)+1:], refers)
	}
	at := inlineScriptAt(tokens)
	return at >= 0 && refers(c.words[at])
}

// definesFunction reports whether a command, read from text, defines a shell
// function: a blank ( ) among its words (`name() { ...; }`, `name ( ) ...`,
// zsh's `a b () ...` and anonymous `() { ...; }`), or the function keyword
// where the command starts (`function name { ...; }`).
func definesFunction(text string, c shellCmd, tokens []string) bool {
	if slices.ContainsFunc(c.subshells, func(s span) bool {
		return strings.TrimSpace(text[s.from:s.to]) == ""
	}) {
		return true
	}
	i := slices.IndexFunc(tokens, func(t string) bool { return !functionLeads[t] })
	return i >= 0 && tokens[i] == "function"
}

// functionLeads are the reserved words that may stand in front of a function
// definition in one command (`if x; then function f { ...; }; fi`).
var functionLeads = wordSet("! { if then elif else while until do time")

// walkEval walks the text eval runs (walkInShell).
func (w *shellWalk) walkEval(c shellCmd, tokens []string, at int, fn func(walkStep)) {
	script := make([]string, 0, len(c.words)-at)
	for _, word := range c.words[at+1:] {
		script = append(script, word.script())
	}
	w.walkInShell(tokens[:at], strings.Join(script, " "), false, fn)
}

// walkInShell walks text this shell runs itself, behind the words pre (eval,
// source): the variables it sets stay set, and a directory it moves to is
// unknown here. args marks a source with arguments of its own, which the
// text reads as $1 and the rest.
func (w *shellWalk) walkInShell(pre []string, text string, args bool, fn func(walkStep)) {
	inner := w.childFor(w.dir, pre)
	inner.positional = inner.positional || args
	inner.walkRead(text, true, fn)
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
		feeds: w.feeds, depth: w.depth + 1, chains: w.chains, mem: w.mem, expanding: w.expanding,
		positional: w.positional,
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
			w.mem.noteAliasConfig(tokens)
			fn(walkStep{
				tokens: tokens, dir: w.gitDir(tokens), configEnv: w.configEnv, unread: unread,
				opaque: opacity{what: w.tokenAlias(tokens)},
			})
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
