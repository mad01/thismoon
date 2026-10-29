package hint

import (
	"os"
	"path/filepath"
	"regexp"
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

// valueFlags are each tool's flags that take a value, so the word after one
// is neither the pattern nor a path. A long flag also takes its value
// attached as --flag=value, and a short one as -C3. Only the common ones: a
// missed flag shifts the parse and the hint names the wrong pattern, while
// a flag listed for the wrong tool would swallow a real operand.
var valueFlags = map[string]map[string]bool{
	"grep": {
		"-A": true, "-B": true, "-C": true, "-e": true, "-m": true,
		"--include": true, "--exclude": true, "--exclude-dir": true, "--regexp": true,
	},
	"git grep": {"-A": true, "-B": true, "-C": true, "-e": true, "-m": true},
	"rg": {
		"-A": true, "-B": true, "-C": true, "-e": true, "-m": true,
		"-g": true, "-t": true, "-T": true, "-r": true, // rg -r is --replace
		"--glob": true, "--iglob": true, "--type": true, "--type-not": true, "--regexp": true,
	},
	"ag": {"-A": true, "-B": true, "-C": true, "-m": true, "-G": true},
	"fd": {
		"-e": true, "-t": true, "-E": true, "-d": true,
		"--extension": true, "--type": true, "--exclude": true,
	},
}

// dialect is the regex language a search tool reads its pattern in.
type dialect int

const (
	// dialectExtended is ERE, PCRE or Rust regex: close enough to the RE2
	// zoekt takes that the pattern passes through.
	dialectExtended dialect = iota
	// dialectBasic is grep's default BRE, where ( ) { } | + ? are literal
	// and their backslashed forms are the operators.
	dialectBasic
	// dialectFixed is a fixed string (-F): every character is literal.
	dialectFixed
)

// grepDialects are the grep and git grep flags that pick the regex language.
var grepDialects = map[string]dialect{
	"-G": dialectBasic, "--basic-regexp": dialectBasic,
	"-E": dialectExtended, "--extended-regexp": dialectExtended,
	"-P": dialectExtended, "--perl-regexp": dialectExtended,
	"-F": dialectFixed, "--fixed-strings": dialectFixed,
}

// dialectFlags are each tool's flags that change how it reads the pattern.
// A short flag counts inside a bundle like -rnF too.
var dialectFlags = map[string]map[string]dialect{
	"grep":     grepDialects,
	"git grep": grepDialects,
	"rg":       {"-F": dialectFixed, "--fixed-strings": dialectFixed},
	"ag": {
		"-Q": dialectFixed, "--literal": dialectFixed,
		"-F": dialectFixed, "--fixed-strings": dialectFixed,
	},
	"fd": {"-F": dialectFixed, "--fixed-strings": dialectFixed},
}

// defaultDialect is the regex language a tool reads with no flag: grep and
// git grep take a BRE, the others an extended regex.
func defaultDialect(tool string) dialect {
	if tool == "grep" || tool == "git grep" {
		return dialectBasic
	}
	return dialectExtended
}

// findOptions are the BSD find options that may come before the paths.
var findOptions = map[string]bool{
	"-H": true, "-L": true, "-P": true, "-E": true, "-X": true, "-d": true, "-s": true, "-x": true,
}

// xargsValueFlags are the xargs options that take the next word as a value.
var xargsValueFlags = map[string]bool{
	"-n": true, "-I": true, "-J": true, "-L": true, "-P": true,
	"-R": true, "-S": true, "-s": true, "-E": true, "-d": true,
}

// redirectOp matches a word that starts with a shell redirection: an
// optional descriptor number, then the operator, then any target written
// against it (2>/dev/null, 2>&1, <file, &>log).
var redirectOp = regexp.MustCompile(`^(?:[0-9]*(?:>>|>\||>&|>|<<<|<<|<>|<&|<)|&>>?)`)

// sweep is a parsed multi-file search: which tool, what it looks for, and
// where.
type sweep struct {
	tool    string
	walks   bool     // it descends into directories, rather than reading named files
	pattern string   // the content pattern as RE2; "" for a file listing
	name    string   // fd's file-name pattern as RE2
	include string   // a file glob (--include, rg -g, find -name, fd -e) for a zoekt f: filter
	targets []string // the searched paths, resolved, in command order
}

// segment is one command of a command line, with the first pipe stage that
// reads its output ("" when the output is not piped).
type segment struct {
	cmd  string
	pipe string
}

// workDirs follows the directory a command's segments run in. A `cd` or
// `pushd` moves it for the segments after it, `popd` moves it back, and a
// `cd` inside a subshell ends with the subshell. "" means the directory
// cannot be known without running the shell, and relative paths then
// resolve to nothing.
type workDirs struct {
	cur    string
	outer  []string // directories to return to as subshells close
	pushed []string // the pushd stack, for popd
}

// step consumes one segment. It strips the subshell parentheses and group
// braces around the command, applies a directory change, and returns the
// bare command with the directory it runs in; a directory change returns ""
// as its command.
func (w *workDirs) step(seg string) (cmd, dir string) {
	s := w.open(seg)
	closes := 0
	for strings.HasSuffix(s, ")") && !strings.HasSuffix(s, `\)`) {
		closes++
		s = strings.TrimSpace(s[:len(s)-1])
	}
	cmd, dir = s, w.cur
	if w.move(s) {
		cmd = ""
	}
	for ; closes > 0 && len(w.outer) > 0; closes-- {
		w.cur = w.outer[len(w.outer)-1]
		w.outer = w.outer[:len(w.outer)-1]
	}
	return cmd, dir
}

// open strips the subshell parentheses and group braces a segment starts
// with. A subshell saves the directory to return to when it closes. A { }
// group runs in the current shell, so its braces only need removing and a
// `cd` inside one stays in effect after it.
func (w *workDirs) open(seg string) string {
	s := strings.TrimSpace(seg)
	for {
		switch {
		case strings.HasPrefix(s, "("):
			w.outer = append(w.outer, w.cur)
		case startsWithBrace(s):
		default:
			return s
		}
		s = strings.TrimSpace(s[1:])
	}
}

// startsWithBrace reports whether s starts with { or } as a word of its
// own, which is how the shell reads a group brace.
func startsWithBrace(s string) bool {
	if s == "" || (s[0] != '{' && s[0] != '}') {
		return false
	}
	return len(s) == 1 || s[1] == ' ' || s[1] == '\t'
}

// move applies seg to the directory when it is a cd, pushd or popd, and
// reports whether it was one.
func (w *workDirs) move(seg string) bool {
	words := splitFields(seg)
	if len(words) == 0 {
		return false
	}
	args := dropRedirects(words[1:])
	switch words[0] {
	case "cd":
		w.cur = cdTarget(args, w.cur)
	case "pushd":
		w.pushed = append(w.pushed, w.cur)
		w.cur = pushdTarget(args, w.cur)
	case "popd":
		w.cur = ""
		if n := len(w.pushed); n > 0 {
			w.cur, w.pushed = w.pushed[n-1], w.pushed[:n-1]
		}
	default:
		return false
	}
	return true
}

// cdTarget reads the directory a `cd` moves to: "" when that cannot be
// known without running the shell (`cd -`, a variable).
func cdTarget(args []string, dir string) string {
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "-" {
		args = args[1:] // -P, -L, --
	}
	switch {
	case len(args) == 0:
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return home
	case args[0] == "-":
		return ""
	}
	return resolve(args[0], dir)
}

// pushdTarget reads the directory a `pushd` moves to: "" for a stack
// rotation (no argument, +N, -N), which only the shell can follow.
func pushdTarget(args []string, dir string) string {
	if len(args) == 0 || strings.HasPrefix(args[0], "+") || strings.HasPrefix(args[0], "-") {
		return ""
	}
	return resolve(args[0], dir)
}

// segments splits a command on the separators that start a new command, so a
// grep after `&&` is seen. A pipe does not start one: `cmd | grep x` is a
// filter over another command's output, which csl cannot replace and which
// the user's conventions explicitly allow. The first stage after a pipe is
// kept beside its command, so `find ... | xargs grep x` can be read whole;
// later stages are dropped.
//
// This deliberately does not reuse guard.splitSegments. That one preserves
// byte offsets and treats `|` exactly like `&&`, which is right for finding
// a denied command anywhere in a pipeline and wrong here: telling a pipe
// filter apart from a standalone search is the entire precision requirement
// of this hint. Quote awareness matters for the same reason: a `|` inside a
// grep pattern must not read as a pipe.
func segments(cmd string) []segment {
	var out []segment
	var cur strings.Builder
	var seg segment
	stage := 0 // the pipe stage cur holds: 0 is the command itself
	endStage := func() {
		switch stage {
		case 0:
			seg.cmd = cur.String()
		case 1:
			seg.pipe = cur.String()
		}
		cur.Reset()
		stage++
	}
	endCommand := func() {
		endStage()
		out = append(out, seg)
		seg, stage = segment{}, 0
	}
	var quote rune
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
		case c == '|' && i+1 < len(runes) && runes[i+1] == '|':
			i++
			endCommand()
		case c == '|':
			endStage()
		case c == '&' && i+1 < len(runes) && runes[i+1] == '&':
			i++
			endCommand()
		case c == ';' || c == '\n':
			endCommand()
		default:
			cur.WriteRune(c)
		}
	}
	endCommand()
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

