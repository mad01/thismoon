package guard

import (
	"cmp"
	"slices"
	"strings"
)

// This file reads what a git command prints as text belt cannot read when a
// shell runs it as code. Most of it comes from what the repo stores (a
// commit message, a file at a commit, a config value, a note, a ref name), and
// a commit, a config write or a note earlier in the same command may have set
// that to anything, so it counts as a download does (unreadableSource): `git
// log -1 --format=%B | sh`, `sh -c "$(git show HEAD:x.sh)"`, `eval "$(git
// config --get x.y)"`, `bash <(git cat-file -p HEAD:x.sh)`. Only the git calls
// that print hashes, counts or the repo's own paths pass (gitPrintsPlain).
// Output a command takes as a value runs nothing (`git checkout "$(git
// rev-parse --abbrev-ref HEAD)"`, `git commit -m "$(git log -1 --format=%B)"`).

// revListText are the rev-list options that make it print commit text
// besides the hashes: a format, a pretty format, --oneline and --header.
var revListText = []string{"--format", "--pretty", "--oneline", "--header"}

// diffNames are the diff options that make it print paths alone: git drops
// the patch and the stat for them, wherever they stand.
var diffNames = wordSet("--name-only --name-status")

// storedSource returns what a git call prints that belt cannot read as code,
// or "" for a git call whose output it reads as plain (gitPrintsPlain) and
// for any other command. A call whose subcommand belt cannot tell (an alias,
// a variable, none at all) prints text it cannot read.
func storedSource(tokens []string) string {
	words, _ := unwrapCommand(tokens)
	if len(words) == 0 || commandName(words[0]) != "git" {
		return ""
	}
	c, ok := parseGitCmd(words[1:], "")
	switch {
	case !ok:
		return "text git prints"
	case gitPrintsPlain(c):
		return ""
	}
	return "text git " + c.sub + " prints from the repo"
}

// gitPrintsPlain reports whether a git call prints only hashes, counts or
// the repo's own paths: rev-parse, merge-base, ls-files and ls-tree, a
// rev-list with no revListText option and a diff with a diffNames one. git
// never lets an alias take a builtin's name, so these words always run the
// builtin.
func gitPrintsPlain(c gitInvocation) bool {
	switch c.sub {
	case "rev-parse", "merge-base", "ls-files", "ls-tree":
		return true
	case "rev-list":
		return !revListPrintsText(c.args)
	case "diff":
		return slices.ContainsFunc(c.args, func(a string) bool { return diffNames[removeQuotes(a)] })
	}
	return false
}

// revListPrintsText reports whether rev-list arguments may make it print
// commit text: a revListText option, with its quotes read past, or a word
// only a shell can expand, which may hold one.
func revListPrintsText(args []string) bool {
	for _, a := range args {
		if strings.ContainsAny(a, "$`") {
			return true
		}
		a = removeQuotes(a)
		for _, opt := range revListText {
			if a == opt || strings.HasPrefix(a, opt+"=") {
				return true
			}
		}
	}
	return false
}

// commandSource returns what one command, read from text, prints that belt
// cannot read as code: what it prints itself (unreadableSource) or what a
// substitution or subshell in it prints (printedSource), as
// codeWalk.stageSource reads a stage with nothing held or saved.
func commandSource(text string, c shellCmd) string {
	src := unreadableSource(c.tokens())
	for _, s := range c.subshells {
		src = cmp.Or(src, printedSource(text[s.from:s.to]))
	}
	for _, p := range c.pieces() {
		src = cmp.Or(src, printedSource(p.code(text)))
	}
	return src
}

// printedSource returns what the commands of a substitution body print that
// belt cannot read as code (commandSource), from any of them.
func printedSource(body string) string {
	cmds, ok := readShell(body)
	if !ok {
		return ""
	}
	var src string
	for _, c := range cmds {
		src = cmp.Or(src, commandSource(body, c))
	}
	return src
}

// sourceOpacity adds to what a command runs that the walk could not read
// what a substitution whose output it runs as code prints that belt cannot
// read (printedSource over walkStep.unreadSubst): shell text, which the push
// guard denies in a repo it guards.
func sourceOpacity(o opacity, bodies []string) opacity {
	for _, body := range bodies {
		if o.what != "" {
			break
		}
		if src := printedSource(body); src != "" {
			o = opacity{what: src, scoped: true}
		}
	}
	return o
}
