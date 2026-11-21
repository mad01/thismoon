package guard

import (
	"slices"
	"strings"
)

// reflogDrops reports whether a reflog call may delete stash entries, which
// live in the reflog of refs/stash: a delete of one of its entries, or an
// expire or a drop of that reflog or of --all, unless it is a dry run. The
// options are read as git reads them (readGitWords), and words belt cannot
// read count as a delete; so do a ref and a subcommand a command in front
// of git may hand over (fed). The subcommand is read with its quotes gone,
// and a first word the shell may still rewrite (mayExpand) may be any. show,
// list, exists and write keep the entries.
func reflogDrops(args []string, fed bool) bool {
	if len(args) == 0 {
		return fed
	}
	var table []gitOpt
	switch unquoteWord(args[0]) {
	case "delete":
		table = reflogDeleteOptions
	case "expire":
		table = reflogExpireOptions
	case "drop":
		table = reflogDropOptions
	default:
		return !strings.HasPrefix(args[0], "-") && mayExpand(args[0])
	}
	w, err := readGitWords(args[1:], table)
	switch {
	case err != nil || fed:
		return true
	case w.on("dry-run"):
		return false
	}
	return w.on("all") || slices.ContainsFunc(slices.Concat(w.operands, w.paths), namesStash)
}

// updateRefDrops reports whether an update-ref call may delete refs/stash,
// and every stash entry with its reflog: -d on a ref that may be it
// (namesStash), or --stdin, whose commands belt never sees. The options are
// read as git reads them (readGitWords), and words belt cannot read count
// as a delete; so do words a command in front of git may hand over (fed).
func updateRefDrops(args []string, fed bool) bool {
	w, err := readGitWords(args, updateRefOptions)
	switch {
	case err != nil || fed || w.on("stdin"):
		return true
	case !w.sets("d"):
		return false
	}
	return slices.ContainsFunc(slices.Concat(w.operands, w.paths), namesStash)
}

// namesStash reports whether a reflog or ref word may name refs/stash:
// stash or refs/stash, in any case (a case-insensitive file system finds
// the ref either way), with or without an @{...} entry, or a word whose ref
// part the shell may still rewrite (mayExpand).
func namesStash(word string) bool {
	ref, _, _ := strings.Cut(word, "@{")
	if mayExpand(ref) {
		return true
	}
	ref = unquoteWord(ref)
	return strings.EqualFold(ref, "stash") || strings.EqualFold(ref, "refs/stash")
}

// mayExpand reports whether the shell may hand git another word than the
// one written, once its quotes and backslashes are gone: an expansion, a
// glob, or a brace list or range. A brace with neither, as in HEAD@{1}, is
// plain text.
func mayExpand(word string) bool {
	if strings.ContainsAny(word, "$`*?[(") {
		return true
	}
	for rest := word; ; {
		_, after, open := strings.Cut(rest, "{")
		body, tail, closed := strings.Cut(after, "}")
		switch {
		case !open || !closed:
			return false
		case strings.Contains(body, ",") || strings.Contains(body, ".."):
			return true
		}
		rest = tail
	}
}

// hiddenGit reads a git call whose subcommand belt cannot see, which may
// then be any: one that names none but has a command in front of git that
// may hand it one (fedGit), one whose subcommand word only a shell can
// resolve (shellSubcommand), and one that runs text belt cannot read, such
// as an alias body (walkStep.opaque, outside scoped text). drops says it may
// delete stash entries too: a fed one always, since belt never sees its
// words; a shell word when a drop or clear follows it, or no plain word does
// (shellWordDrops); an alias when its body may (aliasMayDropStash), and any
// other text belt cannot read. Text no shell runs hides nothing.
func hiddenGit(s walkStep, dir string) (c gitInvocation, drops, hidden bool) {
	if s.prose {
		return gitInvocation{}, false, false
	}
	c, parsed := segmentGitCommand(s.tokens, dir)
	switch {
	case s.opaque.what != "" && !s.opaque.scoped:
		if !parsed {
			c = gitInvocation{dir: dir}
		}
		return c, !parsed || aliasMayDropStash(c), true
	case !parsed:
		c, hidden = fedGit(s.tokens, dir)
		return c, hidden, hidden
	case shellSubcommand(c):
		return c, shellWordDrops(c.args), true
	}
	return c, false, false
}

// fedGit reads a git call that names no subcommand while a command in front
// of git may hand it one (fedOperands): `echo stash clear | xargs git`. Its
// global options are read as parseGitCmd reads them, with the subcommand
// left "", and one whose options belt cannot read runs where belt cannot
// tell.
func fedGit(tokens []string, dir string) (gitInvocation, bool) {
	at := slices.IndexFunc(tokens, isGitWord)
	if at < 0 || !fedOperands(tokens) {
		return gitInvocation{}, false
	}
	if c, ok := parseGitCmd(append(slices.Clone(tokens[at+1:]), ""), dir); ok {
		return c, true
	}
	return gitInvocation{}, true
}

// shellWordDrops reports whether a git call whose subcommand word only a
// shell can resolve may be a stash drop: a drop or clear word follows it,
// or no plain word does (`git $X`), since the shell word may then hold all
// of `stash clear`.
func shellWordDrops(args []string) bool {
	return !slices.ContainsFunc(args, plainRefspec) || slices.ContainsFunc(args, func(a string) bool {
		word := unquoteWord(a)
		return word == "drop" || word == "clear"
	})
}

// aliasMayDropStash reports whether an alias call whose body belt cannot
// read may delete stash entries: unless every body the call's own -c sets
// for it (gitInvocation.config) keeps them (bodyKeepsStash). A body set any
// other way is one belt cannot see.
func aliasMayDropStash(c gitInvocation) bool {
	name := strings.ToLower(c.sub)
	seen := false
	for _, value := range c.config {
		key, body, set := strings.Cut(unquoteWord(value), "=")
		if alias, ok := aliasKeyName(key); !ok || alias != name {
			continue
		}
		if !set || !bodyKeepsStash(body) {
			return true
		}
		seen = true
	}
	return !seen
}

// stashTouching are the builtins a call of which may delete stash entries
// (stashDrops).
var stashTouching = wordSet("stash reflog update-ref")

// bodyKeepsStash reports whether an alias body, as the shell hands it to
// git, keeps the stash entries whatever its expansions hold. git splits a
// body that does not start with ! into words without a shell, so one led
// by a plain builtin other than the stashTouching ones runs that builtin. A
// lead the shell may still rewrite, a global option, a ! body or another
// alias may run anything.
func bodyKeepsStash(body string) bool {
	words := strings.Fields(body)
	if len(words) == 0 {
		return false
	}
	lead := words[0]
	return asWritten(lead) && !strings.HasPrefix(lead, "-") && gitBuiltins[lead] &&
		!stashTouching[lead]
}
