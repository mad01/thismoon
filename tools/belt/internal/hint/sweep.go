package hint

import (
	"os"
	"path/filepath"
	"strings"
)

// This file reads a Bash command line for prefer-csl: which segment is a
// multi-file search, what it searches for, and which directory it runs in.
// It runs nothing; every answer comes from the command text, plus a stat of
// a named file to tell a targeted read from a sweep.

// searchTools are the commands worth advising on. `ls` and `cat` are absent
// on purpose: they are not searches. `git grep` reaches here as one name.
var searchTools = map[string]bool{
	"grep": true, "rg": true, "ag": true, "find": true, "fd": true, "git grep": true,
}

// valueFlags are each tool's short flags that take the next word as their
// value, so that word is neither the pattern nor a path. Only the common
// ones: a missed flag shifts the parse and the hint names the wrong pattern,
// while a flag listed for the wrong tool would swallow a real operand.
var valueFlags = map[string]map[string]bool{
	"grep":     {"-A": true, "-B": true, "-C": true, "-e": true, "-m": true},
	"git grep": {"-A": true, "-B": true, "-C": true, "-e": true, "-m": true},
	"rg": {
		"-A": true, "-B": true, "-C": true, "-e": true, "-m": true,
		"-g": true, "-t": true, "-T": true, "-r": true, // rg -r is --replace
	},
	"ag": {"-A": true, "-B": true, "-C": true, "-m": true, "-G": true},
	"fd": {"-e": true, "-t": true, "-E": true, "-d": true},
}

// sweep is a parsed multi-file search: which tool, what pattern, where, and
// any file-name filter worth carrying into the zoekt query.
type sweep struct {
	tool    string
	pattern string // grep-likes: content regex; fd: file-name regex
	target  string
	include string // a file glob (--include, rg -g, find -name, fd -e) for a zoekt f: filter
}

// workDirs follows the directory a command's segments run in. A `cd` moves
// it for the segments after it, and a `cd` inside a subshell ends with the
// subshell. "" means the directory cannot be known without running the
// shell, and relative paths then resolve to nothing.
type workDirs struct {
	cur   string
	outer []string // directories to return to as subshells close
}

// step consumes one segment. It strips the subshell parentheses around the
// command, applies a `cd`, and returns the bare command with the directory
// it runs in; a `cd` returns "" as its command.
func (w *workDirs) step(seg string) (cmd, dir string) {
	s := strings.TrimSpace(seg)
	for strings.HasPrefix(s, "(") {
		w.outer = append(w.outer, w.cur)
		s = strings.TrimSpace(s[1:])
	}
	closes := 0
	for strings.HasSuffix(s, ")") && !strings.HasSuffix(s, `\)`) {
		closes++
		s = strings.TrimSpace(s[:len(s)-1])
	}
	cmd, dir = s, w.cur
	if target, ok := cdTarget(s, w.cur); ok {
		w.cur = target
		cmd = ""
	}
	for ; closes > 0 && len(w.outer) > 0; closes-- {
		w.cur = w.outer[len(w.outer)-1]
		w.outer = w.outer[:len(w.outer)-1]
	}
	return cmd, dir
}

// cdTarget reports whether seg is a `cd`, and the directory it moves to: ""
// when that cannot be known without running the shell (`cd -`, a variable).
func cdTarget(seg, dir string) (string, bool) {
	fields := splitFields(seg)
	if len(fields) == 0 || fields[0] != "cd" {
		return "", false
	}
	args := fields[1:]
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "-" {
		args = args[1:] // -P, -L, --
	}
	switch {
	case len(args) == 0:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", true
		}
		return home, true
	case args[0] == "-":
		return "", true
	}
	return resolve(args[0], dir), true
}

