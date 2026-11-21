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

// worktreeRemoveOptions are the options of `git worktree remove -h` (git
// 2.55). -f counts up: twice removes a locked worktree too.
var worktreeRemoveOptions = []gitOpt{
	{long: "force", short: 'f', negates: true},
}

// worktreePruneOptions are the options of `git worktree prune -h` (git
// 2.55).
var worktreePruneOptions = []gitOpt{
	{long: "dry-run", short: 'n', negates: true},
	{long: "verbose", short: 'v', negates: true},
	{long: "expire", arg: argNeeded, negates: true},
}

// applyOptions are the options of `git apply -h` (git 2.55). --no-add and
// --add are two entries, as git lists them.
var applyOptions = []gitOpt{
	{long: "exclude", arg: argNeeded},
	{long: "include", arg: argNeeded},
	{short: 'p', arg: argNeeded},
	{long: "no-add"},
	{long: "add"},
	{long: "stat", negates: true},
	{long: "numstat", negates: true},
	{long: "summary", negates: true},
	{long: "check", negates: true},
	{long: "index", negates: true},
	{long: "intent-to-add", short: 'N', negates: true},
	{long: "cached", negates: true},
	{long: "unsafe-paths", negates: true},
	{long: "apply", negates: true},
	{long: "3way", short: '3', negates: true},
	{long: "ours"},
	{long: "theirs"},
	{long: "union"},
	{long: "build-fake-ancestor", arg: argNeeded, negates: true},
	{short: 'z'},
	{short: 'C', arg: argNeeded},
	{long: "whitespace", arg: argNeeded, negates: true},
	{long: "ignore-space-change", negates: true},
	{long: "ignore-whitespace", negates: true},
	{long: "reverse", short: 'R', negates: true},
	{long: "unidiff-zero", negates: true},
	{long: "reject", negates: true},
	{long: "allow-overlap", negates: true},
	{long: "verbose", short: 'v', negates: true},
	{long: "quiet", short: 'q', negates: true},
	{long: "inaccurate-eof", negates: true},
	{long: "recount", negates: true},
	{long: "directory", arg: argNeeded, negates: true},
	{long: "allow-empty", negates: true},
}

// readTreeOptions are the options of `git read-tree -h` (git 2.55).
var readTreeOptions = []gitOpt{
	{long: "index-output", arg: argNeeded},
	{long: "empty", negates: true},
	{long: "verbose", short: 'v', negates: true},
	{short: 'm'},
	{long: "trivial", negates: true},
	{long: "aggressive", negates: true},
	{long: "reset", negates: true},
	{long: "prefix", arg: argNeeded},
	{short: 'u'},
	{long: "exclude-per-directory", arg: argNeeded},
	{short: 'i'},
	{long: "dry-run", short: 'n', negates: true},
	{long: "no-sparse-checkout"},
	{long: "sparse-checkout"},
	{long: "debug-unpack", negates: true},
	{long: "recurse-submodules", arg: argOptional, negates: true},
	{long: "quiet", short: 'q', negates: true},
}

// checkoutIndexOptions are the options of `git checkout-index -h` (git
// 2.55). --no-create and --create are two entries, as git lists them.
var checkoutIndexOptions = []gitOpt{
	{long: "all", short: 'a', negates: true},
	{long: "ignore-skip-worktree-bits", negates: true},
	{long: "force", short: 'f', negates: true},
	{long: "quiet", short: 'q', negates: true},
	{long: "no-create", short: 'n'},
	{long: "create"},
	{long: "index", short: 'u', negates: true},
	{short: 'z'},
	{long: "stdin", negates: true},
	{long: "temp", negates: true},
	{long: "prefix", arg: argNeeded, negates: true},
	{long: "stage", arg: argNeeded},
}

// rmOptions are the options of `git rm -h` (git 2.55).
var rmOptions = []gitOpt{
	{long: "dry-run", short: 'n', negates: true},
	{long: "quiet", short: 'q', negates: true},
	{long: "cached", negates: true},
	{long: "force", short: 'f', negates: true},
	{short: 'r'},
	{long: "ignore-unmatch", negates: true},
	{long: "sparse", negates: true},
	{long: "pathspec-from-file", arg: argNeeded, negates: true},
	{long: "pathspec-file-nul", negates: true},
}

// mvOptions are the options of `git mv -h` (git 2.55).
var mvOptions = []gitOpt{
	{long: "verbose", short: 'v', negates: true},
	{long: "dry-run", short: 'n', negates: true},
	{long: "force", short: 'f', negates: true},
	{short: 'k'},
	{long: "sparse", negates: true},
}

