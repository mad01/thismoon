package guard

import (
	"regexp"
	"strconv"
	"strings"
)

// This file reads a Bash deny entry from the Claude Code settings on argv,
// in Claude Code's own wildcard form:
//
//	<tool> <head words> [* <group>]... [* | :*]
//
// The tool is the command name of the first word. The head's plain words
// are verbs, matched in order past the flags before and between them
// (verbsRun), and a flag in the head is a required option (`gsutil -m rm`).
// A group after a `*` that starts with a flag names required options, and
// one that starts with a plain word is a phrase. The end is always open, so
// every entry reads as a prefix. An rm entry is read by rm's own option
// reader instead (readRm).

// denyEntry is one deny entry as read on argv.
type denyEntry struct {
	// tool is the entry's command name, in lower case.
	tool string
	// verbs are the head's plain words, matched in order.
	verbs []string
	// options must each appear after the tool word, before `--`.
	options []entryOption
	// phrases must each appear as whole words in the arguments.
	phrases []*regexp.Regexp
	// rm holds an rm entry's arguments, which rm's reader matches.
	rm []string
}

// entryOption is one required option: its name without the dashes, whether
// it is a long option, and the value it must carry when valued is set.
type entryOption struct {
	name   string
	long   bool
	value  string
	valued bool
}

// minLongPrefix is the shortest abbreviation of a long option that counts
// as that option. getopt_long takes any unique prefix, and the guard cannot
// tell which are unique, so a prefix of this many letters counts.
const minLongPrefix = 3

// entryWords returns an entry's words without its open end: a trailing `:*`
// or a trailing `*` word.
func entryWords(entry string) []string {
	words := strings.Fields(strings.TrimSuffix(strings.TrimSpace(entry), ":*"))
	for len(words) > 0 && words[len(words)-1] == "*" {
		words = words[:len(words)-1]
	}
	return words
}

// readDenyEntry parses one deny entry. ok is false for a shape the format
// cannot express: a `*` inside a word, a lone `--`, a flag group with a word
// after its value, or an rm option rm does not know. Such an entry matches
// as text only.
func readDenyEntry(entry string) (e denyEntry, ok bool) {
	words := entryWords(entry)
	if len(words) == 0 {
		return denyEntry{}, false
	}
	for _, w := range words {
		if w == "--" || (w != "*" && strings.Contains(w, "*")) {
			return denyEntry{}, false
		}
	}
	e.tool = strings.ToLower(commandName(words[0]))
	if e.tool == "" {
		return denyEntry{}, false
	}
	if e.tool == "rm" {
		return readRmEntry(e, words[1:])
	}
	groups := splitGroups(words[1:])
	for _, w := range groups[0] {
		if !isFlagWord(w) {
			e.verbs = append(e.verbs, w)
			continue
		}
		opts, ok := readOptionWord(w)
		if !ok {
			return denyEntry{}, false
		}
		e.options = append(e.options, opts...)
	}
	for _, g := range groups[1:] {
		if !e.addGroup(g) {
			return denyEntry{}, false
		}
	}
	return e, true
}

// readRmEntry keeps an rm entry's arguments, the `*` words dropped, when rm
// knows every long option in them.
func readRmEntry(e denyEntry, args []string) (denyEntry, bool) {
	for _, a := range args {
		if a != "*" {
			e.rm = append(e.rm, a)
		}
	}
	if _, known := readRm(e.rm); !known {
		return denyEntry{}, false
	}
	return e, true
}

// splitGroups splits the words after the tool at each `*`. The first group
// is the head, which may be empty.
func splitGroups(words []string) [][]string {
	groups := [][]string{nil}
	for _, w := range words {
		if w == "*" {
			groups = append(groups, nil)
			continue
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], w)
	}
	return groups
}

// addGroup reads one group after a `*`: required options when it starts
// with a flag, where a plain word right after a flag is that flag's value,
// and a phrase otherwise.
func (e *denyEntry) addGroup(group []string) bool {
	if len(group) == 0 {
		return true
	}
	if !isFlagWord(group[0]) {
		e.phrases = append(e.phrases, wordsRegexp(group, phraseGap))
		return true
	}
	for i := 0; i < len(group); i++ {
		if !isFlagWord(group[i]) {
			return false
		}
		opts, ok := readOptionWord(group[i])
		if !ok {
			return false
		}
		last := &opts[len(opts)-1]
		if !last.valued && i+1 < len(group) && !isFlagWord(group[i+1]) {
			last.value, last.valued = group[i+1], true
			i++
		}
		e.options = append(e.options, opts...)
	}
	return true
}

// isFlagWord reports whether an entry word is a flag.
func isFlagWord(w string) bool {
	return len(w) > 1 && w[0] == '-'
}