// dropRedirects removes shell redirections from a command's words, with
// the target of an operator written apart from it (`> out`), so neither
// reads as a pattern or a path.
func dropRedirects(words []string) []string {
	out := make([]string, 0, len(words))
	for i := 0; i < len(words); i++ {
		switch op := redirectOp.FindString(words[i]); op {
		case "":
			out = append(out, words[i])
		case words[i]:
			i++ // the target is the next word
		}
	}
	return out
}

// sweepArgs is a search command's arguments sorted by role.
type sweepArgs struct {
	recursive bool
	dialect   dialect
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
	patterns, paths, ok := splitOperands(tool, a)
	if !ok {
		return nil
	}
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
	sw := &sweep{tool: tool, walks: walks, include: a.include, targets: resolveAll(paths, cwd)}
	if len(sw.targets) == 0 {
		return nil
	}
	if re := toRE2(patterns, a.dialect); tool == "fd" {
		sw.name = re
	} else {
		sw.pattern = re
	}
	return sw
}

// takePipe folds an `xargs grep` stage that reads a file listing into the
// sweep, so the query searches the listed files' content.
func (s *sweep) takePipe(stage string) {
	if s.tool == "find" || s.tool == "fd" {
		s.pattern = pipedPattern(stage)
	}
}

// pipedPattern reads a pipe stage like `xargs grep -n foo`, which searches
// the files the stage before it lists, and returns its pattern as RE2; ""
// when the stage is anything else.
func pipedPattern(stage string) string {
	words := splitFields(stage)
	if len(words) == 0 || filepath.Base(words[0]) != "xargs" {
		return ""
	}
	i := 1
	for i < len(words) && strings.HasPrefix(words[i], "-") {
		if xargsValueFlags[words[i]] {
			i++
		}
		i++
	}
	if i >= len(words) {
		return ""
	}
	tool := filepath.Base(words[i])
	if tool != "grep" && tool != "rg" && tool != "ag" {
		return ""
	}
	a := parseArgs(tool, words[i+1:])
	patterns, _, ok := splitOperands(tool, a)
	if !ok {
		return ""
	}
	return toRE2(patterns, a.dialect)
}