// segments splits a command on the separators that start a new command, so a
// grep after `&&` is seen. Anything after a pipe is skipped: `cmd | grep x`
// is a filter over another command's output, which csl cannot replace and
// which the user's conventions explicitly allow.
//
// This deliberately does not reuse guard.splitSegments. That one preserves
// byte offsets and treats `|` exactly like `&&`, which is right for finding
// a denied command anywhere in a pipeline and wrong here: telling a pipe
// filter apart from a standalone search is the entire precision requirement
// of this hint. Quote awareness matters for the same reason: a `|` inside a
// grep pattern must not read as a pipe.
func segments(cmd string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	skip := false // inside a pipeline's downstream stage
	flush := func(isPipe bool) {
		if !skip {
			out = append(out, cur.String())
		}
		cur.Reset()
		skip = isPipe
	}
	runes := []rune(cmd)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case quote != 0:
			if c == quote && (i == 0 || runes[i-1] != '\\') {
				quote = 0
			}
			cur.WriteRune(c)
		case c == '\'' || c == '"':
			quote = c
			cur.WriteRune(c)
		case c == '|':
			// `||` starts a new command; a single `|` starts a pipe stage.
			if i+1 < len(runes) && runes[i+1] == '|' {
				i++
				flush(false)
				continue
			}
			flush(true)
		case c == '&' && i+1 < len(runes) && runes[i+1] == '&':
			i++
			flush(false)
		case c == ';' || c == '\n':
			flush(false)
		default:
			cur.WriteRune(c)
		}
	}
	flush(false)
	return out
}

