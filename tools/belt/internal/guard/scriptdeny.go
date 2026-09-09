package guard

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// ScriptDenyListID identifies the deep deny inspection guard: it applies the
// Bash deny list inside scripts, not just to the top-level command.
const ScriptDenyListID = "script-deny-list"

// maxScriptSize caps how much of a script file gets scanned.
const maxScriptSize = 1 << 20

// shellInterpreters and inlineEInterpreters are command names whose file
// argument is a script worth scanning, split by which flag marks an inline
// script: -c for shells and python, -e/-E/--eval for the rest. Python names
// are matched by prefix (python3.12) in isInterpreter.
var (
	shellInterpreters = map[string]bool{
		"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "fish": true,
	}
	inlineEInterpreters = map[string]bool{
		"perl": true, "ruby": true, "node": true, "osascript": true,
	}
	pythonName = regexp.MustCompile(`^python[0-9.]*$`)
)

// inlineC and inlineE are the flag sets marking an inline script per
// interpreter family.
var (
	inlineC = map[string]bool{"-c": true}
	inlineE = map[string]bool{"-e": true, "-E": true, "--eval": true}
)

// isInterpreter classifies a segment head, returning the flag set that marks
// an inline script for it.
func isInterpreter(head string) (inlineFlags map[string]bool, ok bool) {
	switch {
	case shellInterpreters[head] || pythonName.MatchString(head):
		return inlineC, true
	case inlineEInterpreters[head]:
		return inlineE, true
	}
	return nil, false
}

// scriptExts mark directly-executed files (./x.sh) as scannable scripts.
var scriptExts = map[string]bool{
	".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".py": true,
	".rb": true, ".pl": true, ".js": true, ".mjs": true, ".cjs": true, ".ts": true,
}

// wrapperCommands are stripped from the front of a segment before the head
// is classified; wrappersWithArg additionally consume the next token.
var wrapperCommands = map[string]bool{
	"sudo": true, "command": true, "exec": true, "nohup": true, "time": true, "env": true,
}

var wrappersWithArg = map[string]bool{"timeout": true}

// fileProducers and fetchProducers classify the segment piped into a bare
// interpreter: a file producer's file arguments get scanned, a fetcher is
// denied outright — its content cannot be inspected before it runs.
var (
	fileProducers  = map[string]bool{"cat": true, "head": true, "tail": true}
	fetchProducers = map[string]bool{"curl": true, "wget": true}
)

// ScriptDenyList denies bash tool calls that route deny-listed commands
// around the permission system's view of the top-level command line: an
// executed or sourced script file, a `-c`/`-e` string, a heredoc, a file
// written and run inside the same command, or an indirection head (eval,
// xargs, find -exec) whose real command a prefix matcher never sees. The
// permission system only judges the literal command — `bash cleanup.sh`
// looks harmless even when the script runs `rm -rf`. This guard reads what
// is about to run and applies the same deny list to it. Patterns come from
// the Claude settings deny lists (permissions.deny `Bash(...)` entries) plus
// `extra_patterns` in the belt config; `mode: soft` downgrades denials to
// warn events.
type ScriptDenyList struct {
	cfg config.Config
	// emit sends the soft-mode warning to the events service. Injectable for
	// tests.
	emit func(source, level, title, message string, tags map[string]string)
}

// NewScriptDenyList builds the guard with the real events backend.
func NewScriptDenyList(cfg config.Config) *ScriptDenyList {
	return &ScriptDenyList{cfg: cfg, emit: notify.EmitEventSync}
}

func (g *ScriptDenyList) ID() string    { return ScriptDenyListID }
func (g *ScriptDenyList) Event() string { return EventBash }

// Check scans every script the command would execute and denies on the first
// deny-list match. In soft mode the denial becomes a warn event and the
// command proceeds.
func (g *ScriptDenyList) Check(in Input) *Denial {
	d := g.check(in)
	if d == nil {
		return nil
	}
	if g.cfg.Guards[ScriptDenyListID].Soft() {
		g.emit("belt", "warn", "script-deny-list (soft)",
			d.Reason+" Command proceeding (soft mode).",
			map[string]string{"guard": ScriptDenyListID})
		return nil
	}
	return d
}

// scan carries one check invocation's shared state across segments.
type scan struct {
	in       Input
	segs     []segment
	written  map[string]bool
	patterns []deny
}

