package guard

import (
	"path/filepath"
	"slices"
	"strings"
)

// The script guard reads the command through splitSegments, which cuts
// quoted text at every ; and |, and an inline script scans the text to the
// end of the command. So text that only names a denied command was denied
// as if it ran: an echo string holding `; bash -c '...'`, a commit message
// heredoc with a line that starts with xargs, or any message after an
// earlier `bash -c`. maskData blanks that text out before the checks run.
// It is data when all of this holds, as readShell reads the command:
//
//   - The command is a top-level echo, printf (without -v), git commit (or
//     git -C DIR commit) or a textSafeGh action, and its output is not piped
//     anywhere: no top-level command in the call is followed by a pipe.
//   - The call holds no compound command, group, function or exec at the
//     top level, and no >( ) process substitution, which could take the
//     output of a group or of everything after it.
//   - The masked piece is an argument word with no command in it, a heredoc
//     body the command reads, or the body of a `$(cat <<'EOF' ... EOF)`
//     argument. A heredoc body must have a quoted delimiter or hold no $,
//     backtick or backslash, so the shell expands nothing in it.
//   - Nothing left in the command once the data is blanked can run beside
//     the data command or after the command ends (runsLater): a background
//     &, a coproc, a trap, or a word that starts a job that outlives its
//     parent. Such a job could run the file an echo or a commit writes,
//     after the check that file never reached.
//
// A written file that is run, sourced, made executable or piped on is still
// checked against the raw command (scanTarget), and the bodies of every
// other substitution are scanned (scanSubstitutions).

// maskBlockers are the top-level command words that can carry an echo's
// output to something that runs it: compound commands and groups, whose
// output a later redirect takes as a whole, and exec, which can send all
// later output to a process.
var maskBlockers = wordSet(`
	{ } if then elif else fi for foreach while until do done case esac select
	repeat coproc function end exec
`)

// laterWords start code that can still run once the command naming them
// has returned, or when the shell exits: a coproc, a trap, a job disowned
// or put in a session of its own, a job a scheduler, a terminal multiplexer
// or a macOS app runs, and the interpreter calls that start a child without
// waiting for it.
var laterWords = wordSet(`
	coproc trap disown nohup setsid at batch crontab launchctl screen tmux
	daemonize open osascript Popen spawn fork posix_spawn detach detached
	start_new_session
`)

// maskData returns cmd with its data blanked out (see above): every byte of
// a data piece but its newlines becomes a space, so offsets, lines and the
// segments around the data stay where they were. cmd comes back unchanged
// when readShell cannot read it, the call has a shape that blocks masking,
// or what is left once the data is blanked could run it later (runsLater).
func maskData(cmd string) string {
	cmds, ok := readShell(cmd)
	if !ok || !maskable(cmds) {
		return cmd
	}
	b := []byte(cmd)
	for _, c := range cmds {
		for _, s := range dataSpans(cmd, c) {
			for i := s.from; i < s.to; i++ {
				if b[i] != '\n' {
					b[i] = ' '
				}
			}
		}
	}
	if masked := string(b); !runsLater(masked) {
		return masked
	}
	return cmd
}

// runsLater reports whether text, a command with its data blanked, holds a
// job that can run beside the rest of the command or after it ends: a
// background & anywhere, quoted text included (a -c script, an os.system
// string), or a laterWords word.
func runsLater(text string) bool {
	return backgrounds(text) || mentionsName(text, laterWords)
}

// backgrounds reports whether text holds an & that sends a job to the
// background: one that is not part of &&, |&, &>, >& or <&. zsh's &| and &!
// count.
func backgrounds(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] != '&' {
			continue
		}
		switch {
		case i+1 < len(text) && text[i+1] == '&':
			i++
		case i > 0 && strings.IndexByte("|<>", text[i-1]) >= 0:
		case i+1 < len(text) && text[i+1] == '>':
		default:
			return true
		}
	}
	return false
}

// maskable reports whether no top-level command can pass a data command's
// output on to something that runs it (maskBlockers, a pipe, a function
// definition, a >( ) target).
func maskable(cmds []shellCmd) bool {
	for _, c := range cmds {
		tokens := c.tokens()
		switch {
		case c.after == jointPipe, len(tokens) > 0 && maskBlockers[tokens[0]]:
			return false
		case len(c.subshells) > 0 && len(c.words) > 0:
			return false // f() { ...; } or a glob qualifier
		}
		for _, words := range [][]placedWord{c.words, c.targets} {
			if slices.ContainsFunc(
				words,
				func(w placedWord) bool { return strings.Contains(w.raw, ">(") },
			) {
				return false
			}
		}
	}
	return true
}

// dataSpans returns the pieces of one top-level command that are data.
func dataSpans(cmd string, c shellCmd) []span {
	words, docs, ok := dataWords(c.tokens())
	if !ok {
		return nil
	}
	var out []span
	for _, i := range words {
		out = append(out, wordData(cmd, c.words[i])...)
	}
	for _, d := range c.docs {
		if docs && unexpanded(cmd, d) {
			out = append(out, d.body)
		}
	}
	return out
}