// splitFields splits a segment into shell words, keeping quoted spans whole
// so a pattern like "func Foo" stays one field. Quotes are left in place;
// trimQuotes strips them where the value is used.
func splitFields(seg string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	for _, c := range seg {
		switch {
		case quote != 0:
			cur.WriteRune(c)
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
			cur.WriteRune(c)
		case c == ' ' || c == '\t':
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// trimQuotes removes one matching pair of surrounding quotes.
func trimQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// sweepArgs is a search command's arguments sorted by role.
type sweepArgs struct {
	recursive bool
	patterns  []string // from -e, joined as zoekt alternation
	include   string
	operands  []string
}

// parseSweep returns a sweep when seg is a multi-file search run in cwd, or
// nil when it is anything else. That includes a grep of one or two named
// files, which is filtering a known file and exactly what csl is not for.
func parseSweep(seg, cwd string) *sweep {
	fields := splitFields(seg)
	if len(fields) == 0 {
		return nil
	}
	tool, args := filepath.Base(fields[0]), fields[1:]
	if tool == "git" {
		var ok bool
		if args, cwd, ok = gitGrepArgs(args, cwd); !ok {
			return nil
		}
		tool = "git grep"
	}
	if !searchTools[tool] {
		return nil
	}

	a := parseArgs(tool, args)
	pattern, paths, ok := splitOperands(tool, a)
	if !ok {
		return nil
	}
	sw := &sweep{tool: tool, pattern: pattern, include: a.include}
	if len(paths) == 0 {
		// A recursive search with no path argument sweeps its directory.
		if !a.recursive {
			return nil
		}
		paths = []string{cwd}
	}
	// rg and ag recurse into a directory operand without -r. With no operand
	// they may read stdin instead, so only named paths count as a walk.
	walks := a.recursive || tool == "rg" || tool == "ag"
	if !spansManyFiles(tool, walks, paths, cwd) {
		return nil
	}
	sw.target = resolve(paths[0], cwd)
	if sw.target == "" {
		return nil
	}
	return sw
}

// splitOperands separates the pattern from the paths. grep-likes and fd take
// the pattern first, then paths; find takes paths only. A pattern given with
// -e leaves every operand a path. ok is false when nothing searched the
// filesystem.
func splitOperands(tool string, a sweepArgs) (pattern string, paths []string, ok bool) {
	paths = a.operands
	switch {
	case len(a.patterns) > 0:
		return strings.Join(a.patterns, "|"), paths, true
	case tool == "find":
		return "", paths, len(paths) > 0 // macOS find needs a path
	case len(paths) == 0:
		return "", nil, tool == "fd" // a bare fd lists its directory; grep reads stdin
	}
	return trimQuotes(paths[0]), paths[1:], true
}

// parseArgs sorts a search tool's arguments into flags that matter, the
// pattern given with -e, and the remaining operands.
func parseArgs(tool string, args []string) sweepArgs {
	// find, fd and git grep walk trees by definition.
	a := sweepArgs{recursive: tool == "find" || tool == "fd" || tool == "git grep"}
	values := valueFlags[tool]
	for i := 0; i < len(args); i++ {
		f := args[i]
		switch {
		case tool == "find" && (f == "-name" || f == "-iname"):
			if i+1 < len(args) {
				i++
				a.include = trimQuotes(args[i])
			}
		case values[f]:
			if i+1 >= len(args) {
				continue
			}
			i++
			a.flagValue(tool, f, trimQuotes(args[i]))
		case strings.HasPrefix(f, "--include="):
			a.include = trimQuotes(strings.TrimPrefix(f, "--include="))
			a.recursive = true
		case f == "-r" || f == "-R":
			a.recursive = true
		case strings.HasPrefix(f, "-") && len(f) > 1 && !strings.HasPrefix(f, "--"):
			// Bundled short flags: -rn, -rln, -ril and friends.
			if strings.ContainsAny(f[1:], "rR") {
				a.recursive = true
			}
		case strings.HasPrefix(f, "--"):
			// Other long flags carry no signal for this decision.
		default:
			a.operands = append(a.operands, f)
		}
	}
	return a
}

// flagValue records the value of a flag that matters for the query: a
// pattern given with -e, or a file filter given with rg -g or fd -e.
func (a *sweepArgs) flagValue(tool, flag, value string) {
	switch {
	case tool == "fd" && flag == "-e":
		a.include = "*." + value
	case flag == "-e":
		a.patterns = append(a.patterns, value)
	case tool == "rg" && flag == "-g":
		a.include = value
	}
}

// gitGrepArgs reads a git command's global options up to its subcommand,
// following each -C into the directory it names. It reports whether the
// subcommand is grep, with grep's own arguments and the directory git runs
// in.
func gitGrepArgs(args []string, cwd string) ([]string, string, bool) {
	dir := cwd
	for i := 0; i < len(args); i++ {
		switch f := args[i]; {
		case f == "-C" || f == "-c":
			if i+1 >= len(args) {
				return nil, "", false
			}
			i++
			if f == "-C" {
				dir = resolve(args[i], dir)
			}
		case strings.HasPrefix(f, "-"):
			// --no-pager and the other global flags take no separate value.
		case f == "grep":
			return args[i+1:], dir, true
		default:
			return nil, "", false
		}
	}
	return nil, "", false
}

// spansManyFiles reports whether the path operands make the command a sweep
// rather than a look at one or two named files. A glob or a third path is a
// sweep. So is a directory under a recursive search; a path that does not
// exist counts as one, because a recursive search names directories.
func spansManyFiles(tool string, recursive bool, paths []string, cwd string) bool {
	if tool == "find" || tool == "fd" {
		return true
	}
	if len(paths) > 2 {
		return true
	}
	for _, p := range paths {
		if strings.ContainsAny(p, "*?[") {
			return true
		}
	}
	if !recursive {
		return false // grep without -r reads the named files only
	}
	for _, p := range paths {
		if !isRegularFile(resolve(p, cwd)) {
			return true
		}
	}
	return false
}

// isRegularFile reports whether p names an existing regular file.
func isRegularFile(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// resolve makes p absolute against cwd. It returns "" when the path cannot
// be known without running the shell: a variable, a command substitution,
// another user's home, or a relative path with no known cwd.
func resolve(p, cwd string) string {
	p = trimQuotes(p)
	if p == "" || strings.ContainsAny(p, "$`") {
		return ""
	}
	if strings.HasPrefix(p, "~") {
		if p != "~" && !strings.HasPrefix(p, "~/") {
			return "" // ~user
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	if filepath.IsAbs(p) {
		return p
	}
	if cwd == "" {
		return ""
	}
	return filepath.Join(cwd, p)
}
