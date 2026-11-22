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
//     argument of which bash 3.2 runs no more than cat (bash32Closes), or
//     none at all (bash32Unclosed). A heredoc body must have a quoted
//     delimiter or hold no $, backtick or backslash, so the shell expands
//     nothing in it.
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
// cmd is the shell's whole input (dataCat).
func maskData(cmd string) string {
	return maskWith(cmd, func(cmd string, p wordPart) (span, bool) { return dataCat(cmd, p, true) })
}

// maskDataBash5 is maskData for a shell that ends a `$(cat <<'EOF' ... EOF)`
// where belt does, as bash 5 and zsh do: it masks every catBody, whatever
// bash 3.2 would run (bash32Closes). It only tells a denial that comes from
// bash 3.2 alone (ScriptDenyList.check).
func maskDataBash5(cmd string) string {
	return maskWith(cmd, catBody)
}

// catReader returns where the heredoc body of a substitution that only
// prints it lies (dataCat, catBody).
type catReader func(cmd string, p wordPart) (span, bool)

// maskWith is maskData with cat reading the `$(cat <<'EOF' ... EOF)`
// arguments.
func maskWith(cmd string, cat catReader) string {
	cmds, ok := readShell(cmd)
	if !ok || !maskable(cmds) {
		return cmd
	}
	b := []byte(cmd)
	for _, c := range cmds {
		for _, s := range dataSpans(cmd, c, cat) {
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
func dataSpans(cmd string, c shellCmd, cat catReader) []span {
	words, docs, ok := dataWords(c.tokens())
	if !ok {
		return nil
	}
	var out []span
	for _, i := range words {
		out = append(out, wordData(cmd, c.words[i], cat)...)
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
		return commitMessageWords(tokens)
	case len(tokens) > 2 && tokens[0] == "gh" && textSafeGh[tokens[1]][tokens[2]]:
		words, docs = ghFlags.values(tokens, 3)
		gistStdin := tokens[1] == "gist" && slices.Contains(tokens[3:], "-")
		return words, docs || gistStdin, true
	}
	return nil, false, false
}

// commitMessageWords returns the indexes of the words a git commit (or git
// -C DIR commit) takes as its message, and whether -F - names its heredoc
// as the message file. ok is false for any other command.
func commitMessageWords(tokens []string) (words []int, stdin, ok bool) {
	sub, at := plainSubcommand(tokens)
	if sub != "commit" {
		return nil, false, false
	}
	words, stdin = commitFlags.values(tokens, at+1)
	return words, stdin, true
}

// plainSubcommand returns the subcommand of a git call with nothing but
// -C DIR in front of it, and its index, or "" for any other command. A
// global option such as -c, -p or --exec-path may bring in a config value,
// a pager or a program the message checks cannot vouch for.
func plainSubcommand(tokens []string) (sub string, at int) {
	at = 1
	if len(tokens) > 3 && tokens[1] == "-C" {
		at = 3
	}
	if len(tokens) <= at || tokens[0] != "git" {
		return "", 0
	}
	return tokens[at], at
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
// nothing in it runs a command, the bodies of its cat substitutions when
// those are all it runs, and nothing otherwise.
func wordData(cmd string, w placedWord, cat catReader) []span {
	var bodies []span
	for _, p := range w.parts {
		switch {
		case p.kind == partSubst:
			body, ok := cat(cmd, p)
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
// or a pull request body takes, and returns where that heredoc body lies
// (catBody). bash 3.2 must run no more of a $( ) than cat (bash32Closes): it
// knows no heredoc there, so a ) in the body can end it early, in its parse
// step or in its expansion, and run part of the body. eof says cmd runs to
// the end of the shell's input, so a $( ) bash 3.2's parse step never
// closes there is a syntax error that runs nothing (bash32Unclosed), and the
// body passes too. A backtick substitution ends at its first backtick no
// backslash quotes, in bash 3.2 as in belt.
func dataCat(cmd string, p wordPart, eof bool) (span, bool) {
	body, ok := catBody(cmd, p)
	if !ok || backtickBody(cmd, p.body) {
		return body, ok
	}
	if !bash32Closes(cmd[p.body.from:p.body.to], p.quoted) && (!eof || !bash32Unclosed(cmd, p.body, p.quoted)) {
		return span{}, false
	}
	return body, true
}

// catBody is dataCat for a shell that ends a $( ) where belt does, as bash 5
// and zsh do. A backtick body must hold no backslash, since every shell
// takes backslashes off it before it reads it.
func catBody(cmd string, p wordPart) (span, bool) {
	sub := p.body
	if backtickBody(cmd, sub) && strings.Contains(cmd[sub.from:sub.to], `\`) {
		return span{}, false
	}
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

// backtickBody reports whether sub is the body of a backtick substitution.
func backtickBody(cmd string, sub span) bool {
	return sub.from > 0 && cmd[sub.from-1] == '`'
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
		if d := scanText(s, "the command substitution", patterns); d != nil {
			return d
		}
	}
	return nil
}

// substitutions returns the code of the substitutions scanSubstitutions
// scans in cmd[within.from:within.to]: each body as written, but a
// backtick body with one level of escapes taken off, in a word (piece.code)
// as in an unquoted heredoc body (bodySubstitutions). Text readShell cannot
// follow yields none; the other checks still read it.
func substitutions(cmd string, within span) []string {
	text := cmd[within.from:within.to]
	cmds, ok := readShell(text)
	if !ok {
		return nil
	}
	shift := func(s span) span { return span{s.from + within.from, s.to + within.from} }
	var out []string
	for _, c := range cmds {
		for _, p := range c.pieces() {
			if p.from > 0 && (text[p.from-1] == '(' || text[p.from-1] == '`') {
				out = append(out, p.code(text)) // not a ${...} parameter
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

// bodySubstitutions returns the code of the $( ) and backtick substitutions
// in an unquoted heredoc body. Its backslash-newlines are dropped first
// (joinedBody), so `$\` at a line end
// and `(cmd)` on the next line is a substitution. A backtick body closes at
// the first backtick no backslash quotes and comes back with one level of
// escapes taken off (backtickCode), so the walk reads a nested \`cmd\` in
// it as a substitution of its own. A \" keeps its backslash there, as in
// an unquoted word: a body is not a double-quoted string. One that does not
// close runs to the end of the body.
func bodySubstitutions(cmd string, body span) []string {
	text := joinedBody(cmd[body.from:body.to])
	var out []string
	for i := 0; i < len(text); i++ {
		switch {
		case text[i] == '\\':
			i++
		case text[i] == '$' && i+1 < len(text) && text[i+1] == '(':
			end, ok := matchParen(text, i+1)
			if !ok {
				return append(out, text[i+2:])
			}
			out = append(out, text[i+2:end])
			i = end
		case text[i] == '`':
			end := backtickEnd(text, i+1)
			out = append(out, backtickCode(text[i+1:end], false))
			i = end
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
