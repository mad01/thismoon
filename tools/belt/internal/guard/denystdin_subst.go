package guard

import (
	"fmt"
	"slices"
	"strings"
)

// This file reads the text that reaches a phrase head through a $( ) or
// backtick substitution, and the text an xargs hands the command it runs.
// A substitution that prints a file (`$(cat f)`, `$(< f)`, `$(head …)`,
// `$(tail …)`, `$(tac …)`) reads as that file, read as a `<` file is: a
// regular file alone, placed where the command runs, within the size caps,
// and a file belt cannot read or place denies. One that prints literal text
// (`$(echo …)`, `$(printf …)`) reads as that text. Any other substitution
// (`$(date -I)`, `$(pg_dump …)`) adds nothing. Both hold for a substitution
// in a here-string, an unquoted heredoc body, a printer's words piped in,
// and an argument word of the phrase head (`psql -c "$(cat f)"`).

// wordSubsts returns what the $( ) and backtick substitutions in word w of
// the command at print (substPrints).
func (r *inputReader) wordSubsts(at inputAt, w placedWord) []inputSource {
	var out []inputSource
	for _, p := range w.parts {
		if p.kind == partSubst {
			out = append(out, r.substPrints(at, p.code(at.text))...)
		}
	}
	return out
}

// docSubsts returns what the substitutions in heredoc d of the command at
// print, none for a quoted delimiter, whose body the shell does not expand
// (bodySubstitutions).
func (r *inputReader) docSubsts(at inputAt, d shellDoc) []inputSource {
	if d.quoted {
		return nil
	}
	var out []inputSource
	for _, code := range bodySubstitutions(at.text, d.body) {
		out = append(out, r.substPrints(at, code)...)
	}
	return out
}

// printerSubsts returns what the substitutions a printer piped into a
// command prints hold: in its words, its here-strings and its heredocs.
func (r *inputReader) printerSubsts(at inputAt) []inputSource {
	c := at.cmds[at.i]
	var out []inputSource
	for _, w := range c.words {
		out = append(out, r.wordSubsts(at, w)...)
	}
	for k, op := range c.redirs {
		if op.op == "<<<" && redirFd(op) == "0" {
			out = append(out, r.wordSubsts(at, c.targets[k])...)
		}
	}
	for _, d := range c.docs {
		out = append(out, r.docSubsts(at, d)...)
	}
	return out
}

// substPrints returns what a substitution whose code is body prints into
// the command at, as far as belt reads it: `$(< f)` reads f, and one
// pipeline reads as its last command prints (inputReader.prints), its own
// standard input being the shell's where the command stands, what an exec
// left and the groups around it. Code belt cannot follow, or more than one
// pipeline, gives nothing.
func (r *inputReader) substPrints(at inputAt, body string) []inputSource {
	cmds, ok := readShell(body)
	if !ok || len(cmds) == 0 {
		return nil
	}
	for _, c := range cmds[1:] {
		if c.before != jointPipe {
			return nil
		}
	}
	in := inputAt{
		text: body, cmds: cmds, i: len(cmds) - 1, runDir: at.runDir, nested: true,
		outer: at.outer, stdin: at.stdin, fns: at.fns,
	}
	c := cmds[0]
	if len(cmds) > 1 || len(c.words) > 0 {
		return r.prints(in, "into a substitution")
	}
	var out []inputSource
	for k, op := range c.redirs {
		if (op.op == "<" || op.op == "<>") && redirFd(op) == "0" {
			what := fmt.Sprintf("the file %s that `$(< …)` reads", c.targets[k].raw)
			out = append(out, r.fileSource(what, c.targets[k], in)...)
		}
	}
	return out
}

// argSources returns what the substitutions in the command's words, from
// the one run starts at on, print (wordSubsts), each an argument of the
// command. The words in front of run, such as an assignment (`X="$(cat f)"
// psql`) or a wrapper's options, are not its arguments.
func (r *inputReader) argSources(at inputAt, run []string) []inputSource {
	var out []inputSource
	for _, w := range runWords(at.cmds[at.i], run) {
		for _, src := range r.wordSubsts(at, w) {
			src.arg = true
			out = append(out, src)
		}
	}
	return out
}

// runWords returns the words of c from the one run, a suffix of its tokens
// (commandRun), starts at, or all of them when run is not one: the words a
// wrapper hands a shell are split again.
func runWords(c shellCmd, run []string) []placedWord {
	tokens := c.tokens()
	if off := len(tokens) - len(run); off >= 0 && slices.Equal(tokens[off:], run) {
		return c.words[off:]
	}
	return c.words
}

// xargsNames are the names xargs goes by: Homebrew installs GNU's as gxargs.
var xargsNames = wordSet("xargs gxargs")

// xargsHit reads an xargs whose command is a phrase head (`cat f | xargs
// psql -c`): the items it reads, from its -a file or else from its standard
// input, are argument text for that command, read as check reads a source.
// With -a, GNU's xargs hands the command its own standard input, so that
// counts too. nil for any other command.
func (r *inputReader) xargsHit(at inputAt, run []string, patterns []deny, tools map[string]bool) *inputHit {
	if len(run) == 0 || !xargsNames[commandName(shellValue(run[0]))] {
		return nil
	}
	call := readXargs(run)
	inner := commandRun(call.command)
	tool := phraseHead(inner, tools)
	if tool == "" {
		return nil
	}
	var srcs []inputSource
	if call.itemFile > 0 {
		srcs = append(srcs, r.itemFile(at, run, call)...)
	}
	for _, src := range r.sources(at) {
		if src.arg = call.itemFile == 0; src.arg {
			src.what += " (through `xargs`)"
		}
		srcs = append(srcs, src)
	}
	srcs = append(srcs, r.argSources(at, run)...)
	return firstHit(inner, tool, srcs, patterns)
}

// itemFile returns the file an xargs -a (--arg-file) names, run being the
// xargs words from its name on, read as a `<` file is. A name glued to its
// option (`-ax.sql`, `--arg-file=x.sql`) is read when it holds no
// expansion; belt cannot place one that does.
func (r *inputReader) itemFile(at inputAt, run []string, call xargsCall) []inputSource {
	src := inputSource{what: fmt.Sprintf("the file %s that `xargs` reads its items from", call.itemName), arg: true}
	words := runWords(at.cmds[at.i], run)
	if len(words) != len(run) {
		src.unread = "belt cannot tell which word of the xargs command names it"
		return []inputSource{src}
	}
	word := words[call.itemFile]
	if v, ok := word.literal(); ok && v != call.itemName {
		word = placedWord{shWord: shWord{raw: call.itemName, parts: []wordPart{{text: call.itemName}}}}
	} else if !ok && strings.HasPrefix(word.raw, "-") {
		src.unread = "its name holds an expansion only the shell can read"
		return []inputSource{src}
	}
	out := r.fileSource(src.what, word, at)
	for i := range out {
		out[i].arg = true
	}
	return out
}