// resolveAll resolves each path against cwd, leaving out the ones only the
// shell could resolve.
func resolveAll(paths []string, cwd string) []string {
	var out []string
	for _, p := range paths {
		if r := resolve(p, cwd); r != "" {
			out = append(out, r)
		}
	}
	return out
}

// splitOperands separates the patterns from the paths. grep-likes and fd
// take the pattern first, then paths; find takes paths only. Patterns given
// with -e leave every operand a path. ok is false when nothing searched the
// filesystem.
func splitOperands(tool string, a sweepArgs) (patterns, paths []string, ok bool) {
	paths = a.operands
	switch {
	case len(a.patterns) > 0:
		return a.patterns, paths, true
	case tool == "find":
		return nil, paths, len(paths) > 0 // macOS find needs a path
	case len(paths) == 0:
		return nil, nil, tool == "fd" // a bare fd lists its directory; grep reads stdin
	}
	return []string{trimQuotes(paths[0])}, paths[1:], true
}

// parseArgs sorts a search tool's arguments into flags that matter, the
// patterns given with -e, and the remaining operands. Redirections are
// neither, and drop out first.
func parseArgs(tool string, args []string) sweepArgs {
	args = dropRedirects(args)
	if tool == "find" {
		return parseFindArgs(args)
	}
	// fd and git grep walk trees by definition.
	a := sweepArgs{recursive: tool == "fd" || tool == "git grep", dialect: defaultDialect(tool)}
	for i := 0; i < len(args); i++ {
		word := args[i]
		if !strings.HasPrefix(word, "-") || word == "-" {
			a.operands = append(a.operands, word)
			continue
		}
		flag, value, attached := a.readFlag(tool, word)
		if flag == "" {
			continue
		}
		if !attached {
			if i+1 >= len(args) {
				continue
			}
			i++
			value = args[i]
		}
		a.flagValue(tool, flag, trimQuotes(value))
	}
	return a
}