// textFlags are the options of a carrier command whose value is message
// text, and those whose value names the file the message comes from, where
// - means standard input: the command's heredoc.
type textFlags struct{ text, file map[string]bool }

var (
	commitFlags = textFlags{text: wordSet("-m --message"), file: wordSet("-F --file")}
	ghFlags     = textFlags{
		text: wordSet("-t --title -b --body -n --notes -d --desc"),
		file: wordSet("-F --body-file --notes-file"),
	}
)

// dataWords returns the indexes of a data command's words that are data,
// and whether the heredoc it reads is data too. ok is false when the
// command is not a data command. echo and printf (printsWords) print every
// argument and read no heredoc. A git commit carries the values of its
// message flags and a -F - heredoc; a textSafeGh action carries its title,
// body and notes values and a -F - heredoc, or a gist's - file.
func dataWords(tokens []string) (words []int, docs, ok bool) {
	switch {
	case printsWords(tokens):
		for i := 1; i < len(tokens); i++ {
			words = append(words, i)
		}
		return words, true, true
	case len(tokens) > 0 && tokens[0] == "git":
		at := 1
		if len(tokens) > 3 && tokens[1] == "-C" {
			at = 3
		}
		if len(tokens) > at && tokens[at] == "commit" {
			words, docs = commitFlags.values(tokens, at+1)
			return words, docs, true
		}
	case len(tokens) > 2 && tokens[0] == "gh" && textSafeGh[tokens[1]][tokens[2]]:
		words, docs = ghFlags.values(tokens, 3)
		gistStdin := tokens[1] == "gist" && slices.Contains(tokens[3:], "-")
		return words, docs || gistStdin, true
	}
	return nil, false, false
}

// printsWords reports whether a command is echo, or printf without -v
// (which stores the text in a variable a later eval may run): the commands
// whose words are the text they print.
func printsWords(tokens []string) bool {
	switch {
	case len(tokens) == 0:
		return false
	case tokens[0] == "echo":
		return true
	}
	return tokens[0] == "printf" &&
		!slices.ContainsFunc(tokens[1:], func(t string) bool { return strings.HasPrefix(t, "-v") })
}

// values returns the indexes of the words in tokens[from:] that are a text
// flag's value, and whether a file flag names -, standard input.
func (f textFlags) values(tokens []string, from int) (words []int, stdin bool) {
	for i := from; i < len(tokens); i++ {
		flag, glued, value := f.split(tokens[i])
		switch {
		case f.text[flag] && glued:
			words = append(words, i)
		case f.file[flag] && glued:
			stdin = stdin || value == "-"
		case (f.text[flag] || f.file[flag]) && i+1 < len(tokens):
			i++
			if f.text[flag] {
				words = append(words, i)
			}
			stdin = stdin || (f.file[flag] && tokens[i] == "-")
		}
	}
	return words, stdin
}

// split reads one option word as git and gh parse it: --name=value, --name,
// or short flags, where the first one of f in a bundle takes the rest of
// the word as its value (-mmsg, -F-) or, when nothing follows it, the next
// word (-am msg).
func (f textFlags) split(arg string) (flag string, glued bool, value string) {
	if name, v, ok := strings.Cut(arg, "="); ok && strings.HasPrefix(arg, "--") {
		return name, true, v
	}
	if strings.HasPrefix(arg, "--") || !strings.HasPrefix(arg, "-") {
		return arg, false, ""
	}
	for j := 1; j < len(arg); j++ {
		if short := "-" + arg[j:j+1]; f.text[short] || f.file[short] {
			return short, j+1 < len(arg), arg[j+1:]
		}
	}
	return arg, false, ""
}

// wordData returns what is data in one argument word: the whole word when
// nothing in it runs a command, the bodies of its dataCat substitutions when
// those are all it runs, and nothing otherwise.
func wordData(cmd string, w placedWord) []span {
	var bodies []span
	for _, p := range w.parts {
		switch {
		case p.kind == partSubst:
			body, ok := dataCat(cmd, p.body)
			if !ok {
				return nil
			}
			bodies = append(bodies, body)
		case p.body.to > p.body.from:
			return nil // a process substitution, $((...)) or ${...}
		}
	}
	if len(bodies) == 0 {
		return []span{w.at}
	}
	return bodies
}

// dataCat reports whether a substitution only prints a heredoc the shell
// hands over unchanged, `$(cat <<'EOF' ... EOF)`, the form a commit message
// or a pull request body takes, and returns where that heredoc body lies.
func dataCat(cmd string, sub span) (span, bool) {
	cmds, ok := readShell(cmd[sub.from:sub.to])
	if !ok || len(cmds) != 1 {
		return span{}, false
	}
	c := cmds[0]
	if tokens := c.tokens(); len(tokens) != 1 || tokens[0] != "cat" ||
		len(c.docs) != 1 || len(c.targets) != 1 || len(c.subshells) > 0 ||
		len(c.comments) > 0 || len(c.pieces()) > 0 {
		return span{}, false
	}
	d := c.docs[0]
	d.body = span{sub.from + d.body.from, sub.from + d.body.to}
	if !unexpanded(cmd, d) {
		return span{}, false
	}
	return d.body, true
}

