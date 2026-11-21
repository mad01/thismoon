package guard

import (
	"fmt"
	"slices"
	"strings"
)

// optArg is how a git option takes its value, as `git <sub> -h` shows it.
type optArg int

const (
	// argNone takes no value; one after = makes git fail.
	argNone optArg = iota
	// argNeeded takes a value: after =, the rest of a short bundle, or else
	// the next word, even `--` or one that starts with -.
	argNeeded
	// argOptional takes a value only after =, or as the rest of a short
	// bundle.
	argOptional
)

// gitOpt is one entry of a subcommand's option table.
type gitOpt struct {
	// long is the name after --, "" for a short option alone.
	long string
	// short is the letter after -, 0 for a long option alone.
	short byte
	arg   optArg
	// negates marks an option git also takes as --no-<long>, shown as
	// `--[no-]` in `git <sub> -h`.
	negates bool
}

// name is how a read option names its entry: the long name, or the letter
// of a short option alone.
func (o gitOpt) name() string {
	if o.long != "" {
		return o.long
	}
	return string(o.short)
}

// resetOptions are the options of `git reset -h` (git 2.55). The five
// modes take no --no- form.
var resetOptions = []gitOpt{
	{long: "quiet", short: 'q', negates: true},
	{long: "no-refresh"},
	{long: "refresh"},
	{long: "mixed"},
	{long: "soft"},
	{long: "hard"},
	{long: "merge"},
	{long: "keep"},
	{long: "recurse-submodules", arg: argOptional, negates: true},
	{long: "patch", short: 'p', negates: true},
	{long: "auto-advance", negates: true},
	{long: "unified", short: 'U', arg: argNeeded},
	{long: "inter-hunk-context", arg: argNeeded},
	{long: "intent-to-add", short: 'N', negates: true},
	{long: "pathspec-from-file", arg: argNeeded, negates: true},
	{long: "pathspec-file-nul", negates: true},
}

// cleanOptions are the options of `git clean -h` (git 2.55).
var cleanOptions = []gitOpt{
	{long: "quiet", short: 'q', negates: true},
	{long: "dry-run", short: 'n', negates: true},
	{long: "force", short: 'f', negates: true},
	{long: "interactive", short: 'i', negates: true},
	{short: 'd'},
	{long: "exclude", short: 'e', arg: argNeeded},
	{short: 'x'},
	{short: 'X'},
}

// gitOptRead is one option as git reads it from a word.
type gitOptRead struct {
	// name is the entry's gitOpt.name.
	name string
	// unset marks a --no- form.
	unset bool
	// word is the word it was read from.
	word string
}

// gitWords are a subcommand's words the way git reads them.
type gitWords struct {
	// opts are the options in the order git reads them.
	opts []gitOptRead
	// operands are the words before `--` that are no option or value.
	operands []string
	// paths are the words after `--`, and dashed says there was one.
	paths  []string
	dashed bool
}

// sets reports whether an option of that name is read in a form that sets
// it, not a --no- one.
func (w gitWords) sets(name string) bool {
	return slices.ContainsFunc(w.opts, func(o gitOptRead) bool {
		return o.name == name && !o.unset
	})
}

// readGitWords reads a subcommand's words against its option table the way
// git's parse-options does. An option counts anywhere before `--`, short
// ones bundled (`-fdx`), a long one as its exact name, a prefix of one name
// alone (`--forc`) or a --no- form of one that negates. A value-taking
// option takes its value even when it is `--` or starts with -. It refuses
// a word git may read some way the table does not say: an option not in it,
// a prefix of more than one name, a value on an option that takes none, a
// missing value, and a word the shell may still turn into an option
// (mayBecomeOption). The table is an allow list: an option belt does not
// know may be one that discards, so callers read a refusal as a discard.
func readGitWords(args []string, table []gitOpt) (gitWords, error) {
	var w gitWords
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case mayBecomeOption(a):
			return w, fmt.Errorf("the word %s", a)
		case a == "--":
			w.paths, w.dashed = args[i+1:], true
			return w, nil
		case a == "--end-of-options":
			return w.endOptions(args[i+1:])
		case a == "-" || !strings.HasPrefix(a, "-"):
			w.operands = append(w.operands, a)
			continue
		}
		read, next, err := readOptWord(a, table)
		switch {
		case err != nil:
			return w, err
		case next && i+1 == len(args):
			return w, fmt.Errorf("option %s without its value", a)
		case next && mayBecomeOption(args[i+1]):
			return w, fmt.Errorf("the word %s", args[i+1])
		case next:
			i++
		}
		w.opts = append(w.opts, read...)
	}
	return w, nil
}