// readFlag reads one flag word and records the switches it sets. When the
// word holds a flag that takes a value, it returns that flag and the value
// written into the word (--glob=*.go, -C3) with attached true, or attached
// false when the value is the next word. A run of short flags such as -rnF
// is read letter by letter, and a letter that takes a value ends it.
func (a *sweepArgs) readFlag(tool, word string) (flag, value string, attached bool) {
	if strings.HasPrefix(word, "--") {
		name, v, hasValue := strings.Cut(word, "=")
		if valueFlags[tool][name] {
			return name, v, hasValue
		}
		a.setSwitch(tool, name)
		return "", "", false
	}
	letters := word[1:]
	for i, l := range letters {
		short := "-" + string(l)
		if valueFlags[tool][short] {
			rest := letters[i+1:]
			return short, rest, rest != ""
		}
		a.setSwitch(tool, short)
	}
	return "", "", false
}

// setSwitch records a flag that takes no value, when it recurses or picks
// the regex dialect.
func (a *sweepArgs) setSwitch(tool, flag string) {
	if d, ok := dialectFlags[tool][flag]; ok {
		a.dialect = d
	}
	switch flag {
	case "-r", "-R", "--recursive", "--dereference-recursive":
		a.recursive = true
	}
}

// flagValue records the value of a flag that matters for the query: a
// pattern given with -e, or a file filter given with --include, rg -g or
// fd -e.
func (a *sweepArgs) flagValue(tool, flag, value string) {
	switch {
	case tool == "fd" && (flag == "-e" || flag == "--extension"):
		a.include = "*." + value
	case flag == "-e" || flag == "--regexp":
		a.patterns = append(a.patterns, value)
	case flag == "--include":
		a.include = value
		a.recursive = true
	case tool == "rg" && (flag == "-g" || flag == "--glob" || flag == "--iglob"):
		a.include = value
	}
}

// parseFindArgs reads find's arguments. The paths come first, before the
// first word of the expression, so a value like the f in `-type f` is not
// read as a path. -name or -iname carries the file glob.
func parseFindArgs(args []string) sweepArgs {
	a := sweepArgs{recursive: true}
	inExpr := false
	for i := 0; i < len(args); i++ {
		word := args[i]
		switch {
		case word == "-name" || word == "-iname":
			if i+1 < len(args) {
				i++
				a.include = trimQuotes(args[i])
			}
			inExpr = true
		case len(a.operands) == 0 && findOptions[word]:
			// -L and friends, before the paths.
		case strings.HasPrefix(word, "-") || word == "(" || word == `\(` || word == "!":
			inExpr = true
		case !inExpr:
			a.operands = append(a.operands, word)
		}
	}
	return a
}

// toRE2 translates each pattern from the dialect the tool read it in to
// the RE2 zoekt takes, joined as one alternation.
func toRE2(patterns []string, d dialect) string {
	out := make([]string, len(patterns))
	for i, p := range patterns {
		switch d {
		case dialectFixed:
			out[i] = regexp.QuoteMeta(p)
		case dialectBasic:
			out[i] = basicToRE2(p)
		default:
			out[i] = p
		}
	}
	return strings.Join(out, "|")
}

// breSwapped are the characters a BRE and RE2 read the other way round:
// literal when bare in a BRE, operators when backslashed.
const breSwapped = "(){}|+?"

// basicToRE2 rewrites a GNU basic regex as RE2: it swaps the backslash on
// the characters in breSwapped, and turns the word boundaries \< and \>,
// which RE2 lacks, into \b.
func basicToRE2(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '\\' && i+1 < len(p):
			i++
			switch n := p[i]; {
			case strings.IndexByte(breSwapped, n) >= 0:
				b.WriteByte(n)
			case n == '<' || n == '>':
				b.WriteString(`\b`)
			default:
				b.WriteByte('\\')
				b.WriteByte(n)
			}
		case strings.IndexByte(breSwapped, c) >= 0:
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
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
