package guard

import (
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// This file places the two directory words agents write for "here": $PWD
// and $(git rev-parse --show-toplevel). The cd tracking (shellWalk.move) and
// a git -C target (parseGitCmd) both read them.

// pwdWords are the spellings of the variable that holds the shell's own
// directory.
var pwdWords = []string{"${PWD}", "$PWD"}

// pwdNames is that variable's name, for mentionsName.
var pwdNames = wordSet("PWD")

// pwdRef matches a plain reference to it: $PWD or ${PWD}.
var pwdRef = regexp.MustCompile(`\$PWD\b|\$\{PWD\}`)

// rewritesPWD reports whether text names PWD anywhere but in a plain
// reference: an assignment, export, read or unset may give it a value that
// is not the shell's directory, so belt places no $PWD after it.
func rewritesPWD(text string) bool {
	return mentionsName(pwdRef.ReplaceAllString(text, ""), pwdNames)
}

// hereKnown reports whether a word naming where the shell is (namesHere) in
// the text the walk reads can be placed from w.dir. It can only in the
// top-level command text, and only while nothing in the command may have
// set PWD (pwdSet) or run a command that may move the shell where the walk
// does not see it (cdHidden, keepsShellPlaced). Nested script text keeps it unknown: a subshell, a -c script,
// an eval, a heredoc body or anything else a child walk reads. The outer
// shell may expand the word there before the script's own cd runs, so belt
// does not guess which shell expands it.
func (w *shellWalk) hereKnown() bool {
	return !w.pwdSet && !w.cdHidden && w.depth == 0 && !w.prose
}

// namesHere reports whether text holds a word expandShellDir places from the
// shell's directory: PWD, or a --show-toplevel substitution.
func namesHere(text string) bool {
	return mentionsName(text, pwdNames) || strings.Contains(text, "--show-toplevel")
}

// keepsShellPlaced reports whether a command leaves the shell where the walk
// can place it: a git call or a readOnlyCommand other than printf, behind
// wrappers, that defines no function. Any other command may move the shell
// where the walk does not see it: a function it calls, a builtin such as
// trap, source, emulate or setopt, a bare directory word under zsh's autocd.
// printf -v sets a variable whose name may be built at run time, PWD
// included. The walk tracks cd, pushd and popd itself (move), so they never
// get here.
func keepsShellPlaced(defines bool, tokens []string) bool {
	words := stripWrappers(tokens)
	if defines || len(words) == 0 || path.Base(words[0]) == "printf" {
		return false
	}
	return isGitWord(words[0]) || readOnlyCommand(words)
}

// shellSideEffect matches an expansion that may run code or assign a
// variable in this shell from inside a word, so a cd or a PWD set there
// counts for every word after it, in any command of the text: any ${...}
// (bash 5.3's `${ cmd; }`, a := or ::= default, zsh's (P) flag on a name
// built at run time), and $((...)) or $[...] arithmetic.
var shellSideEffect = regexp.MustCompile(`\$\{|\$\(\(|\$\[`)

// expandsInShell reports whether text holds a shellSideEffect expansion
// other than a plain ${PWD}, which only reads.
func expandsInShell(text string) bool {
	return shellSideEffect.MatchString(strings.ReplaceAll(text, "${PWD}", ""))
}

// expandShellDir expands a directory target that starts with a word naming
// where the shell is: $PWD or ${PWD}, the directory the walk holds there
// (shellDir), and $(git rev-parse --show-toplevel), with or without a -C,
// the top of the repository git finds from there (shellToplevel). The path
// after the word stays as written. A target that starts with neither comes
// back as written. One whose word belt cannot place (an unknown shellDir, no
// repository, a value the shell would split unquoted) comes back "", which
// every directory parser reads as unknown. Single quotes keep the word
// literal, as the shell does.
func expandShellDir(shellDir, target string) string {
	quote, inner := unquote(target)
	if quote == '\'' {
		return target
	}
	value, rest, ok := shellDirWord(shellDir, inner)
	switch {
	case !ok:
		return target
	case value == "" || (quote == 0 && strings.IndexFunc(value, unicode.IsSpace) >= 0):
		return ""
	}
	return value + rest
}

// shellDirWord splits text into the expandShellDir word it starts with,
// expanded ("" when unknown), and the path after it. ok is false when text
// starts with no such word, or the word runs on into a longer one ($PWDX).
func shellDirWord(shellDir, text string) (value, rest string, ok bool) {
	for _, v := range pwdWords {
		if rest, found := strings.CutPrefix(text, v); found && pathTail(rest) {
			return shellDir, rest, true
		}
	}
	body, found := strings.CutPrefix(text, "$(")
	if !found {
		return "", "", false
	}
	body, rest, found = strings.Cut(body, ")")
	if !found || !pathTail(rest) {
		return "", "", false
	}
	from, found := toplevelFrom(shellDir, strings.Fields(body))
	if !found {
		return "", "", false
	}
	return shellToplevel(from), rest, true
}

// pathTail reports whether rest may follow a directory word: nothing, or a
// path below it.
func pathTail(rest string) bool {
	return rest == "" || rest[0] == '/'
}

// toplevelFrom reads the words of a substitution as `git rev-parse
// --show-toplevel` or `git -C <dir> rev-parse --show-toplevel` and returns
// the directory git starts from: shellDir, or dir placed under it the way a
// -C target is (placeDir). ok is false for any other command.
func toplevelFrom(shellDir string, words []string) (dir string, ok bool) {
	tail := []string{"rev-parse", "--show-toplevel"}
	switch {
	case slices.Equal(words, append([]string{"git"}, tail...)):
		return shellDir, true
	case len(words) == 5 && words[0] == "git" && words[1] == "-C" &&
		slices.Equal(words[3:], tail):
		return placeDir(shellDir, words[2]), true
	}
	return "", false
}

// shellToplevel returns the top of the work tree git finds from dir, as
// `git rev-parse --show-toplevel` prints it (symlinks resolved), or "" when
// belt cannot tell: an unknown dir, a git variable in belt's environment
// that points git elsewhere, a layout discoverRepo gives no answer for, or a
// top with a control character in it: the shell strips trailing newlines
// from a substitution, so it would land somewhere else. It reads the files
// git would and runs no git.
func shellToplevel(dir string) string {
	if dir == "" || gitEnvRedirected() {
		return ""
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return ""
	}
	repo, ok := discoverRepo(real)
	if !ok || strings.ContainsFunc(repo.top, unicode.IsControl) {
		return ""
	}
	return repo.top
}
