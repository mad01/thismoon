package guard

import (
	"path/filepath"
	"slices"
	"strings"
)

// This file reads a pipeline into a command that runs its standard input as
// code, for the stages in front of it that print a download or decoded
// text: through tee, inside a { } group, if, loop or ( ) subshell on either
// side of the pipe, and into an xargs that runs its items as code. A >( )
// that runs its standard input as code is read the same way, for what the
// command writes into it.

// pipedSource returns what the commands piped into a command that runs its
// standard input as code (runsInput) print that belt cannot read, or "".
// The command may read the pipe inside a group the pipe feeds (pipeFeed),
// and the stages may sit in one (pipelineSource).
func (w codeWalk) pipedSource(text string, cmds []shellCmd, i int, run []string) string {
	feed, ok := pipeFeed(cmds, i)
	if !ok || !w.runsInput(text, cmds[i], run) {
		return ""
	}
	return w.pipelineSource(text, cmds, feed)
}

// procSource returns what a command writes that belt cannot read into a
// >( ) whose body runs its standard input as code, or "": its output, sent
// there by a redirection (`curl … > >(sh)`) or written to it as a file it
// names (`curl -o >(sh) …`), and for tee the input it copies there (`curl
// … | tee >(sh)`).
func (w codeWalk) procSource(text string, cmds []shellCmd, i int, run []string) string {
	if !w.writesToRunner(text, cmds[i]) {
		return ""
	}
	if src := w.stageSource(text, cmds[i]); src != "" {
		return src
	}
	if len(run) > 0 && commandName(run[0]) == "tee" {
		if feed, ok := pipeFeed(cmds, i); ok {
			return w.pipelineSource(text, cmds, feed)
		}
	}
	return ""
}

// writesToRunner reports whether a command's words or redirection targets
// hold a >( ) whose body runs its standard input as code (bodyRunsInput).
func (w codeWalk) writesToRunner(text string, c shellCmd) bool {
	for _, words := range [][]placedWord{c.words, c.targets} {
		for _, word := range words {
			for _, p := range word.pieces() {
				if p.from >= 2 && text[p.from-2:p.from] == ">(" && w.bodyRunsInput(text[p.from:p.to]) {
					return true
				}
			}
		}
	}
	return false
}

// pipelineSource returns what the stages of the pipeline in front of
// cmds[feed] print that belt cannot read, or "". Every stage counts, and so
// does every command of a group a stage is (`curl … | tee x | sh`, `{ curl
// …; } | sh`, `if x; then curl …; fi | sh`).
func (w codeWalk) pipelineSource(text string, cmds []shellCmd, feed int) string {
	depth := 0
	for j := feed - 1; j >= 0; j-- {
		if src := w.stageSource(text, cmds[j]); src != "" {
			return src
		}
		tokens := cmds[j].tokens()
		depth += groupCloses(tokens) - groupOpens(tokens)
		if depth <= 0 && cmds[j].before != jointPipe {
			return ""
		}
	}
	return ""
}

// pipeFeed returns the index of the command whose joint in front is the
// pipe cmds[i] reads its standard input from: cmds[i] itself, or the
// command that opens a group cmds[i] runs inside (`curl … | { cd x; sh;
// }`, `curl … | while read l; do sh; done`). false when no pipe feeds it.
// Walking back, depth counts the groups closed before cmds[i]; an opener
// past them opens a group cmds[i] is in.
func pipeFeed(cmds []shellCmd, i int) (int, bool) {
	if cmds[i].before == jointPipe {
		return i, true
	}
	depth := 0
	for k := i; k >= 0; k-- {
		tokens := cmds[k].tokens()
		if k < i {
			depth += groupCloses(tokens)
		}
		depth -= groupOpens(tokens)
		if depth >= 0 {
			continue
		}
		if cmds[k].before == jointPipe {
			return k, true
		}
		depth = 0
	}
	return 0, false
}

// groupOpeners and groupClosers are the reserved words that open and close
// a group of commands the reader reads as commands of their own.
var (
	groupOpeners = wordSet("{ if while until for select case")
	groupClosers = wordSet("} fi done esac")
)

// groupOpens counts the groups a command's leading words open (`{ {`, `then
// if`); for, select and case end the count, as a word that is no reserved
// word follows them.
func groupOpens(tokens []string) int {
	n := 0
	for _, t := range tokens {
		switch {
		case t == "for" || t == "select" || t == "case":
			return n + 1
		case groupOpeners[t]:
			n++
		case !shellReserved[t]:
			return n
		}
	}
	return n
}

// groupCloses counts the groups a command's leading words close (`}`,
// `done`, `} }`).
func groupCloses(tokens []string) int {
	n := 0
	for _, t := range tokens {
		if !groupClosers[t] {
			break
		}
		n++
	}
	return n
}

// runsInput reports whether a command runs what it reads from standard
// input as code: an interpreter or source that reads its script from it
// (readsStdinScript), an xargs that runs its items as code
// (xargsRunsItems), or a ( ) subshell with such a command inside that no
// pipe of its own feeds (bodyRunsInput).
func (w codeWalk) runsInput(text string, c shellCmd, run []string) bool {
	if readsStdinScript(text, c, run) {
		return true
	}
	if _, ok := xargsRunsItems(w.s.names, run); ok {
		return true
	}
	return slices.ContainsFunc(c.subshells, func(s span) bool {
		return w.bodyRunsInput(text[s.from:s.to])
	})
}

