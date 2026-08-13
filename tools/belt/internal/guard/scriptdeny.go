package guard

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// ScriptDenyListID identifies the deep deny inspection guard: it applies the
// Bash deny list inside scripts, not just to the top-level command.
const ScriptDenyListID = "script-deny-list"

// maxScriptSize caps how much of a script file gets scanned.
const maxScriptSize = 1 << 20

// interpreters are command names whose file argument is a script worth
// scanning (and whose -c string is an inline script).
var interpreters = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true,
	"python": true, "python2": true, "python3": true,
}

// scriptExts mark directly-executed files (./x.sh) as scannable scripts.
var scriptExts = map[string]bool{".sh": true, ".bash": true, ".zsh": true, ".py": true}

// wrapperCommands are stripped from the front of a segment before the head
// is classified; wrappersWithArg additionally consume the next token.
var wrapperCommands = map[string]bool{
	"sudo": true, "command": true, "exec": true, "nohup": true, "time": true, "env": true,
}
var wrappersWithArg = map[string]bool{"timeout": true}

// ScriptDenyList denies bash tool calls that route deny-listed commands
// through a script: an executed or sourced script file, a `-c` string, or a
// heredoc. The permission system only sees the top-level command line —
// `bash cleanup.sh` looks harmless even when the script runs `rm -rf`. This
// guard reads what is about to run and applies the same deny list to it.
// Patterns come from the Claude settings deny lists (permissions.deny
// `Bash(...)` entries) plus `extra_patterns` in the belt config.
type ScriptDenyList struct {
	cfg config.Config
}

// NewScriptDenyList builds the guard.
func NewScriptDenyList(cfg config.Config) *ScriptDenyList {
	return &ScriptDenyList{cfg: cfg}
}

func (g *ScriptDenyList) ID() string    { return ScriptDenyListID }
func (g *ScriptDenyList) Event() string { return EventBash }

// Check scans every script the command would execute and denies on the first
// deny-list match.
func (g *ScriptDenyList) Check(in Input) *Denial {
	patterns := compilePatterns(g.patterns())
	if len(patterns) == 0 || in.Command == "" {
		return nil
	}
	for _, seg := range splitSegments(in.Command) {
		tokens := stripWrappers(strings.Fields(seg.text))
		if len(tokens) == 0 {
			continue
		}
		head := filepath.Base(tokens[0])
		switch {
		case interpreters[head]:
			inline, file := parseInterpreterArgs(tokens[1:])
			if inline || strings.Contains(seg.text, "<<") {
				// Inline script: -c string, or a heredoc feeding the
				// interpreter. Scan from this segment to the end of the
				// command — the script text follows the interpreter and
				// may cross separator splits inside quotes, but text
				// before the segment belongs to other commands.
				if d := scanText(in.Command[seg.off:], "the inline script", patterns); d != nil {
					return d
				}
			}
			if file != "" {
				if d := g.scanFile(file, in.Cwd, patterns); d != nil {
					return d
				}
			}
		case head == "source" || tokens[0] == ".":
			if len(tokens) > 1 {
				if d := g.scanFile(tokens[1], in.Cwd, patterns); d != nil {
					return d
				}
			}
		case strings.ContainsRune(tokens[0], '/') && scriptExts[filepath.Ext(tokens[0])]:
			if d := g.scanFile(tokens[0], in.Cwd, patterns); d != nil {
				return d
			}
		}
	}
	return nil
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

// scanFile reads the script (resolved against cwd, capped at maxScriptSize)
// and scans it. Unreadable files are allowed — the shell will fail on them
// anyway, and belt must not block on its own limitations.
func (g *ScriptDenyList) scanFile(path, cwd string, patterns []deny) *Denial {
	resolved := expandHome(path)
	if !filepath.IsAbs(resolved) && cwd != "" {
		resolved = filepath.Join(cwd, resolved)
	}
	for _, excl := range g.cfg.Guards[ScriptDenyListID].ExcludePaths {
		if excl != "" && strings.Contains(resolved, excl) {
			return nil
		}
	}
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

// deny pairs a deny-list pattern with its compiled matcher.
type deny struct {
	pattern string
	re      *regexp.Regexp
}

// compilePatterns builds a word-boundary matcher per pattern: the pattern's
// tokens in order, separated by whitespace, not embedded in a longer word.
// `kubectl delete` matches `  kubectl   delete pod x` but not `kubectl deleted`.
func compilePatterns(patterns []string) []deny {
	var out []deny
	for _, p := range patterns {
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

// scanText checks every non-comment line against every pattern and denies on
// the first hit.
func scanText(text, where string, patterns []deny) *Denial {
	for lineNo, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, d := range patterns {
			if d.re.MatchString(line) {
				return Reasonf(ScriptDenyListID,
					"%s runs %q (line %d) — that command is on the Bash deny list and scripts don't get to bypass it. "+
						"Run the blocked operation as a direct command so the permission system can evaluate it, or ask the user first.",
					where, d.pattern, lineNo+1)
			}
		}
	}
	return nil
}

// parseInterpreterArgs walks the tokens after an interpreter and reports
// whether it runs an inline -c script, or which file it runs. A -c counts
// only before the file argument — in `python x.py -c foo` the -c belongs to
// the script, and x.py must still be scanned.
func parseInterpreterArgs(tokens []string) (inline bool, file string) {
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch {
		case tok == "-c":
			return true, ""
		case tok == "--":
			if i+1 < len(tokens) {
				return false, tokens[i+1]
			}
			return false, ""
		case tok == "-m":
			return false, "" // python -m runs a module, not a file
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