func (g *ScriptDenyList) check(in Input) *Denial {
	patterns := compilePatterns(g.patterns())
	if len(patterns) == 0 || in.Command == "" {
		return nil
	}
	segs := splitSegments(in.Command)
	s := scan{in: in, segs: segs, written: redirectTargets(segs, in.Cwd), patterns: patterns}
	for i := range segs {
		if d := g.checkSegment(s, i); d != nil {
			return d
		}
	}
	return nil
}

// checkSegment classifies one segment's head and scans whatever script text
// or file it would run.
func (g *ScriptDenyList) checkSegment(s scan, i int) *Denial {
	seg := s.segs[i]
	tokens := stripWrappers(strings.Fields(seg.text))
	if len(tokens) == 0 {
		return nil
	}
	head := filepath.Base(tokens[0])
	if flags, ok := isInterpreter(head); ok {
		return g.checkInterpreter(s, i, tokens[1:], flags)
	}
	switch {
	case head == "uv" && len(tokens) > 1 && tokens[1] == "run":
		_, file := parseInterpreterArgs(tokens[2:], inlineC)
		return g.scanTarget(s, file)
	case head == "eval":
		// The eval argument may span separator splits inside quotes; scan
		// from this segment to the end of the command, like inline scripts.
		return scanText(s.in.Command[seg.off:], "the eval argument", s.patterns)
	case head == "xargs":
		return scanText(strings.Join(tokens[1:], " "), "the xargs command", s.patterns)
	case head == "find":
		return scanFindExec(tokens, s.patterns)
	case head == "source" || tokens[0] == ".":
		if len(tokens) > 1 {
			return g.scanTarget(s, tokens[1])
		}
	case strings.ContainsRune(tokens[0], '/'):
		resolved := resolvePath(tokens[0], s.in.Cwd)
		if s.written[resolved] || scannableScript(resolved) {
			return g.scanTarget(s, tokens[0])
		}
	}
	return nil
}

// checkInterpreter handles a segment whose head is an interpreter: an inline
// script (-c/-e string or heredoc), a script file argument, a `< file` stdin
// redirect, or a pipe feeding it from a producer segment.
func (g *ScriptDenyList) checkInterpreter(
	s scan, i int, args []string, inlineFlags map[string]bool,
) *Denial {
	seg := s.segs[i]
	inline, file := parseInterpreterArgs(args, inlineFlags)
	heredoc := strings.Contains(seg.text, "<<")
	if inline || heredoc {
		// Inline script: scan from this segment to the end of the command —
		// the script text follows the interpreter and may cross separator
		// splits inside quotes, but text before the segment belongs to
		// other commands.
		if d := scanText(s.in.Command[seg.off:], "the inline script", s.patterns); d != nil {
			return d
		}
	}
	if file != "" {
		return g.scanTarget(s, file)
	}
	if !inline && !heredoc {
		return g.checkProducer(s, i)
	}
	return nil
}

// checkProducer looks at the segment piped into a bare interpreter: file
// producers (cat x.sh | bash) get their file arguments scanned, fetchers
// (curl ... | bash) are denied because nothing can read what would run.
func (g *ScriptDenyList) checkProducer(s scan, i int) *Denial {
	if i == 0 || !pipeBefore(s.in.Command, s.segs[i].off) {
		return nil
	}
	prev := stripWrappers(strings.Fields(s.segs[i-1].text))
	if len(prev) == 0 {
		return nil
	}
	switch head := filepath.Base(prev[0]); {
	case fileProducers[head]:
		for _, arg := range prev[1:] {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			if d := g.scanTarget(s, arg); d != nil {
				return d
			}
		}
	case fetchProducers[head]:
		return Reasonf(
			ScriptDenyListID,
			"pipes a %s download straight into an interpreter, so nothing can inspect what would run. "+
				"Download to a file first, then run the file, so the deny list (and the user) can read it before it executes.",
			head,
		)
	}
	return nil
}

// pipeBefore reports whether the segment starting at off follows a single |
// (a pipe), not a || or another separator.
func pipeBefore(command string, off int) bool {
	return off > 0 && command[off-1] == '|' && (off < 2 || command[off-2] != '|')
}

// patterns merges the Claude settings deny prefixes with the guard's
// configured extra patterns.
func (g *ScriptDenyList) patterns() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range g.cfg.ClaudeDeny {
		add(p)
	}
	for _, p := range g.cfg.Guards[ScriptDenyListID].ExtraPatterns {
		add(p)
	}
	return out
}