// readOptionWord reads one flag word of an entry: `--name` or `--name=v` is
// one long option, and `-abc` is the one-letter options a, b and c, the
// last of which takes the value of `-c=v`.
func readOptionWord(w string) ([]entryOption, bool) {
	name, value, valued := strings.Cut(w, "=")
	if long, ok := strings.CutPrefix(name, "--"); ok {
		if long == "" {
			return nil, false
		}
		return []entryOption{{name: long, long: true, value: value, valued: valued}}, true
	}
	letters := name[1:]
	if letters == "" {
		return nil, false
	}
	opts := make([]entryOption, 0, len(letters))
	for _, c := range letters {
		opts = append(opts, entryOption{name: string(c)})
	}
	opts[len(opts)-1].value, opts[len(opts)-1].valued = value, valued
	return opts, true
}

// phraseGap is what may stand between a phrase's words: whitespace, \v
// included (sqlSpace), and SQL comments, since a phrase is the SQL rule. A
// block comment (`DELETE/**/FROM t`) may stand anywhere, and a line comment
// (`--` or `#`) counts up to the newline that ends it, so `DELETE -- x` and
// `FROM t` on the next line match while `DELETE --x FROM t` on one line
// does not. The text match keeps textGap: in shell text `/*` is a glob, not
// a comment.
const phraseGap = `(?:[\s\v]|/\*(?s:.*?)\*/|(?:--|#)[^\n\x00]*\n)+`

// runs reports whether one command, its words with the command name first,
// runs what the entry names: the verbs in order, every required option, and
// every phrase (phraseText). A word in ANSI-C quoting reads as the shell
// passes it (ansiCWords).
func (e denyEntry) runs(words []string) bool {
	args, ok := e.named(words)
	return ok && (e.tool == "rm" || e.phrasesIn(args))
}

// runsReading is runs for a command that reads input, the text on its
// standard input: the input counts as one more argument, for the phrases
// alone (phrasesReading). An entry with no phrase reads no input, so it
// never matches here.
func (e denyEntry) runsReading(words []string, input string) bool {
	if len(e.phrases) == 0 {
		return false
	}
	args, ok := e.named(words)
	return ok && e.phrasesReading(args, input)
}

// named reports whether a command names the entry's tool, verbs and
// options, and returns its arguments after the tool word. For an rm entry
// it reports whether rm's reader matches the arguments.
func (e denyEntry) named(words []string) ([]string, bool) {
	if len(words) == 0 {
		return nil, false
	}
	words = ansiCWords(words)
	if !strings.EqualFold(commandName(words[0]), e.tool) {
		return nil, false
	}
	args := words[1:]
	if e.tool == "rm" {
		return args, rmRuns(e.rm, args)
	}
	if !verbsRun(e.tool, e.verbs, args) {
		return nil, false
	}
	for _, o := range e.options {
		if !o.in(args) {
			return nil, false
		}
	}
	return args, true
}

// phrasesIn reports whether every phrase of the entry is in the arguments
// after the tool word (phraseText).
func (e denyEntry) phrasesIn(args []string) bool {
	if len(e.phrases) == 0 {
		return true
	}
	return e.phrasesInText(phraseText(args, ""))
}

// phrasesReading is phrasesIn with input, the text the command reads on its
// standard input, as one more argument after the others (phraseText).
func (e denyEntry) phrasesReading(args []string, input string) bool {
	return e.phrasesInText(phraseText(args, input))
}

// phrasesInText reports whether every phrase of the entry is in a phrase
// text.
func (e denyEntry) phrasesInText(text string) bool {
	for _, p := range e.phrases {
		if !p.MatchString(text) {
			return false
		}
	}
	return true
}

// phraseText is the text a phrase is read in: the arguments after the tool
// word as one text with the quotes collapsed, since a quoted argument
// reaches the guard as the words it holds, then each value a single-dash
// word may glue to its option (gluedValues). Each value is a piece of its
// own, set apart by a NUL, so a phrase never runs from one into the next.
// input, the text a command reads on its standard input, joins the
// arguments as one more after them when it is not empty. It glues to no
// option, so a body of any size is read once.
func phraseText(args []string, input string) string {
	first := strings.Join(args, " ")
	if input != "" {
		first += " " + input
	}
	pieces := []string{first}
	for i, a := range args {
		pieces = append(pieces, gluedValues(a, args[i+1:])...)
	}
	return lineNoise.Replace(strings.Join(pieces, "\x00"))
}

