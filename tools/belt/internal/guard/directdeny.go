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
// misses those forms. Only the commands the call runs itself
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
	for _, words := range directCommands(in.Command) {
		run, kind := unwrapCommand(words)
		if kind == runsAsText {
			// sudo -s, env -S and watch hand their words to a shell, which
			// splits a quoted string again.
			run = strings.Fields(strings.Join(run, " "))
		}
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
	}
	return nil
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

// directCommands returns the commands a Bash call runs itself, each as its
// words with the quotes dropped and a $'...' string decoded (ansiCWords):
// every simple command readShell reads at the top level, after a
// background & included, and inside a ( ) subshell. A heredoc body, a
// comment, a quoted string and a substitution are not among them. When
// readShell cannot read the call, the whole lines before the one it cannot
// follow are read so all the same, since bash runs them before it meets the
// error, and the rest is read by unreadCommands.
func directCommands(cmd string) [][]string {
	cmds, readTo, ok := readShellLines(cmd)
	if ok {
		return shellCommands(cmd, cmds)
	}
	var out [][]string
	if head, ok := readShell(cmd[:readTo]); ok {
		out = shellCommands(cmd[:readTo], head)
	} else {
		readTo = 0
	}
	return append(out, unreadCommands(cmd[readTo:])...)
}

// shellCommands returns the commands readShell read from src, the bodies of
// their ( ) subshells included (directCommands).
func shellCommands(src string, cmds []shellCmd) [][]string {
	var out [][]string
	for _, c := range cmds {
		if len(c.words) > 0 {
			out = append(out, ansiCWords(c.tokens()))
		}
		for _, sub := range c.subshells {
			out = append(out, directCommands(src[sub.from:sub.to])...)
		}
	}
	return out
}

// unreadCommands reads text readShell cannot follow: the segments of the
// masked text (splitSegments over maskData), cut again at every operator
// and parenthesis (lineCommands). Each command is read as written, where a
// phrase reads a quote as a space (`-e"delete from t"`), and again as the
// shell passes its words (unquotedWords). So text the reader cannot follow
// reads as more commands, not fewer.
func unreadCommands(text string) [][]string {
	var out [][]string
	for _, seg := range splitSegments(maskData(text)) {
		for _, words := range lineCommands(seg.text) {
			out = append(out, words)
			if plain := unquotedWords(words); len(plain) > 0 && !slices.Equal(plain, words) {
				out = append(out, plain)
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
