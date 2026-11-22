package guard

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// This file holds the one reader of xargs's options (scanXargs), for the
// script guard's token walk (readXargs) and file-discard's words
// (readXargsWords), and what the walk reads from the items an xargs runs as
// code.

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

// xargsSplit are the bytes at which xargs without -0 or -d cuts its input,
// and the quotes and backslash it reads as its own.
const xargsSplit = " \t\n\v\f\r'\"\\"

// xargsCall is what xargs's options say: the words of the command it runs,
// the string -I (-i, --replace, BSD -J) replaces with each item, and the
// index among its words (the name first) of the word that holds the name of
// the file -a (--arg-file) reads the items from (`-a FILE`, `-0aFILE`), 0
// for none. sep is the -d (--delimiter) value as written, or a NUL for -0
// (--null), whichever comes last, and split marks that one was given: the
// items then end at it alone, with no quote read. unread marks a -d value
// with an expansion in it. batch marks BSD's -J, which puts every item in
// place of the one word that is its string alone.
type xargsCall struct {
	command  []string
	replace  string
	itemFile int
	sep      string
	split    bool
	unread   bool
	batch    bool
}

// readXargs reads an xargs command, its tokens from the name on
// (scanXargs), each one text belt reads.
func readXargs(run []string) xargsCall {
	call, at := scanXargs(len(run), func(i int) (string, bool) { return run[i], true })
	if at >= 0 {
		call.command = run[at:]
	}
	return call
}

// readXargsWords reads an xargs command's words after its name the way
// readXargs reads tokens, and returns the words of the command it runs. A
// word with an expansion where an option may stand starts the command, and
// one in a value's place is the value as written.
func readXargsWords(args []shWord) (xargsCall, []shWord) {
	call, at := scanXargs(len(args)+1, func(i int) (string, bool) {
		if v, ok := args[i-1].literal(); ok {
			return v, true
		}
		return args[i-1].raw, false
	})
	if at < 0 {
		return call, nil
	}
	return call, args[at-1:]
}

// scanXargs reads the options of an xargs command of n words, its name
// first, as getopt does (xargsOptions): a short bundle letter by letter up
// to one that takes a value, which is the rest of the word or else the next
// word (`-0a FILE`, `-ra<(…)`, `-0I{}`, `-0n 1`). word gives word i's text
// and whether belt can read it; one it cannot read starts the command. A
// word xargs does not know as an option is passed over, since xargs stops
// on it and runs nothing. at is the index of the command's first word, -1
// when the words hold no command.
func scanXargs(n int, word func(int) (string, bool)) (call xargsCall, at int) {
	for i := 1; i < n; i++ {
		v, read := word(i)
		switch {
		case !read:
			return call, i
		case v == "--":
			return call, i + 1
		}
		opts, ok := xargsOptions.options(v)
		switch {
		case !ok:
			continue
		case opts == nil:
			return call, i
		}
		last := &opts[len(opts)-1]
		if last.arg == argNeeded && !last.attached {
			if i++; i == n {
				return call, -1
			}
			last.value, read = word(i)
		}
		for _, o := range opts {
			call.note(o, i, read)
		}
	}
	return call, -1
}

// note records what an option says, with the index of the word that holds
// its value and whether belt reads that value: the string -I, -J, -i or
// --replace replaces with each item (`{}` for -i or --replace with none),
// the word that names -a's file, and the separator -0 or -d sets.
func (c *xargsCall) note(o option, at int, read bool) {
	switch o.name {
	case "0", "null":
		c.sep, c.split, c.unread = "\x00", true, false
	case "d", "delimiter":
		c.sep, c.split, c.unread = o.value, true, !read
	case "I", "J", "i", "replace":
		c.replace, c.batch = o.value, o.name == "J"
		if o.arg == argOptional && !o.attached {
			c.replace = "{}"
		}
	case "a", "arg-file":
		c.itemFile = at
	}
}

// cut returns the bytes at which xargs cuts its input into items: the -d
// byte, "" for a NUL, which no path holds, and xargsSplit with neither.
func (c xargsCall) cut() string {
	if !c.split {
		return xargsSplit
	}
	if b, ok := delimByte(c.sep); ok && b != 0 {
		return string(b)
	}
	return ""
}

// delimWhy says why belt cannot read the -d value, "" when it can.
func (c xargsCall) delimWhy() string {
	if _, ok := delimByte(c.sep); !c.split || ok && !c.unread {
		return ""
	}
	return "belt cannot read the xargs delimiter `" + c.sep + "`"
}

// marks are the texts xargs fills in the command it runs: the -I string
// anywhere in a word, and BSD's -J string only as a whole word.
func (c xargsCall) marks() *regexp.Regexp {
	if c.batch {
		return regexp.MustCompile("^" + regexp.QuoteMeta(c.replace) + "$")
	}
	return regexp.MustCompile(regexp.QuoteMeta(c.replace))
}