// bodyRunsInput reports whether a ( ) or >( ) body holds a command that runs
// its standard input as code (runsInput) and that no pipe of its own feeds.
// A command word that names an interpreter through a variable counts as
// that interpreter (shellNames.withRunner).
func (w codeWalk) bodyRunsInput(body string) bool {
	cmds, ok := readShell(body)
	if !ok {
		return false
	}
	for k, inner := range cmds {
		run, _ := unwrapCommand(functionBody(body, inner, inner.tokens()))
		if _, fed := pipeFeed(cmds, k); !fed && w.runsInput(body, inner, w.s.names.withRunner(run)) {
			return true
		}
	}
	return false
}

// readsStdinScript reports whether a command, run its words from the
// command name on, runs the script it reads from standard input: an
// interpreter with no script string, file, heredoc or input redirection, or
// with /dev/stdin as its file, and source or . of /dev/stdin.
func readsStdinScript(text string, c shellCmd, run []string) bool {
	switch use := scriptOf(run); {
	case use.at > 0:
		return !use.inline && stdinFiles[run[use.at]]
	case use.stdin:
		return len(c.docs) == 0 && len(inputTargets(text, c)) == 0
	}
	return false
}

// xargsOptions are the options of GNU and BSD xargs, marked as
// wrapperSpec marks them: GNU's from findutils 4.10's xargs --help, BSD's
// from the macOS man page. Both read them the way getopt does. GNU's -e, -i
// and -l take a value only in the same word; -a and -d are GNU's, and -J,
// -R and -S BSD's.
var xargsOptions = wrapperSpec{
	short: "0a:d:E:e::I:i::J:L:l::n:oP:pR:rS:s:tx",
	long: "null arg-file: delimiter: eof:: replace:: max-lines: max-args: " +
		"max-procs: max-chars: process-slot-var: open-tty interactive " +
		"no-run-if-empty show-limits verbose exit help version",
}

// xargsCall is what xargs's options say: the words of the command it runs,
// the string -I (-i, --replace, BSD -J) replaces with each item, and the
// index among its words (the name first) of the word that holds the name of
// the file -a (--arg-file) reads the items from (`-a FILE`, `-0aFILE`), 0
// for none. sep is the -d (--delimiter) value as written, or a NUL for -0
// (--null), whichever comes last, and split marks that one was given: the
// items then end at it alone, with no quote read.
type xargsCall struct {
	command  []string
	replace  string
	itemFile int
	sep      string
	split    bool
}

// xargsRunsItems reads an xargs command, its words from the name on, and
// reports whether it runs the items it reads as code: an interpreter whose
// inline script string is the first item (`xargs -0 sh -c`), or holds the
// string -I replaces with each item (`xargs -I{} sh -c '{}'`). A command
// word that names an interpreter through a variable counts as that
// interpreter (shellNames.withRunner).
func xargsRunsItems(names shellNames, run []string) (xargsCall, bool) {
	if len(run) == 0 || filepath.Base(run[0]) != "xargs" {
		return xargsCall{}, false
	}
	call := readXargs(run)
	inner, _ := unwrapCommand(call.command)
	inner = names.withRunner(inner)
	switch use := scriptOf(inner); {
	case use.at < 0:
		return call, true
	case use.at > 0 && use.inline && call.replace != "":
		return call, strings.Contains(inner[use.at], call.replace)
	}
	return call, false
}

// readXargs reads the options of an xargs command, its words from the name
// on (xargsCall), as getopt does (xargsOptions): a short bundle letter by
// letter up to one that takes a value, which is the rest of the word or
// else the next word (`-0a FILE`, `-ra<(…)`, `-0I{}`, `-0n 1`). A word
// xargs does not know as an option is passed over, since xargs stops on it
// and runs nothing.
func readXargs(run []string) xargsCall {
	var call xargsCall
	for i := 1; i < len(run); i++ {
		if run[i] == "--" {
			call.command = run[i+1:]
			return call
		}
		opts, ok := xargsOptions.options(run[i])
		switch {
		case !ok:
			continue
		case opts == nil:
			call.command = run[i:]
			return call
		}
		last := &opts[len(opts)-1]
		if last.arg == argNeeded && !last.attached {
			if i++; i == len(run) {
				return call
			}
			last.value = run[i]
		}
		for _, o := range opts {
			call.note(o, i)
		}
	}
	return call
}

// note records what an option says, with the index of the word that holds
// its value: the string -I, -J, -i or --replace replaces with each item
// (`{}` for -i or --replace with none), the word that names -a's file, and
// the separator -0 or -d sets.
func (c *xargsCall) note(o option, at int) {
	switch o.name {
	case "0", "null":
		c.sep, c.split = "\x00", true
	case "d", "delimiter":
		c.sep, c.split = o.value, true
	case "I", "J", "i", "replace":
		c.replace = o.value
		if o.arg == argOptional && !o.attached {
			c.replace = "{}"
		}
	case "a", "arg-file":
		c.itemFile = at
	}
}
