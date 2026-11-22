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
	// read fail-closed (walkUnread). bash32 marks one from a
	// `$(cat <<'EOF' ... EOF)` message read that way only because bash 3.2
	// may run part of it (bash32Cats): a denial there names the forms that
	// stay data (bash32Hinted).
	unread bool
	bash32 bool
	// targets are the command's redirection targets as tokens, heredoc
	// delimiters included. A command from the token walk keeps them in
	// tokens instead.
	targets []string
	// redirs are the operators of the command's redirections and
	// targetWords their targets with their parts, both in the order of
	// targets. A command from the token walk has neither.
	redirs      []redirOp
	targetWords []placedWord
	// group is the body whose output the redirections take, nil when the
	// command's own: a ( ) subshell's, or the { } group's or compound
	// command's the command closes (`}`, `fi`, `done`).
	group *shellGroup
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
	// words are the command's words as readShell read them, with their
	// parts, for a guard that judges what each word names on disk. A command
	// from the token walk has none.
	words []placedWord
	// cwd is the shell's own directory where the command runs ("" when only
	// a shell can tell). Unlike dir it ignores git's -C and GIT_DIR. cwdSure
	// is false when a cd that set it may not have run (shellWalk.dirSure).
	cwd     string
	cwdSure bool
	// pwd is what $PWD expands to in the command's words, "" when belt
	// cannot tell (pwdKnown).
	pwd string
	// feeder are the words of the command right in front of this one in a
	// pipeline, whose output it reads (`git ls-files | xargs rm`); nil when
	// no pipe feeds it or the command in front has no words, as a ( )
	// subshell has none. feederStderr marks a feeder whose stderr reaches
	// the pipe too (shellCmd.stderrPiped).
	feeder       []placedWord
	feederStderr bool
	// stdinRedirected marks a command that reads its standard input from a
	// redirection, a heredoc or a herestring, in place of any pipe.
	stdinRedirected bool
	// pipeText is the text of the commands in front of it in its pipeline
	// (frontText), and unreadSubst the bodies of the substitutions whose
	// output it runs as code and the walk could not read (unreadSubst).
	// docText is the text of the heredoc bodies the command reads.
	pipeText    string
	unreadSubst []string
	docText     string
	// id numbers the command among the steps of one walkCommands call,
	// from 1, and scriptOf is the id of the command whose -c script it is
	// part of, 0 for none. A command from the token walk has neither.
	id, scriptOf int
	// scripts are the interpreter scripts the command runs that the walk
	// found (feedsOf), read or not: an inline -c or -e script, a heredoc,
	// a here-string or a pipe it reads, a file the command wrote.
	scripts []feed
	// writes are the redirection targets the command writes to, as tokens
	// (writeTargets).
	writes []string
	// written returns what the commands of the walk wrote to the file at a
	// clean path so far (walkMemory.fileAt), for a guard that reads a file a
	// command makes. A command from the token walk has none.
	written func(p string) (script, bool)
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
	w := newShellWalk(command, cwd)
	w.walkRead(w.input, true, fn)
}

// walkCommandsBare is walkCommands that also reaches fn for a command of
// redirections alone (`> f`), which opens its targets and runs nothing, or
// of ( ) bodies and redirections (`( : ) > f`). Its step has no tokens and
// empty words; a ( ) body is its group.
func walkCommandsBare(command, cwd string, fn func(walkStep)) {
	w := newShellWalk(command, cwd)
	w.bare = true
	w.walkRead(w.input, true, fn)
}

// newShellWalk starts the walk of command from cwd.
func newShellWalk(command, cwd string) *shellWalk {
	return &shellWalk{
		dir: cwd, cdpath: os.Getenv("CDPATH") != "", chains: new(int), steps: new(int),
		mem: newWalkMemory(), outerSure: true, input: resolveLiteralVars(command),
	}
}

