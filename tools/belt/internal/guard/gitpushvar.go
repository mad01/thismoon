package guard

import (
	"os"
	"strings"
)

// branchVars returns the names a command sets to the current branch in the
// one shape where a refspec "$NAME" is sure to hold that branch when the push
// runs: `B=$(git branch --show-current); git push -u origin "$B"`. The
// assignment is a top-level command of its own (branchAssignment), NAME
// appears nowhere but there and, after it, as whole push refspecs
// (onlyRefspecUses), nothing in the command can set a variable again
// (canSetVars), and nothing moves the shell, so the substitution ran where
// the push runs. An assignment after && may not run, so NAME must then be
// unset in the environment: an unset NAME leaves the refspec empty, which
// pushes the current branch or fails. pushes are the command's pushes, for
// the refspec count.
func branchVars(command string, pushes []gitPush) map[string]bool {
	refs := refspecVars(pushes)
	if len(refs) == 0 {
		return nil
	}
	cmds, ok := readShell(command)
	if !ok || mentionsName(command, shellMoveWords) || canSetVars(command) {
		return nil
	}
	vars := map[string]bool{}
	for _, c := range cmds {
		name, ok := branchAssignment(c)
		if !ok || !onlyRefspecUses(command, c.words[0].at.to, name, refs[name]) {
			continue
		}
		if _, set := os.LookupEnv(name); c.before == jointAnd && set {
			continue
		}
		vars[name] = true
	}
	return vars
}

// shellMoveWords are the commands that move the shell's directory.
var shellMoveWords = wordSet("cd pushd popd chdir")

// branchAssignment returns NAME when the command is NAME=<a current-branch
// substitution> (currentBranchWord) and nothing else: one word, no
// redirection or subshell, joined to its neighbours by the start or end of
// the command, a ; or newline, or &&, and a name the shell does not manage
// itself (shellManagedNames).
func branchAssignment(c shellCmd) (string, bool) {
	if len(c.words) != 1 || len(c.targets) > 0 || len(c.subshells) > 0 ||
		!sequential(c.before) || !sequential(c.after) {
		return "", false
	}
	name, value, ok := strings.Cut(c.words[0].raw, "=")
	if !ok || !isShellName(name) || shellManagedNames[name] || !currentBranchWord(value) {
		return "", false
	}
	return name, true
}

// sequential reports whether a joint runs the commands on both sides in this
// shell, one after the other: the start or end, a ; or newline, or &&.
func sequential(j joint) bool {
	return j == jointList || j == jointAnd
}

// onlyRefspecUses reports whether name stands as a word (nameWords) only once
// before end, in its assignment, and everywhere else only as one of the refs
// push refspecs that are exactly a reference to it.
func onlyRefspecUses(command string, end int, name string, refs int) bool {
	return nameWords(command[:end], name) == 1 && nameWords(command, name) == 1+refs
}

// canSetVars reports whether a command the shell runs, nested ones included,
// is a shellDenyWords word that can set a variable or run hidden code (read,
// export, eval, source, a loop or if keyword), a `.` with a file, or a brace
// that opens or closes a group. Comments and prose bodies run nothing.
func canSetVars(command string) bool {
	found := false
	walkCommands(command, "", func(s walkStep) {
		if s.prose {
			return
		}
		for i, tok := range s.tokens {
			found = found || shellDenyWords[tok] || tok == "{" || tok == "}" ||
				(tok == "." && i+1 < len(s.tokens))
		}
	})
	return found
}

// refspecVars counts, per name, the push refspecs that are exactly a
// reference to that variable (refspecVar).
func refspecVars(pushes []gitPush) map[string]int {
	refs := map[string]int{}
	for _, p := range pushes {
		for _, spec := range p.refspecs {
			if name, ok := refspecVar(spec); ok {
				refs[name]++
			}
		}
	}
	return refs
}

// refspecVar returns the name a refspec token references when it is exactly
// $NAME or ${NAME}, bare or in double quotes.
func refspecVar(spec string) (string, bool) {
	if inner, ok := strings.CutPrefix(spec, `"`); ok {
		if spec, ok = strings.CutSuffix(inner, `"`); !ok {
			return "", false
		}
	}
	name, n := varRef(spec)
	return name, n > 0 && n == len(spec)
}