// scanTarget scans one script path the command would run: the file's current
// content, and — when the same command also writes that path — the raw
// command text, which is where the future content lives at check time.
func (g *ScriptDenyList) scanTarget(s scan, path string) *Denial {
	if path == "" {
		return nil
	}
	resolved := resolvePath(path, s.in.Cwd)
	if g.cfg.Guards[ScriptDenyListID].ExcludesPath(resolved) {
		return nil
	}
	if s.written[resolved] {
		where := fmt.Sprintf("the command (which writes and then runs %s)", path)
		if d := scanText(s.in.Command, where, s.patterns); d != nil {
			return d
		}
	}
	return g.scanFile(resolved, s.patterns)
}

// scanFile reads the script (capped at maxScriptSize) and scans it.
// Unreadable files are allowed — the shell will fail on them anyway, and
// belt must not block on its own limitations; the write-then-run pass in
// scanTarget covers the file that does not exist yet.
func (g *ScriptDenyList) scanFile(resolved string, patterns []deny) *Denial {
	f, err := os.Open(resolved)
	if err != nil {
		return nil
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxScriptSize))
	if err != nil {
		return nil
	}
	return scanText(string(data), fmt.Sprintf("script %s", resolved), patterns)
}

// resolvePath expands ~ and joins relative paths against cwd, mirroring how
// the shell will resolve the path.
func resolvePath(path, cwd string) string {
	resolved := config.ExpandHome(path)
	if !filepath.IsAbs(resolved) && cwd != "" {
		resolved = filepath.Join(cwd, resolved)
	}
	return resolved
}

// redirectTargets collects the files the command writes via >/>> redirects
// or tee, resolved like executed scripts, so scanTarget can spot a script
// written and run inside one command — at check time that file does not
// exist yet and scanFile alone would fail open.
func redirectTargets(segs []segment, cwd string) map[string]bool {
	targets := map[string]bool{}
	add := func(path string) {
		if path != "" && !strings.HasPrefix(path, "&") {
			targets[resolvePath(path, cwd)] = true
		}
	}
	for _, seg := range segs {
		tokens := strings.Fields(seg.text)
		for i := 0; i < len(tokens); i++ {
			if filepath.Base(tokens[i]) == "tee" {
				for _, arg := range tokens[i+1:] {
					if !strings.HasPrefix(arg, "-") {
						add(arg)
					}
				}
				break
			}
			j := strings.IndexByte(tokens[i], '>')
			if j < 0 {
				continue
			}
			rest := strings.TrimLeft(tokens[i][j:], ">")
			if rest == "" {
				if i+1 < len(tokens) {
					add(tokens[i+1])
					i++
				}
				continue
			}
			add(rest)
		}
	}
	return targets
}

// scannableScript reports whether a directly-executed path is script-like: a
// known script extension, or an existing file that starts with a shebang, so
// ./deploy with #!/bin/bash gets scanned without an allowlisted extension.
func scannableScript(resolved string) bool {
	if scriptExts[filepath.Ext(resolved)] {
		return true
	}
	f, err := os.Open(resolved)
	if err != nil {
		return false
	}
	defer f.Close()
	var magic [2]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return false
	}
	return magic[0] == '#' && magic[1] == '!'
}

// scanFindExec scans the command after find's -exec/-execdir/-ok/-okdir —
// the one part of a find invocation that runs something.
func scanFindExec(tokens []string, patterns []deny) *Denial {
	for i, tok := range tokens {
		switch tok {
		case "-exec", "-execdir", "-ok", "-okdir":
			return scanText(strings.Join(tokens[i+1:], " "), "the find "+tok+" command", patterns)
		}
	}
	return nil
}

// deny pairs a deny-list pattern with its compiled matcher.
type deny struct {
	pattern string
	re      *regexp.Regexp
}