// gluedValues returns the values a single-dash word may carry glued to an
// option: the text after each letter or digit of the word's leading run of
// them, since any of them may be the option that takes the rest of the
// word, as `c` does in `-cDROP TABLE t` and in the bundles `-tAcDROP TABLE
// t` and `-1cDROP TABLE t`. A quote before the dash drops (`'-cDROP TABLE
// t'`). Only a value that holds whitespace (sqlSpace) or a `/*` comment
// counts: a statement a phrase names takes more than one word, and a
// one-word value would read every letter run as a value (`-dbackdrop` holds
// `drop`). A word cut from a longer shell word (cutWord) runs on into rest,
// the words after it.
func gluedValues(word string, rest []string) []string {
	cut := cutWord(word) && len(rest) > 0
	word = strings.TrimLeft(word, `'"`)
	if len(word) < 3 || word[0] != '-' || word[1] == '-' {
		return nil
	}
	if cut {
		word += " " + strings.Join(rest, " ")
	}
	var out []string
	for i := 1; i < len(word)-1 && (isLetter(word[i]) || isDigit(word[i])); i++ {
		if value := word[i+1:]; strings.ContainsAny(value, sqlSpace) || strings.Contains(value, "/*") {
			out = append(out, value)
		}
	}
	return out
}

// sqlSpace is the whitespace an SQL statement may set its words apart
// with, as the psql and mysql lexers read it.
const sqlSpace = " \t\n\r\f\v"

// cutWord reports whether a word holds an open quote or ends in a
// backslash. The script and unread reads split a line at whitespace, so
// such a word was cut from a longer shell word: `'-cDROP` from `'-cDROP
// TABLE t'`, `-cDROP\` from `-cDROP\ TABLE\ t`.
func cutWord(word string) bool {
	return strings.Count(word, "'")%2 == 1 || strings.Count(word, `"`)%2 == 1 || strings.HasSuffix(word, `\`)
}

// isLetter reports whether c is an ASCII letter.
func isLetter(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// isDigit reports whether c is an ASCII digit.
func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

// allLetters reports whether s is made of ASCII letters alone.
func allLetters(s string) bool {
	for i := range len(s) {
		if !isLetter(s[i]) {
			return false
		}
	}
	return true
}

// in reports whether the option is among the arguments, before `--`. A
// valued option's value follows `=`, follows a one-letter flag in the same
// word (`-XDELETE`), or is the next word.
func (o entryOption) in(args []string) bool {
	for i, arg := range args {
		if arg == "--" {
			return false
		}
		if o.at(args, i) {
			return true
		}
	}
	return false
}

// at reports whether args[i] gives the option with the value the entry
// names: in the same word, or as the next word.
func (o entryOption) at(args []string, i int) bool {
	value, has, ok := o.read(args[i])
	if !ok {
		return false
	}
	if !o.valued {
		return true
	}
	if !has && i+1 < len(args) {
		value, has = args[i+1], true
	}
	return has && o.takes(value)
}

// read reports whether one argument gives the option, and the value the
// same word carries, if any. A long option matches by its name or a prefix
// of minLongPrefix letters or more. A one-letter option matches as the
// word's first letter, or inside a bundle made of letters alone (`-fv`,
// `-iXDELETE`), up to an `=`. Past the first letter of any other word the
// letter is part of a value (`-HX-Req: $ID` gives -H, not -X).
func (o entryOption) read(arg string) (value string, has, ok bool) {
	if o.long {
		rest, isLong := strings.CutPrefix(arg, "--")
		if !isLong {
			return "", false, false
		}
		name, value, has := strings.Cut(rest, "=")
		if name != o.name && (len(name) < minLongPrefix || !strings.HasPrefix(o.name, name)) {
			return "", false, false
		}
		// The quote-collapsed form of `--method="DELETE"` is `--method= DELETE`.
		return value, has && value != "", true
	}
	if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
		return "", false, false
	}
	bundle := arg[1:]
	flags, _, _ := strings.Cut(bundle, "=")
	at := strings.Index(flags, o.name)
	if at < 0 || (at > 0 && !allLetters(flags)) {
		return "", false, false
	}
	value = strings.TrimPrefix(bundle[at+len(o.name):], "=")
	return value, value != "", true
}

// takes reports whether a command's value for the option is the entry's
// value, case aside and with quotes dropped, or the same number when both
// are integers (sameInteger). A value the shell expands ($X) may be any
// value, so it counts.
func (o entryOption) takes(value string) bool {
	value = strings.Trim(value, `'"`)
	return strings.Contains(value, "$") || strings.EqualFold(value, o.value) || sameInteger(value, o.value)
}

// sameInteger reports whether a and b are the same integer, read in base
// 10 or with Go's base prefixes, as the pflag-based CLIs read a number:
// `--replicas=00`, `+0` and `0x0` are all 0.
func sameInteger(a, b string) bool {
	for _, base := range []int{10, 0} {
		x, errA := strconv.ParseInt(a, base, 64)
		y, errB := strconv.ParseInt(b, base, 64)
		if errA == nil && errB == nil && x == y {
			return true
		}
	}
	return false
}