// walkRead walks one text. code says the shell runs it as commands; with
// code false it is a heredoc body a shell may run (walkProse), and one the
// reader cannot follow is walked the token way. Code nested past
// maxShellNesting is read fail-closed. A text that may set PWD makes every
// $PWD from there on unknown (rewritesPWD), and one that may cd or assign in
// this shell from inside a word (expandsInShell) does the same for every
// directory word expandShellDir places.
func (w *shellWalk) walkRead(text string, code bool, fn func(walkStep)) {
	w.pwdSet = w.pwdSet || rewritesPWD(text)
	w.cdHidden = w.cdHidden || expandsInShell(text)
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
// still reaches fn, marked unread. Such code may hold a
// `$(cat <<'EOF' ... EOF)` bash 3.2 ends in another place when it holds a
// $( ), so the walk around it reads the rest of its own text fail-closed
// too (shellWalk.unsynced).
func (w *shellWalk) walkUnread(text string, fn func(walkStep)) {
	w.unsynced = w.unsynced || strings.Contains(text, "$(")
	loose := strings.NewReplacer(
		"\\\n", "", `'`, "", `"`, "", `\`, "",
		"(", ";", ")", ";", "`", ";", "{", ";", "}", ";",
	).Replace(text)
	w.walkTokens(loose, true, fn)
}

// walkCmds walks the commands readShell read from text. Each command's
// heredoc bodies are judged with the command a pipe hands them to, if any.
// In a bare walk a command of redirections alone reaches fn too. The step
// of a command that closes a group carries that group (groupsOf).
// Once a command holds a `$(cat <<'EOF' ... EOF)` bash 3.2 may end in
// another place than belt (walkNested), a quote of its body may stay open
// into anything after it, so the rest of text from there is read
// fail-closed (walkUnread) in place of the commands after it, its steps
// marked walkStep.bash32.
func (w *shellWalk) walkCmds(text string, cmds []shellCmd, fn func(walkStep)) {
	groups := groupsOf(text, cmds)
	for i, c := range cmds {
		if !w.prose {
			w.join(c.before)
			w.joinChain(c.before)
		}
		afterMove := w.justMoved
		w.justMoved = false
		tail := w.walkNested(text, c, fn)
		if tokens := c.tokens(); len(tokens) > 0 {
			w.walkWords(text, c, tokens, w.pipeInto(cmds, i), withGroup(fn, groups[i]))
			if afterMove && leavesOnFail(c, tokens) {
				w.settleMove()
			}
		} else if w.bare && len(c.targets) > 0 {
			step := w.stepFor(c, []string{}, w.pipeInto(cmds, i), w.dir, false)
			step.words = []placedWord{}
			if len(c.subshells) > 0 {
				step.group = subshellGroup(text, c)
			}
			fn(step)
		}
		w.heredoc = w.heredoc || len(c.docs) > 0
		var reader *shellCmd
		if c.after == jointPipe && i+1 < len(cmds) {
			reader = &cmds[i+1]
		}
		w.walkProse(text, c, reader, fn)
		if tail >= 0 {
			w.unsynced = true
			w.walkUnread(text[tail:], markBash32(fn))
			return
		}
	}
}

// joinChain takes the separator in front of the next command: anything but
// && starts a new run of commands joined by && (walkStep.chain), and
// anything but && or | a new shellWalk.run.
func (w *shellWalk) joinChain(before joint) {
	if before != jointAnd || w.chain == 0 {
		*w.chains++
		w.chain = *w.chains
	}
	if before != jointAnd && before != jointPipe || w.run == 0 {
		*w.chains++
		w.run = *w.chains
	}
}

// walkNested walks what a command runs in a subshell, each from the
// directory the walk has reached. When the command may run the text a
// substitution prints (runsText), that text is code, so a heredoc body
// printed there is read as code too (shellWalk.feeds). In a plainGit
// commit, a message substitution that only prints a heredoc (messageCats)
// runs nothing and is not walked. A `$(cat <<'EOF' ... EOF)` bash 3.2 may
// run more of than cat (bash32Cats) is read fail-closed (walkUnread): bash
// 3.2 may run any part of its heredoc body, with quotes belt cannot follow.
// Both read a $( ) in the shell's whole input to its end (whole). Any other
// substitution is walked as the command it runs (piece.code): a backtick
// body with one level of escapes taken off, so cmd in the body echo \`cmd\`
// is walked too. It returns where the text bash 3.2 may read in another way
// than belt starts, -1 for nowhere: at the ) of a bash32Cats message it may
// end elsewhere, or at the end of a subshell or substitution whose own walk
// reached such a message (shellWalk.unsynced).
func (w *shellWalk) walkNested(text string, c shellCmd, fn func(walkStep)) (tail int) {
	feeds := w.feeds || runsText(c)
	var message map[span]bool
	if w.plainGit(c) {
		message = messageCats(text, c, w.whole(text))
	}
	bash32, tail := bash32Cats(text, c, w.whole(text))
	walk := func(at span, code string) {
		if message[at] {
			return
		}
		inner := w.child(w.dir)
		inner.feeds = feeds
		if bash32[at] {
			inner.walkUnread(text[at.from:at.to], markBash32(fn))
			return
		}
		inner.walkRead(code, true, fn)
		if inner.unsynced && (tail < 0 || at.to < tail) {
			tail = at.to
		}
	}
	for _, p := range c.pieces() {
		walk(p.span, p.code(text))
	}
	for _, s := range c.subshells {
		walk(s, text[s.from:s.to])
	}
	return tail
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
// those bodies are walked as code wherever the body goes. A body a plainGit
// call reads as its message file (readsGitMessage) is text through and
// through, so only its $( ) and backticks are walked. reader is the command
// a pipe after c hands its output to, nil when there is none.
func (w *shellWalk) walkProse(text string, c shellCmd, reader *shellCmd, fn func(walkStep)) {
	prose := w.prose
	w.prose = true
	runs := w.feeds || docRuns(c, reader)
	message := !runs && w.plainGit(c) && readsGitMessage(c)
	asProse := func(s walkStep) {
		s.prose = true
		fn(s)
	}
	for _, d := range c.docs {
		switch {
		case runs:
			w.walkRead(text[d.body.from:d.body.to], false, fn)
		case !message:
			w.walkTokens(text[d.body.from:d.body.to], false, asProse)
		}
		if d.quoted {
			continue
		}
		for _, code := range bodySubstitutions(text, d.body) {
			inner := w.child(w.dir)
			inner.feeds = runs
			inner.walkRead(code, true, fn)
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

// plainGit reports whether c is a git call whose message belt can vouch
// for: the word git itself comes first (no path, wrapper or assignment in
// front, so PATH and the command name are what they were), in top-level
// text no shell runs as printed output (depth, feeds), after no command that
// may have changed the shell unseen, such as a function, an alias, exec or a
// PATH change (cdHidden), and with no redirection but its heredocs on
// standard input (numbered) and no pipe or >( ) (runsText), so what git
// prints cannot reach a shell.
func (w *shellWalk) plainGit(c shellCmd) bool {
	tokens := c.tokens()
	return w.depth == 0 && !w.feeds && !w.cdHidden &&
		len(tokens) > 0 && tokens[0] == "git" &&
		len(c.targets) == len(c.docs) && !c.numbered && !runsText(c)
}

// plainArgs reports whether the shell passes every word of c after the
// first as one word, as written, so the options and operands belt reads
// are the ones git gets: literal text with no unquoted glob, brace, tilde
// or zsh =word character. Any other expansion ($E, {-e,-q}, "$(printf -e)")
// may turn an operand into an option. The token messageArgs reads must be
// that literal too: token keeps a word with a quote or $ in it as written,
// so '-em$' would read as an operand while git gets -e. A word in message,
// a -m value, is a value whatever it holds, and may also hold a $( )
// inside double quotes, which stays one word.
func plainArgs(c shellCmd, message map[int]bool) bool {
	for i := 1; i < len(c.words); i++ {
		if v, ok := c.words[i].literal(); !message[i] && (!ok || c.words[i].token() != v) {
			return false
		}
		for j, p := range c.words[i].parts {
			switch {
			case p.kind == partSubst && p.quoted && message[i]:
			case p.kind != partLit:
				return false
			case p.quoted:
			case strings.ContainsAny(p.text, "*?[]{}~#^") || (j == 0 && strings.HasPrefix(p.text, "=")):
				return false
			}
		}
	}
	return true
}

// readsGitMessage reports whether c is a git tag or git notes add with no
// global option but -C (plainSubcommand) whose -F or --file reads its
// standard input, with options belt knows leave the message alone
// (messageArgs) and every word as written (plainArgs), so its heredoc is
// only ever the message. Neither command
// touches the index or starts a hook. git commit and git merge refresh the
// index, and core.fsmonitor then runs a configured program with git's
// standard input, so their heredoc may still reach a shell.
func readsGitMessage(c shellCmd) bool {
	tokens := c.tokens()
	sub, _ := plainSubcommand(tokens)
	g, ok := segmentGitCommand(tokens, "")
	if !ok || sub != g.sub || (g.sub != "tag" && g.sub != "notes") || !plainArgs(c, nil) {
		return false
	}
	known, stdin := messageArgs(g)
	return known && stdin
}

// messageOptions are, per subcommand, the options known to read no standard
// input and to start no editor, prompt or hook that gets the message file:
// shorts are the one-letter forms that may share a word (-qa). -m, -F and
// their long forms are read by messageArgs itself. Any other option, -e,
// -p or --trailer among them, is not known, so the message keeps the walk.
var messageOptions = map[string]struct {
	shorts string
	longs  map[string]bool
}{
	"commit": {"qasn", wordSet(
		"--quiet --all --signoff --no-verify --allow-empty --allow-empty-message --amend --no-edit",
	)},
	"tag":   {"af", wordSet("--annotate --force")},
	"notes": {"f", wordSet("--force --allow-empty")},
}

// messageArgs reads the arguments of a git commit, tag or notes add as git
// does. known is true when every one is a messageOptions option, a -m or -F
// with its value, or an operand (a word with no leading -, or anything after
// a --). stdin is true when a -F or --file value names standard input. A
// value is skipped whatever it holds, so a -- or -F taken as one (-m -- -e,
// -m -F -) is read as the value it is.
func messageArgs(g gitInvocation) (known, stdin bool) {
	opts, ok := messageOptions[g.sub]
	args := g.args
	if g.sub == "notes" {
		ok = ok && len(args) > 0 && args[0] == "add"
		args = args[min(1, len(args)):]
	}
	if !ok {
		return false, false
	}
	for i := 0; i < len(args); i++ {
		a, file := args[i], ""
		switch {
		case a == "--":
			return true, stdin
		case a == "--message", a == "--file":
			i++
			if a == "--file" && i < len(args) {
				file = args[i]
			}
		case strings.HasPrefix(a, "--message="):
		case strings.HasPrefix(a, "--file="):
			file = strings.TrimPrefix(a, "--file=")
		case strings.HasPrefix(a, "--"):
			if !opts.longs[a] {
				return false, false
			}
		case len(a) > 1 && a[0] == '-':
			letter, value, takesNext, known := shortMessageWord(a[1:], opts.shorts)
			if !known {
				return false, false
			}
			if takesNext {
				i++
				if i < len(args) {
					value = args[i]
				}
			}
			if letter == 'F' {
				file = value
			}
		}
		stdin = stdin || (file != "" && namesStdin(file))
	}
	return true, stdin
}

// shortMessageWord reads one word of short options (without its -): each
// letter must be in shorts until an m or F (letter), which takes the rest of
// the word as its value, or the next word when nothing follows it
// (takesNext).
func shortMessageWord(letters, shorts string) (letter rune, value string, takesNext, known bool) {
	for i, r := range letters {
		switch {
		case r == 'm' || r == 'F':
			return r, letters[i+1:], i == len(letters)-1, true
		case !strings.ContainsRune(shorts, r):
			return 0, "", false, false
		}
	}
	return 0, "", false, true
}

// messageCats returns the substitutions in a git commit's message values
// that only print a heredoc the shell hands over as written, the
// `-m "$(cat <<'EOF' ... EOF)"` form of which bash 3.2 runs no more than
// cat (dataCat), when the commit's options leave the message alone
// (messageArgs), with every word as written (plainArgs). What they print is
// the message, so they run nothing but cat. eof says text is the shell's
// whole input (dataCat). The caller checks the commit is a plainGit call.
func messageCats(text string, c shellCmd, eof bool) map[span]bool {
	words, _, ok := commitMessageWords(c.tokens())
	g, isGit := segmentGitCommand(c.tokens(), "")
	message := map[int]bool{}
	for _, i := range words {
		message[i] = true
	}
	if known, _ := messageArgs(g); !ok || !isGit || !known || !plainArgs(c, message) {
		return nil
	}
	cats := map[span]bool{}
	for _, i := range words {
		for _, p := range c.words[i].parts {
			if p.kind != partSubst {
				continue
			}
			if _, ok := dataCat(text, p, eof); ok {
				cats[p.body] = true
			}
		}
	}
	return cats
}

// bash32Cats returns the substitutions in c's words and redirection targets
// that only print a heredoc in bash 5 and zsh (catBody) but of which bash
// 3.2 may run more than cat (dataCat refuses them): it can end the $( )
// early and run any part of the heredoc body, so walkNested reads it
// fail-closed. tail is the ) of the first of them after which bash 3.2 may
// read the rest of text in another way (bash32Refused) and run any of it,
// -1 for none. eof says text is the shell's whole input (dataCat).
func bash32Cats(text string, c shellCmd, eof bool) (cats map[span]bool, tail int) {
	cats, tail = map[span]bool{}, -1
	bash32Refused(text, c, eof, func(p wordPart, apart bool) {
		cats[p.body] = true
		if apart && (tail < 0 || p.body.to < tail) {
			tail = p.body.to
		}
	})
	return cats, tail
}

// markBash32 is fn for the steps of a message bash32Cats reads fail-closed:
// each is marked walkStep.bash32.
func markBash32(fn func(walkStep)) func(walkStep) {
	return func(s walkStep) {
		s.bash32 = true
		fn(s)
	}
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

// readsMessage reports whether docRuns reads a git command's heredoc as
// prose: git commit always, and the takesStdinMessage forms. Only a git tag
// or notes add body may skip the token walk too (readsGitMessage).
func readsMessage(g gitInvocation) bool {
	return g.sub == "commit" || takesStdinMessage(g)
}

// takesStdinMessage reports whether a git command may read its standard
// input as its message file: git commit, tag or merge, or git notes add,
// with -F or --file naming standard input.
func takesStdinMessage(g gitInvocation) bool {
	switch g.sub {
	case "commit", "tag", "merge":
		return readsStdinFile(g.args, "--file", "-F")
	case "notes":
		return len(g.args) > 0 && g.args[0] == "add" && readsStdinFile(g.args[1:], "--file", "-F")
	}
	return false
}

// readsStdinFile reports whether args name standard input as the file of
// the option long or its short form, as - or /dev/stdin: `-F -`, `-F-`,
// `--file -`, `--file=-`, `-F /dev/stdin`.
func readsStdinFile(args []string, long, short string) bool {
	for i, arg := range args {
		switch {
		case arg == short || arg == long:
			if i+1 < len(args) && namesStdin(args[i+1]) {
				return true
			}
		case strings.HasPrefix(arg, long+"="):
			if namesStdin(strings.TrimPrefix(arg, long+"=")) {
				return true
			}
		case strings.HasPrefix(arg, short) && !strings.HasPrefix(arg, "--"):
			if namesStdin(strings.TrimPrefix(arg, short)) {
				return true
			}
		}
	}
	return false
}

// namesStdin reports whether a file argument is standard input.
func namesStdin(file string) bool {
	return file == "-" || file == "/dev/stdin"
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
	words := tokens[runAt(tokens):]
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
	// prev is the command right in front of it, nil outside a pipeline, and
	// front every command in front of it in the pipeline.
	prev  *shellCmd
	front []shellCmd
}

// pipeInto returns what the pipeline in front of command i of cmds hands it.
func (w *shellWalk) pipeInto(cmds []shellCmd, i int) pipeIn {
	var in pipeIn
	if i > 0 && cmds[i].before == jointPipe {
		in.prev = &cmds[i-1]
	}
	for j := i - 1; j >= 0 && cmds[j+1].before == jointPipe; j-- {
		in.front = cmds[j:i]
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
// definition moves nothing, even one named cd, and the eval, script and
// feeds of its body are read where it stands (bodyOf). Code from a
// definition on may be a function's body, run with the call's arguments, so
// an eval or a -c there that runs one of them is opaque (afterFunction).
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
		w.justMoved = !w.prose
		return
	}
	w.cdHidden = w.cdHidden || (!w.prose && !keepsShellPlaced(defines, tokens))
	w.noteGitEnv(tokens)
	dir := w.gitDir(tokens)
	step := w.stepFor(c, tokens, in, dir, defines)
	step.pipeText, step.unreadSubst = frontText(text, in.front), w.unreadSubst(text, c, tokens)
	step.docText, step.writes = docBodies(text, c), writeTargets(text, c)
	step.written = w.mem.fileAt
	w.mem.noteAliasConfig(tokens)
	body, run := bodyOf(text, c, tokens)
	feeds := w.feedsOf(text, body, run, in, dir)
	opaque, walk := w.mem.claim(feeds)
	w.afterFunction = w.afterFunction || defines
	switch {
	case opaque.what != "":
	case w.positional && runsPositional(body, run):
		opaque = opacity{what: "a script argument the script runs as code", scoped: true}
	case w.afterFunction && runsPositionalText(body, run):
		opaque = opacity{what: "a function argument the function runs as code", scoped: true}
	}
	step.opaque = opaque
	step.scripts = slices.DeleteFunc(slices.Clone(feeds), func(f feed) bool { return !f.interp })
	fn(step)
	w.remember(text, body, run, in, dir)
	if at := inlineScriptAt(tokens); at >= 0 {
		inner := w.childFor(dir, tokens[:at])
		inner.positional = at+1 < len(tokens) || appendsItems(run)
		inner.script = step.id
		inner.walkRead(c.words[at].script(), true, fn)
	}
	if at := evalAt(run); at >= 0 {
		w.walkEval(body, run, at, fn)
	}
	if walk {
		w.walkFeeds(feeds, fn)
	}
}

// writeTargets returns the redirection targets of a command that it writes
// to (`>`, `>>`, `2>`, `&>`, `<>` and the rest), as tokens.
func writeTargets(text string, c shellCmd) []string {
	var out []string
	for _, t := range c.targets {
		if strings.Contains(redirectOp(text, t), ">") {
			out = append(out, t.token())
		}
	}
	return out
}

// bodyOf returns a command, its words and its tokens with a function
// definition's head dropped (functionBody), so the walk reads an eval, a
// script or a feed in the body where the function is defined, whether the
// command calls it or not (`f() { eval "$x"; }`). Any other command comes
// back whole.
func bodyOf(text string, c shellCmd, tokens []string) (shellCmd, []string) {
	run := functionBody(text, c, tokens)
	c.words = c.words[len(tokens)-len(run):]
	return c, run
}

// stepFor builds the step fn sees for one command that is no cd, run from
// git's directory dir.
func (w *shellWalk) stepFor(c shellCmd, tokens []string, in pipeIn, dir string, defines bool) walkStep {
	targets := make([]string, 0, len(c.targets))
	for _, t := range c.targets {
		targets = append(targets, t.token())
	}
	step := walkStep{
		tokens: tokens, dir: dir, configEnv: w.configEnv, targets: targets, defines: defines,
		redirs: c.redirs, targetWords: c.targets,
		pipedConfig: in.namesConfig, words: c.words, cwd: w.dir, cwdSure: w.dirSure(),
		stdinRedirected: c.stdinRedirected(), scriptOf: w.script,
	}
	*w.steps++
	step.id = *w.steps
	if w.pwdKnown() {
		step.pwd = w.dir
	}
	if !w.prose {
		step.before, step.after, step.chain = c.before, c.after, w.chain
	}
	if in.prev != nil {
		step.feeder, step.feederStderr = in.prev.words, in.prev.stderrPiped()
	}
	return step
}

// positionalRef is a reference to a script argument: $1 to $9, ${10}, $@
// or $*.
var positionalRef = regexp.MustCompile(`\$\{?[0-9@*]`)

// runsPositional reports whether a command runs a script argument as code:
// its command word is one, or eval or a shell's -c takes one as its text
// (runsPositionalText). The walk cannot see the argument from inside the
// script, so the text is unread.
func runsPositional(c shellCmd, tokens []string) bool {
	head := runAt(tokens)
	return head < len(c.words) && refersPositional(c.words[head]) || runsPositionalText(c, tokens)
}

// runsPositionalText reports whether eval or a shell's -c takes a script or
// function argument as its text (`sh -c 'eval "$1"' _ 'git push'`, `f() {
// eval "$1"; }`). A function's call shows its arguments as words, so one it
// runs as a command (`"$@"`) is read there; only text run as code hides.
func runsPositionalText(c shellCmd, tokens []string) bool {
	if at := evalAt(tokens); at >= 0 {
		return slices.ContainsFunc(c.words[at+1:], refersPositional)
	}
	at := inlineScriptAt(tokens)
	return at >= 0 && refersPositional(c.words[at])
}

// refersPositional reports whether a word refers to a script argument
// (positionalRef).
func refersPositional(w placedWord) bool {
	return positionalRef.MatchString(w.raw)
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

// walkEval walks the text eval runs (walkInShell). An eval that runs another
// eval reads the same words again, so a chain of evals that nests past
// maxShellNesting (`eval eval eval ... git push`) reads as unread text at
// once, instead of once per eval down to the limit.
func (w *shellWalk) walkEval(c shellCmd, tokens []string, at int, fn func(walkStep)) {
	script := make([]string, 0, len(c.words)-at)
	for _, word := range c.words[at+1:] {
		script = append(script, word.script())
	}
	text := strings.Join(script, " ")
	if w.depth+1+evalChain(tokens[at+1:]) <= maxShellNesting {
		w.walkInShell(tokens[:at], text, false, fn)
		return
	}
	inner := w.childFor(w.dir, tokens[:at])
	inner.walkUnread(text, fn)
	w.leaveShell(inner)
}

// evalChain counts the evals that start the command in tokens one inside
// another (evalAt), up to one past maxShellNesting: the words after the first
// eval of `eval eval git push` hold one.
func evalChain(tokens []string) int {
	n := 0
	for at := evalAt(tokens); at >= 0 && n <= maxShellNesting; at = evalAt(tokens) {
		n++
		tokens = tokens[at+1:]
	}
	return n
}

// walkInShell walks text this shell runs itself, behind the words pre (eval,
// source): the variables it sets stay set, and a directory it moves to is
// unknown here. args marks a source with arguments of its own, which the
// text reads as $1 and the rest.
func (w *shellWalk) walkInShell(pre []string, text string, args bool, fn func(walkStep)) {
	inner := w.childFor(w.dir, pre)
	inner.positional = inner.positional || args
	inner.walkRead(text, true, fn)
	w.leaveShell(inner)
}

// leaveShell keeps what a text this shell ran itself (walkInShell) left
// set: its variables, a move belt did not see (cdHidden), and an unknown
// directory where it moved.
func (w *shellWalk) leaveShell(inner *shellWalk) {
	w.gitEnv = w.gitEnv || inner.gitEnv
	w.configEnv = w.configEnv || inner.configEnv
	w.cdpath = w.cdpath || inner.cdpath
	w.cdHidden = w.cdHidden || inner.cdHidden
	if inner.dir != w.dir || inner.andOnly {
		w.dir, w.andOnly = "", false
	}
}

// child starts the walk of a subshell in dir. It inherits what the shell
// has set so far; what it sets itself stays inside it.
func (w *shellWalk) child(dir string) *shellWalk {
	return &shellWalk{
		dir: dir, cdpath: w.cdpath, gitEnv: w.gitEnv, configEnv: w.configEnv, heredoc: w.heredoc,
		feeds: w.feeds, depth: w.depth + 1, chains: w.chains, steps: w.steps, mem: w.mem, expanding: w.expanding,
		positional: w.positional, afterFunction: w.afterFunction, pwdSet: w.pwdSet, cdHidden: w.cdHidden,
		outerSure: w.dirSure(), bare: w.bare,
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

// evalAt returns the index of an eval word that starts the command, past
// what runAt reads past, or -1.
func evalAt(tokens []string) int {
	at := runAt(tokens)
	if at == len(tokens) || tokens[at] != "eval" {
		return -1
	}
	return at
}

// runAt returns the index of the word a command runs as its name,
// len(tokens) when none is left: past the reserved words that open it ({,
// !, if, then, else, while, do, function and its name) and the wrappers in
// front of its name, as the script guard reads them (unwrapCommand), and
// past the head of a case whose pattern stands in ( ), which the reader
// keeps as a subshell (`case x in (x) eval …`). Every lookup of what a
// command runs starts here, so an eval, a shell that reads its script or an
// xargs inside a group, a loop, an if or a function body is read as it is
// at the top level. walkWords drops a function definition's head first
// (functionBody).
func runAt(tokens []string) int {
	at := 0
	for {
		rest, _ := unwrapCommand(tokens[at:])
		at = len(tokens) - len(rest)
		if len(rest) < 3 || rest[0] != "case" || rest[2] != "in" {
			return at
		}
		at += 3
	}
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