// compilePatterns builds a matcher per pattern. A `re:` prefix compiles the
// rest as a case-insensitive regex — the escape hatch for shapes the literal
// form cannot express (flag reordering, argument wildcards); anchor it
// yourself when word boundaries matter. Every other pattern matches its
// tokens in order, separated by whitespace, not embedded in a longer word:
// `kubectl delete` matches `  kubectl   delete pod x` but not `kubectl
// deleted`.
func compilePatterns(patterns []string) []deny {
	var out []deny
	for _, p := range patterns {
		if raw, ok := strings.CutPrefix(p, "re:"); ok {
			if re, err := regexp.Compile("(?i)" + raw); err == nil {
				out = append(out, deny{pattern: p, re: re})
			}
			continue
		}
		toks := strings.Fields(p)
		if len(toks) == 0 {
			continue
		}
		quoted := make([]string, len(toks))
		for i, t := range toks {
			quoted[i] = regexp.QuoteMeta(t)
		}
		re, err := regexp.Compile(`(?i)(?:^|[^\w-])` + strings.Join(quoted, `\s+`) + `(?:[^\w-]|$)`)
		if err != nil {
			continue
		}
		out = append(out, deny{pattern: p, re: re})
	}
	return out
}

// lineNoise collapses the separators that hide a command from the
// word-boundary matcher — quotes, commas, brackets, backticks — so
// subprocess.run(["rm", "-rf", ...]) and kubectl "delete" match the same
// patterns as their plain forms.
var lineNoise = strings.NewReplacer(
	`"`, " ", `'`, " ", ",", " ", "[", " ", "]", " ", "`", " ",
)

// logicalLine is one script statement after backslash-continued lines are
// joined, tagged with the physical line it starts on.
type logicalLine struct {
	text   string
	number int
}

// logicalLines splits text into statements, joining lines that end in a
// backslash so a pattern split across a continuation still matches.
func logicalLines(text string) []logicalLine {
	raw := strings.Split(text, "\n")
	out := make([]logicalLine, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		start := i + 1
		line := raw[i]
		for strings.HasSuffix(line, `\`) && i+1 < len(raw) {
			line = strings.TrimSuffix(line, `\`) + " " + raw[i+1]
			i++
		}
		out = append(out, logicalLine{text: line, number: start})
	}
	return out
}

// scanText checks every non-comment logical line against every pattern —
// first as written, then with quoting noise collapsed — and denies on the
// first hit.
func scanText(text, where string, patterns []deny) *Denial {
	for _, line := range logicalLines(text) {
		trimmed := strings.TrimSpace(line.text)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		normalized := lineNoise.Replace(line.text)
		for _, d := range patterns {
			if d.re.MatchString(line.text) || d.re.MatchString(normalized) {
				return Reasonf(
					ScriptDenyListID,
					"%s runs %q (line %d) — that command is on the Bash deny list and scripts don't get to bypass it. "+
						"Run the blocked operation as a direct command so the permission system can evaluate it, or ask the user first.",
					where,
					d.pattern,
					line.number,
				)
			}
		}
	}
	return nil
}

// parseInterpreterArgs walks the tokens after an interpreter and reports
// whether it runs an inline script (a flag from inlineFlags), or which file
// it runs — a positional argument or a `< file` stdin redirect. An inline
// flag counts only before the file argument — in `python x.py -c foo` the -c
// belongs to the script, and x.py must still be scanned.
func parseInterpreterArgs(tokens []string, inlineFlags map[string]bool) (inline bool, file string) {
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch {
		case inlineFlags[tok]:
			return true, ""
		case tok == "--":
			if i+1 < len(tokens) {
				return false, tokens[i+1]
			}
			return false, ""
		case tok == "-m":
			return false, "" // python -m runs a module, not a file
		case tok == "<<" || tok == "<<-":
			i++ // heredoc: skip the delimiter token, the body is scanned via the segment text
		case strings.HasPrefix(tok, "<<"):
			continue // <<EOF / <<<word: the body is scanned via the segment text
		case tok == "<":
			if i+1 < len(tokens) {
				return false, tokens[i+1]
			}
			return false, ""
		case strings.HasPrefix(tok, "<"):
			return false, tok[1:] // attached stdin redirect: <x.sh
		case strings.HasPrefix(tok, "-"):
			continue
		default:
			return false, tok
		}
	}
	return false, ""
}

// stripWrappers drops leading env assignments and wrapper commands (sudo,
// env, exec, ...) so the real head of the segment gets classified.
func stripWrappers(tokens []string) []string {
	for len(tokens) > 0 {
		head := filepath.Base(tokens[0])
		switch {
		case strings.Contains(tokens[0], "="):
			tokens = tokens[1:]
		case wrapperCommands[head]:
			tokens = tokens[1:]
		case wrappersWithArg[head] && len(tokens) > 1:
			tokens = tokens[2:]
		default:
			return tokens
		}
	}
	return tokens
}