// checkoutOptions are the options of `git checkout -h` (git 2.55). --ours
// and --theirs take no --no- form.
var checkoutOptions = []gitOpt{
	{short: 'b', arg: argNeeded},
	{short: 'B', arg: argNeeded},
	{short: 'l'},
	{long: "guess", negates: true},
	{long: "overlay", negates: true},
	{long: "auto-advance", negates: true},
	{long: "quiet", short: 'q', negates: true},
	{long: "recurse-submodules", arg: argOptional, negates: true},
	{long: "progress", negates: true},
	{long: "merge", short: 'm', negates: true},
	{long: "conflict", arg: argNeeded, negates: true},
	{long: "detach", short: 'd', negates: true},
	{long: "track", short: 't', arg: argOptional, negates: true},
	{long: "force", short: 'f', negates: true},
	{long: "orphan", arg: argNeeded, negates: true},
	{long: "overwrite-ignore", negates: true},
	{long: "ignore-other-worktrees", negates: true},
	{long: "ours", short: '2'},
	{long: "theirs", short: '3'},
	{long: "patch", short: 'p', negates: true},
	{long: "unified", short: 'U', arg: argNeeded},
	{long: "inter-hunk-context", arg: argNeeded},
	{long: "ignore-skip-worktree-bits", negates: true},
	{long: "pathspec-from-file", arg: argNeeded, negates: true},
	{long: "pathspec-file-nul", negates: true},
}

// switchOptions are the options of `git switch -h` (git 2.55).
var switchOptions = []gitOpt{
	{long: "create", short: 'c', arg: argNeeded, negates: true},
	{long: "force-create", short: 'C', arg: argNeeded, negates: true},
	{long: "guess", negates: true},
	{long: "discard-changes", negates: true},
	{long: "quiet", short: 'q', negates: true},
	{long: "recurse-submodules", arg: argOptional, negates: true},
	{long: "progress", negates: true},
	{long: "merge", short: 'm', negates: true},
	{long: "conflict", arg: argNeeded, negates: true},
	{long: "detach", short: 'd', negates: true},
	{long: "track", short: 't', arg: argOptional, negates: true},
	{long: "force", short: 'f', negates: true},
	{long: "orphan", arg: argNeeded, negates: true},
	{long: "overwrite-ignore", negates: true},
	{long: "ignore-other-worktrees", negates: true},
}

// reflogDeleteOptions are the options of `git reflog delete -h` (git 2.55).
var reflogDeleteOptions = []gitOpt{
	{long: "dry-run", short: 'n', negates: true},
	{long: "rewrite", negates: true},
	{long: "updateref", negates: true},
	{long: "verbose", negates: true},
}

// reflogExpireOptions are the options of `git reflog expire -h` (git 2.55):
// those of delete and its own.
var reflogExpireOptions = slices.Concat(reflogDeleteOptions, []gitOpt{
	{long: "expire", arg: argNeeded},
	{long: "expire-unreachable", arg: argNeeded},
	{long: "stale-fix", negates: true},
	{long: "all", negates: true},
	{long: "single-worktree", negates: true},
})

// reflogDropOptions are the options of `git reflog drop -h` (git 2.55).
var reflogDropOptions = []gitOpt{
	{long: "all", negates: true},
	{long: "single-worktree", negates: true},
}

// updateRefOptions are the options of `git update-ref -h` (git 2.55).
// --no-deref and --deref are two entries, as git lists them.
var updateRefOptions = []gitOpt{
	{short: 'm', arg: argNeeded},
	{short: 'd'},
	{long: "no-deref"},
	{long: "deref"},
	{short: 'z'},
	{long: "stdin", negates: true},
	{long: "create-reflog", negates: true},
	{long: "batch-updates", short: '0', negates: true},
}

// gitOptRead is one option as git reads it from a word.
type gitOptRead struct {
	// name is the entry's gitOpt.name.
	name string
	// unset marks a --no- form.
	unset bool
	// word is the word it was read from.
	word string
	// value is the value it took: after =, the rest of a short bundle or
	// the next word. "" when it took none.
	value string
}

// asWord writes the option back as one word git reads the same way: -p1,
// --no-reverse, --directory=sub.
func (o gitOptRead) asWord() string {
	switch {
	case len(o.name) == 1:
		return "-" + o.name + o.value
	case o.unset:
		return "--no-" + o.name
	case o.value != "":
		return "--" + o.name + "=" + o.value
	}
	return "--" + o.name
}

// gitWords are a subcommand's words the way git reads them.
type gitWords struct {
	// opts are the options in the order git reads them, and optWords the
	// words they were read from, each value in a word of its own included.
	opts     []gitOptRead
	optWords []string
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

// on reports whether the last read of the named option sets it: git keeps
// the last of `--check --no-check`, and a --no-force resets a count.
func (w gitWords) on(name string) bool {
	on := false
	for _, o := range w.opts {
		if o.name == name {
			on = !o.unset
		}
	}
	return on
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
			w.optWords = append(w.optWords, a)
			i++
			a = args[i]
			read[len(read)-1].value = a
		}
		w.opts = append(w.opts, read...)
		w.optWords = append(w.optWords, a)
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
	name, value, valued := strings.Cut(text, "=")
	opt, unset, ok := resolveLong(name, table)
	switch {
	case !ok:
		return nil, false, fmt.Errorf("option %s", word)
	case valued && (unset || opt.arg == argNone):
		return nil, false, fmt.Errorf("option %s takes no value", word)
	}
	read = []gitOptRead{{name: opt.name(), unset: unset, word: word, value: value}}
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
			read[len(read)-1].value = word[i+1:]
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