// delimByte decodes one xargs -d value into its byte: one byte, or a
// backslash escape (\n, \t, \0, \\, an octal \NNN or a hex \xHH).
func delimByte(v string) (byte, bool) {
	switch {
	case len(v) == 1:
		return v[0], true
	case len(v) < 2 || v[0] != '\\':
		return 0, false
	}
	if c, ok := map[byte]byte{
		'n': '\n', 't': '\t', 'r': '\r', 'v': '\v', 'f': '\f', 'a': '\a', 'b': '\b', '\\': '\\',
	}[v[1]]; ok && len(v) == 2 {
		return c, true
	}
	base, digits := 8, v[1:]
	if v[1] == 'x' {
		base, digits = 16, v[2:]
	}
	n, err := strconv.ParseUint(digits, base, 8)
	return byte(n), err == nil && digits != ""
}

// xargsFeeds returns what an xargs runs from the items it reads, when it
// runs them as code (xargsRunsItems): a shell's or an interpreter's -c with
// no script of its own runs each item as its script (`xargs -0 sh -c`), and
// one whose script holds -I's string runs that script with the item in its
// place (`xargs -I{} sh -c {}`). The items are cut (xargsItems) from -a's
// file, or else from what the command reads on its standard input
// (inputScripts), the text `| sh` would read there. Each item is a feed of
// its own, as xargs starts a shell for each. Input the walk cannot read is
// one unread feed, so the command is opaque as `curl … | sh` is.
func (w *shellWalk) xargsFeeds(text string, c shellCmd, tokens []string, in pipeIn, dir string) []feed {
	at := runAt(tokens)
	call, ok := xargsRunsItems(shellNames{}, tokens[at:])
	if !ok {
		return nil
	}
	inner, _ := unwrapCommand(call.command)
	head := len(tokens) - len(inner)
	run := shellNames{}.withRunner(inner)
	name := filepath.Base(run[0])
	base := feed{
		runner: fmt.Sprintf("the text `xargs` hands `%s` to run", tokens[head]),
		dir:    dir, pre: tokens[:head], positional: call.replace == "",
	}
	if !shellInterpreters[name] {
		base.runner = fmt.Sprintf("the `%s` script `xargs` runs", tokens[head])
		base.interp, base.lang = true, interpreterLang(name)
	}
	template := ""
	if use := scriptOf(run); use.at > 0 {
		template = c.words[head+use.at].script()
		if !strings.Contains(template, call.replace) {
			template = call.replace // an expansion belt cannot read holds the item
		}
	}
	operand := -1
	if call.itemFile > 0 {
		operand = at + call.itemFile
	}
	inputs := []script{{}} // -a's file glued to its option (`-0a<(…)`) is unread
	if operand < 0 || !strings.HasPrefix(c.words[operand].raw, "-") {
		inputs = w.inputScripts(text, c, in.from, operand, true, dir)
	}
	var out []feed
	for _, s := range inputs {
		if !s.ok {
			out = append(out, base.with(s))
			continue
		}
		for _, item := range xargsItems(s.text, call) {
			if template != "" {
				item = strings.ReplaceAll(template, call.replace, item)
			}
			out = append(out, base.with(script{text: item, ok: true}))
		}
	}
	return out
}

// appendsItems reports whether a command, its words from a function's body
// on (bodyOf), is an xargs that adds the items it reads to the words of the
// command it runs: one with no -I (-i, -J, --replace) string. A shell's -c
// script there then runs with them as its arguments, $0 the first, even
// when the script is the last word (`xargs -0 sh -c 'eval "$0"'`).
func appendsItems(run []string) bool {
	at := runAt(run)
	return at < len(run) && filepath.Base(run[at]) == "xargs" && readXargs(run[at:]).replace == ""
}

// xargsItems cuts text into the items xargs reads from it. After -0 or -d
// they end at that separator alone (delimByte). Else they end at a newline,
// and at a blank too unless -I (-i, -J, --replace) makes each line one
// item, with quotes and backslashes taken off as xargs takes them off. A
// quote left open runs to the end of the text, so the walk reads more than
// xargs, which stops there.
func xargsItems(text string, call xargsCall) []string {
	if call.split {
		b, ok := delimByte(call.sep)
		if !ok {
			return []string{text}
		}
		sep := string([]byte{b})
		return strings.Split(strings.TrimSuffix(text, sep), sep)
	}
	lines := call.replace != ""
	var items []string
	var b strings.Builder
	have := false
	var quote byte
	for i := 0; i < len(text); i++ {
		switch ch := text[i]; {
		case quote != 0 && ch == quote:
			quote = 0
		case quote != 0:
			b.WriteByte(ch)
		case ch == '\'' || ch == '"':
			quote, have = ch, true
		case ch == '\\' && i+1 < len(text):
			i++
			b.WriteByte(text[i])
			have = true
		case ch == '\n' || !lines && (ch == ' ' || ch == '\t'):
			if have {
				items = append(items, b.String())
			}
			b.Reset()
			have = false
		case lines && !have && (ch == ' ' || ch == '\t'):
		default:
			b.WriteByte(ch)
			have = true
		}
	}
	if have {
		items = append(items, b.String())
	}
	return items
}
