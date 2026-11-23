package guard

import (
	"slices"
	"strings"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// DirectDenyListID is the direct-deny-list guard's id.
const DirectDenyListID = "direct-deny-list"

// askUserToRun ends a denial for an entry of the Claude settings deny list.
// direct-deny-list denies such a command in every form the agent can write,
// so the advice is to hand it to the user, not to rewrite it.
const askUserToRun = "Ask the user to run it; they can type ! <command> in the prompt."

// DirectDenyList denies a Bash call whose own commands run an entry of the
// Claude Code settings deny list (permissions.deny `Bash(...)` entries,
// cfg.ClaudeDeny), read on argv the way script-deny-list reads it
// (denyentry.go): past global flags, wrapper options, launchers and
// quoting, so `kubectl -n prod delete`, `sudo kubectl delete`, `\kubectl
// delete` and `kubectl exec pod -- kubectl delete` all count. The
// permission system's own match reads the entry as a text prefix, which
// misses those forms. For a head an entry with a phrase names (`psql * DROP
// *`), the SQL it reads on standard input counts as one more argument: a
// heredoc, a here-string, a `<` file or a printer piped in, on the command
// itself, on a group or ( ) around it, or left by an exec (denystdin.go).
// Only the commands the call runs itself
// are read (directCommands); a -c string, eval, a substitution, an alias,
// the body of a `f() { …; }` function and a script file stay with
// script-deny-list. A `function f { …; }` body reads as top-level commands,
// since unwrapCommand reads past `function f` as an opener. extra_patterns
// stays with script-deny-list too: its entries are ask entries in the
// settings, and a direct call should still get the ask.
type DirectDenyList struct {
	cfg config.Config
	// emit sends the soft-mode warning to the events service. Injectable for
	// tests.
	emit emitFunc
	// stdinRoot is the folder the standard input reads take for the root of
	// the file system (inputReader), "" for the real one. Injectable for
	// tests, so a test reads no file of the host.
	stdinRoot string
}

// NewDirectDenyList builds the guard with the real events backend.
func NewDirectDenyList(cfg config.Config) *DirectDenyList {
	return &DirectDenyList{cfg: cfg, emit: notify.EmitEventSync}
}

func (g *DirectDenyList) ID() string    { return DirectDenyListID }
func (g *DirectDenyList) Event() string { return EventBash }

// Check denies the first command that runs a settings deny entry. In soft
// mode the denial becomes a warn event and the command proceeds to the
// permission system's own check.
func (g *DirectDenyList) Check(in Input) *Denial {
	d := g.check(in)
	if d == nil {
		return nil
	}
	if g.cfg.Guards[DirectDenyListID].Soft() {
		g.emit("belt", "warn", "direct-deny-list (soft)",
			d.Reason+" Command proceeding (soft mode).",
			map[string]string{"guard": DirectDenyListID})
		return nil
	}
	return d
}

func (g *DirectDenyList) check(in Input) *Denial {
	if in.Command == "" {
		return nil
	}
	patterns := settingsDenies(g.cfg.ClaudeDeny)
	if len(patterns) == 0 {
		return nil
	}
	tools := phraseTools(patterns)
	input := newInputReader(g.stdinRoot)
	for _, c := range directCommands(in.Command, in.Cwd) {
		run := commandRun(c.words)
		if d := leadingDeny(run, patterns); d != nil {
			return Reasonf(
				DirectDenyListID,
				"this runs %q from the Bash deny list (read past global flags and wrappers). "+
					"It can delete or change state that may not come back, so every form of it is denied: "+
					"a script, a wrapper or an alias too. %s",
				d.pattern,
				askUserToRun,
			)
		}
		var hit *inputHit
		if c.in != nil {
			hit = input.check(*c.in, run, patterns, tools)
		} else {
			hit = unreadHit(run, patterns, tools)
		}
		if hit != nil {
			return hit.directReason()
		}
	}
	return nil
}

// commandRun returns the words a command runs once what opens it and its
// wrappers are read past (unwrapCommand). sudo -s, env -S and watch hand
// their words to a shell, which splits a quoted string again, so behind
// them the words are split again too.
func commandRun(words []string) []string {
	run, kind := unwrapCommand(words)
	if kind == runsAsText {
		run = strings.Fields(strings.Join(run, " "))
	}
	return run
}

// settingsDenies compiles the settings deny entries, each marked as one
// (deny.settings).
func settingsDenies(entries []string) []deny {
	patterns := compilePatterns(uniquePatterns(entries))
	for i := range patterns {
		patterns[i].settings = true
	}
	return patterns
}

// directCmd is one command a Bash call runs itself (directCommands): its
// words, and where it reads its standard input from (inputAt), nil for a
// command from text readShell cannot follow.
type directCmd struct {
	words []string
	in    *inputAt
}

// directCommands returns the commands a Bash call run from cwd runs itself,
// each as its words with the quotes dropped and a $'...' string decoded
// (ansiCWords): every simple command readShell reads at the top level,
// after a background & included, and inside a ( ) subshell. A heredoc body,
// a comment, a quoted string and a substitution are not among them. When
// readShell cannot read the call, the whole lines before the one it cannot
// follow are read so all the same, since bash runs them before it meets the
// error, and the rest is read by unreadCommands. Each command read by
// readShell carries the directory it runs in, followed past the cd, pushd
// and popd before it the way the git guards' walk follows them.
func directCommands(cmd, cwd string) []directCmd {
	return newShellWalk("", cwd).directIn(cmd, nil)
}

// directIn is directCommands for one text the walk w places: the call, or
// the ( ) body of outer, whose standard input every command in the body
// reads too.
func (w *shellWalk) directIn(cmd string, outer *inputAt) []directCmd {
	cmds, readTo, ok := readShellLines(cmd)
	if ok {
		return w.shellCommands(cmd, cmds, outer)
	}
	var out []directCmd
	if head, ok := readShell(cmd[:readTo]); ok {
		out = w.shellCommands(cmd[:readTo], head, outer)
	} else {
		readTo = 0
	}
	return append(out, unreadCommands(cmd[readTo:])...)
}

// shellCommands returns the commands readShell read from src, the bodies of
// their ( ) subshells included (directCommands). A body starts where the
// command it is in runs, and a cd or an exec inside it moves nothing
// outside. Each command reads what the groups it is in and an exec before it
// give it too (stdinWalk).
func (w *shellWalk) shellCommands(src string, cmds []shellCmd, outer *inputAt) []directCmd {
	w.placeText(src)
	in := newStdinWalk(src, cmds, outer, outer != nil)
	var out []directCmd
	for i, c := range cmds {
		where, afterMove := w.enter(c)
		at := in.at(i, where)
		if len(c.words) > 0 {
			out = append(out, directCmd{words: ansiCWords(c.tokens()), in: at})
		}
		for _, sub := range c.subshells {
			out = append(out, w.child(where.dir).directIn(src[sub.from:sub.to], at)...)
		}
		in.leave(i, at)
		w.leave(src, c, afterMove)
	}
	return out
}

// unreadCommands reads text readShell cannot follow: the segments of the
// masked text (splitSegments over maskData), cut again at every operator
// and parenthesis (lineCommands). Each command is read as written, where a
// phrase reads a quote as a space (`-e"delete from t"`), and again as the
// shell passes its words (unquotedWords). So text the reader cannot follow
// reads as more commands, not fewer.
func unreadCommands(text string) []directCmd {
	var out []directCmd
	for _, seg := range splitSegments(maskData(text, nil)) {
		for _, words := range lineCommands(seg.text) {
			out = append(out, directCmd{words: words})
			if plain := unquotedWords(words); len(plain) > 0 && !slices.Equal(plain, words) {
				out = append(out, directCmd{words: plain})
			}
		}
	}
	return out
}

// unquotedWords returns words with each one's $'...' strings decoded and every
// backslash and quote inside it dropped, as the shell drops them
// (`kube\ctl`, `D"ELETE"`). A word left empty is dropped.
func unquotedWords(words []string) []string {
	var out []string
	for _, w := range words {
		if p := unquoteWord(ansiCWord(w)); p != "" {
			out = append(out, p)
		}
	}
	return out
}