// endOptions reads the words after --end-of-options, which git reads as
// operands, with a later -- still ending them where paths start. A word the
// shell may still turn into a -- or a glob (mayBecomeOption) is refused,
// since it would move words from operands to paths.
func (w gitWords) endOptions(rest []string) (gitWords, error) {
	for _, a := range rest {
		if mayBecomeOption(a) {
			return w, fmt.Errorf("the word %s", a)
		}
	}
	if j := slices.Index(rest, "--"); j >= 0 {
		w.operands = append(w.operands, rest[:j]...)
		w.paths, w.dashed = rest[j+1:], true
		return w, nil
	}
	w.operands = append(w.operands, rest...)
	return w, nil
}

// readOptWord reads one option word: a long option or a bundle of short
// ones. next says its last option takes the next word as its value.
func readOptWord(word string, table []gitOpt) (read []gitOptRead, next bool, err error) {
	text, long := strings.CutPrefix(word, "--")
	if !long {
		return readBundle(word, table)
	}
	name, _, valued := strings.Cut(text, "=")
	opt, unset, ok := resolveLong(name, table)
	switch {
	case !ok:
		return nil, false, fmt.Errorf("option %s", word)
	case valued && (unset || opt.arg == argNone):
		return nil, false, fmt.Errorf("option %s takes no value", word)
	}
	read = []gitOptRead{{name: opt.name(), unset: unset, word: word}}
	return read, !valued && !unset && opt.arg == argNeeded, nil
}

// resolveLong finds the entry a long option name names, as git does: an
// exact name or an exact --no- form first, else the one entry the name, or
// the part after no-, is a prefix of. A name that is a prefix of no-
// itself git reads as every negation at once, which is ambiguous.
func resolveLong(name string, table []gitOpt) (opt gitOpt, unset, ok bool) {
	if strings.HasPrefix("no-", name) {
		return gitOpt{}, false, false
	}
	negated, isNo := strings.CutPrefix(name, "no-")
	type match struct {
		opt   gitOpt
		unset bool
	}
	var found []match
	for _, o := range table {
		switch {
		case o.long == "":
		case o.long == name:
			return o, false, true
		case isNo && o.negates && o.long == negated:
			return o, true, true
		case strings.HasPrefix(o.long, name):
			found = append(found, match{o, false})
		case isNo && o.negates && strings.HasPrefix(o.long, negated):
			found = append(found, match{o, true})
		}
	}
	if len(found) != 1 {
		return gitOpt{}, false, false
	}
	return found[0].opt, found[0].unset, true
}

// readBundle reads a bundle of short options (`-fdx`). A letter that takes
// a value takes the rest of the word, or for argNeeded the next word when
// it ends the bundle.
func readBundle(word string, table []gitOpt) (read []gitOptRead, next bool, err error) {
	for i := 1; i < len(word); i++ {
		at := slices.IndexFunc(table, func(o gitOpt) bool { return o.short == word[i] })
		if at < 0 {
			return nil, false, fmt.Errorf("option %s", word)
		}
		o := table[at]
		read = append(read, gitOptRead{name: o.name(), word: word})
		if o.arg != argNone {
			return read, o.arg == argNeeded && i == len(word)-1, nil
		}
	}
	return read, false, nil
}

// shellPatternLead are the characters that, leading a word, make the shell
// expand it into file names or several words: a glob, a brace, and zsh's ^
// negation.
const shellPatternLead = "*?[{^"

// mayBecomeOption reports whether the shell may still turn a word into one
// git reads as an option, or split one off it. The walk keeps a word as
// written when the shell has more to do with it. A $ or ` expansion may
// hold anything, and so may a ( pattern (bash's @(...), zsh's qualifiers).
// A leading shellPatternLead pattern may match a file named like an option.
// A quote or backslash in front keeps the next character literal, so it
// hides a pattern but still hands git a leading -.
func mayBecomeOption(word string) bool {
	if strings.ContainsAny(word, "$`(") {
		return true
	}
	lead := word
	for strings.HasPrefix(lead, `''`) || strings.HasPrefix(lead, `""`) {
		lead = lead[2:]
	}
	switch {
	case lead == "":
		return false
	case strings.IndexByte(`'"\`, lead[0]) >= 0:
		return len(lead) > 1 && lead[1] == '-'
	}
	return strings.IndexByte(shellPatternLead, lead[0]) >= 0
}
