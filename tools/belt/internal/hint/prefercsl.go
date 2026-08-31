package hint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

// PreferCSL advises using csl_search after a bash command did a multi-file
// search inside a repo csl already indexes. It fires after the fact: the grep
// has run and its output stands, so the cost of a false positive is one line
// of ignored advice rather than a stalled session (docs/adr/0008).
type PreferCSL struct {
	cfg config.Config
	// resolveRepo maps a swept path to its canonical host/owner/repo
	// identity via the working tree's origin remote, for exclude_repos
	// matching — the filesystem-derived shard name is not an identity (a
	// worktree under any parent dir would dodge a path-based match).
	// Injectable for tests.
	resolveRepo func(target string) string
}

func NewPreferCSL(cfg config.Config) *PreferCSL {
	return &PreferCSL{cfg: cfg, resolveRepo: repoIdentityForPath}
}

// repoIdentityForPath resolves a swept path (file, dir, or unexpanded glob)
// to the canonical identity of the repo it sits in; "" outside a repo.
func repoIdentityForPath(target string) string {
	dir := target
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		dir = filepath.Dir(dir)
	}
	return guard.CanonicalRepoAt(dir)
}

func (h *PreferCSL) ID() string    { return "prefer-csl" }
func (h *PreferCSL) Event() string { return EventBash }

// searchTools are the commands worth advising on. `ls` and `cat` are absent
// on purpose: they are not searches.
var searchTools = map[string]bool{"grep": true, "rg": true, "ag": true, "find": true, "fd": true}

func (h *PreferCSL) Check(in Input) *Advice {
	if in.Command == "" {
		return nil
	}
	repos := indexedRepos()
	if len(repos) == 0 {
		return nil
	}
	for _, seg := range segments(in.Command) {
		sw := parseSweep(seg, in.Cwd)
		if sw == nil {
			continue
		}
		repo, ok := isIndexed(sw.target, repos)
		if !ok {
			continue
		}
		if h.cfg.HintRepoExcluded(h.ID(), h.resolveRepo(sw.target)) {
			continue
		}
		return &Advice{Hint: h.ID(), Text: sw.advice(repo)}
	}
	return nil
}

// sweep is a parsed multi-file search: which tool, what pattern, where, and
// any file-type filter worth carrying into the zoekt query.
type sweep struct {
	tool    string
	pattern string
	target  string
	include string // from --include=GLOB, translated to a zoekt f: filter
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
// of this hint. Quote awareness matters for the same reason — a `|` inside a
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

// parseSweep returns a sweep when seg is a multi-file search, or nil when it
// is anything else — including a single-file grep on a known path, which is a
// targeted read and exactly what csl is not for.
func parseSweep(seg, cwd string) *sweep {
	fields := splitFields(seg)
	if len(fields) == 0 {
		return nil
	}
	tool := filepath.Base(fields[0])
	if !searchTools[tool] {
		return nil
	}

	sw := &sweep{tool: tool}
	recursive := tool == "find" || tool == "fd" // both walk trees by definition
	var operands []string
	for _, f := range fields[1:] {
		switch {
		case strings.HasPrefix(f, "--include="):
			sw.include = trimQuotes(strings.TrimPrefix(f, "--include="))
			recursive = true
		case f == "-r" || f == "-R":
			recursive = true
		case strings.HasPrefix(f, "-") && len(f) > 1 && !strings.HasPrefix(f, "--"):
			// Bundled short flags: -rn, -rln, -ril and friends.
			if strings.ContainsAny(f[1:], "rR") {
				recursive = true
			}
		case strings.HasPrefix(f, "--"):
			// Other long flags carry no signal for this decision.
		default:
			operands = append(operands, f)
		}
	}
	if len(operands) == 0 {
		return nil // reading stdin, not the filesystem
	}

	// grep-likes take the pattern first, then paths. find takes paths first.
	var paths []string
	if tool == "find" || tool == "fd" {
		paths = operands
	} else {
		sw.pattern = trimQuotes(operands[0])
		paths = operands[1:]
	}
	if len(paths) == 0 {
		// A recursive grep with no path argument sweeps the working directory.
		if !recursive {
			return nil
		}
		paths = []string{cwd}
	}
	if !recursive && !multiFile(paths) {
		return nil // single-file targeted read
	}
	sw.target = resolve(paths[0], cwd)
	if sw.target == "" {
		return nil
	}
	return sw
}

// multiFile reports whether the path operands span more than one file, either
// by count or by containing a glob the shell may not have expanded.
func multiFile(paths []string) bool {
	if len(paths) > 1 {
		return true
	}
	return strings.ContainsAny(paths[0], "*?[")
}

func resolve(p, cwd string) string {
	p = trimQuotes(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~") {
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

// advice renders the replacement call. Handing back a usable query is the
// whole point: a bare "use csl" costs a turn while the model guesses at zoekt
// syntax, which is not the same language as grep.
func (s *sweep) advice(repo string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s is indexed by csl, and a recursive %s re-walks the filesystem for results the index already has.", repo, s.tool)
	if s.pattern == "" {
		fmt.Fprintf(&b, " Prefer csl_ls (repo=%q) for listing files, or csl_search for content.", repo)
		return b.String()
	}
	query := zoektQuery(s.pattern)
	if f := zoektFileFilter(s.include); f != "" {
		query += " " + f
	}
	fmt.Fprintf(&b, " Prefer: csl_search repo=%q query=%q.", repo, query)
	b.WriteString(" Note zoekt is not grep: OR is `a|b` with no spaces, and two space-separated terms mean AND within one file.")
	return b.String()
}

// zoektQuery translates the grep dialect belt sees most: escaped alternation
// becomes plain alternation. Everything else passes through, since zoekt
// takes regex.
func zoektQuery(pattern string) string {
	return strings.ReplaceAll(pattern, `\|`, `|`)
}

// zoektFileFilter turns --include=*.go into the f: regex zoekt wants. Globs
// and regexes differ enough that only a plain extension is translated. Brace
// alternation, character classes, and path globs are dropped rather than
// mistranslated: an unfiltered query returns too much, but a malformed one
// returns nothing and looks like the code is absent.
func zoektFileFilter(include string) string {
	ext, ok := strings.CutPrefix(include, "*.")
	if !ok || ext == "" {
		return ""
	}
	for _, r := range ext {
		alnum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !alnum {
			return ""
		}
	}
	return `f:\.` + ext + `$`
}