// unexpanded reports whether the shell hands a heredoc body over as
// written: its delimiter is quoted, or it holds no $, backtick or backslash.
// An odd body (a line that starts with the delimiter without being it) is
// never vouched for, since bash and zsh may end it at different lines.
func unexpanded(cmd string, d shellDoc) bool {
	return !d.odd && (d.quoted || !strings.ContainsAny(cmd[d.body.from:d.body.to], "$`\\"))
}

// scanSubstitutions scans the code the shell runs while it reads the
// command: the body of every command and process substitution outside
// single quotes, in a word, a redirection target or an unquoted heredoc
// body, those inside ( ) subshells included. A substitution runs where it
// stands, whatever command it is an argument of, so `echo "$(rm -rf x)"` runs
// rm. cmd is the masked command, so a dataCat body is already blank.
func scanSubstitutions(cmd string, patterns []deny) *Denial {
	for _, s := range substitutions(cmd, span{0, len(cmd)}) {
		if d := scanText(cmd[s.from:s.to], "the command substitution", patterns); d != nil {
			return d
		}
	}
	return nil
}

// substitutions returns the substitution bodies scanSubstitutions scans in
// cmd[within.from:within.to], as offsets into cmd. Text readShell cannot
// follow yields none; the other checks still read it.
func substitutions(cmd string, within span) []span {
	cmds, ok := readShell(cmd[within.from:within.to])
	if !ok {
		return nil
	}
	shift := func(s span) span { return span{s.from + within.from, s.to + within.from} }
	var out []span
	for _, c := range cmds {
		for _, p := range c.pieces() {
			if p := shift(p); p.from > 0 && (cmd[p.from-1] == '(' || cmd[p.from-1] == '`') {
				out = append(out, p) // not a ${...} parameter
			}
		}
		for _, sub := range c.subshells {
			out = append(out, substitutions(cmd, shift(sub))...)
		}
		for _, d := range c.docs {
			if !d.quoted {
				out = append(out, bodySubstitutions(cmd, shift(d.body))...)
			}
		}
	}
	return out
}

// bodySubstitutions returns the $( ) and backtick bodies in an unquoted
// heredoc body. One that does not close runs to the end of the body.
func bodySubstitutions(cmd string, body span) []span {
	var out []span
	for i := body.from; i < body.to; i++ {
		switch {
		case cmd[i] == '\\':
			i++
		case cmd[i] == '$' && i+1 < body.to && cmd[i+1] == '(':
			end, ok := matchParen(cmd[:body.to], i+1)
			if !ok {
				return append(out, span{i + 2, body.to})
			}
			out = append(out, span{i + 2, end})
			i = end
		case cmd[i] == '`':
			end := strings.IndexByte(cmd[i+1:body.to], '`')
			if end < 0 {
				return append(out, span{i + 1, body.to})
			}
			out = append(out, span{i + 1, i + 1 + end})
			i += end + 1
		}
	}
	return out
}

// scanPipedPrint scans what echo or printf prints into an interpreter that
// reads its script from standard input (`echo "..." | sh`), through any cat
// or tee between them. The printed text is the script.
func scanPipedPrint(cmd string, patterns []deny) *Denial {
	cmds, ok := readShell(cmd)
	if !ok {
		return nil
	}
	for i, c := range cmds {
		if c.before != jointPipe || !readsScriptFromStdin(c) {
			continue
		}
		j := i - 1
		for j > 0 && cmds[j].before == jointPipe && passesStdin(cmds[j]) {
			j--
		}
		words := cmds[j].words
		if !printsWords(cmds[j].tokens()) || len(words) < 2 {
			continue
		}
		text := cmd[words[1].at.from:words[len(words)-1].at.to]
		if d := scanText(text, "the text printed into an interpreter", patterns); d != nil {
			return d
		}
	}
	return nil
}

// readsScriptFromStdin reports whether a command is an interpreter with no
// inline script, file argument or heredoc, so it runs what it reads.
func readsScriptFromStdin(c shellCmd) bool {
	tokens := stripWrappers(c.tokens())
	if len(tokens) == 0 || len(c.docs) > 0 {
		return false
	}
	flags, ok := isInterpreter(filepath.Base(tokens[0]))
	if !ok {
		return false
	}
	inline, file := parseInterpreterArgs(tokens[1:], flags)
	return !inline && file == ""
}

// passesStdin reports whether a command in a pipeline hands its input on:
// cat or tee.
func passesStdin(c shellCmd) bool {
	tokens := c.tokens()
	return len(tokens) > 0 && (tokens[0] == "cat" || tokens[0] == "tee")
}
