package hint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

// PreferCSL advises the csl CLI command, and the csl_search call after it,
// when a bash command did a multi-file search inside a repo csl already
// indexes. It fires after the fact: the grep has run and its output stands,
// so the cost of a false positive is one line of ignored advice rather than
// a stalled session (docs/adr/0008).
type PreferCSL struct {
	cfg config.Config
	// resolveRepo maps a swept path to its canonical host/owner/repo
	// identity via the working tree's origin remote, for exclude_repos
	// matching. The filesystem-derived shard name is not an identity: a
	// worktree under any parent dir would dodge a path-based match.
	// Injectable for tests.
	resolveRepo func(target string) string
	// indexed lists the repos csl has a shard for. Injectable for tests.
	indexed func() map[string]bool
}

func NewPreferCSL(cfg config.Config) *PreferCSL {
	return &PreferCSL{cfg: cfg, resolveRepo: repoIdentityForPath, indexed: indexedRepos}
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

// Check advises on the first segment of the command that sweeps an indexed
// repo. Each segment's paths resolve against the directory it runs in, so a
// search after `cd <repo> &&` names that repo and not the session's cwd.
func (h *PreferCSL) Check(in Input) *Advice {
	if in.Command == "" {
		return nil
	}
	repos := h.indexed()
	if len(repos) == 0 {
		return nil
	}
	dirs := workDirs{cur: in.Cwd}
	for _, seg := range segments(in.Command) {
		cmd, dir := dirs.step(seg.cmd)
		sw := parseSweep(cmd, dir)
		if sw == nil {
			continue
		}
		sw.takePipe(seg.pipe)
		if repo, ok := h.firstIndexed(sw.targets, repos); ok {
			return &Advice{Hint: h.ID(), Text: sw.advice(repo)}
		}
	}
	return nil
}

// firstIndexed names the repo of the first target that sits in a repo csl
// indexes and the config does not exclude.
func (h *PreferCSL) firstIndexed(targets []string, repos map[string]bool) (string, bool) {
	for _, target := range targets {
		repo, ok := isIndexed(target, repos)
		if ok && !h.cfg.HintRepoExcluded(h.ID(), h.resolveRepo(target)) {
			return repo, true
		}
	}
	return "", false
}

// advice renders the replacement, the CLI command first and the MCP call
// after it: the CLI runs in any session with csl on PATH and has no
// per-call result cap, and the MCP form serves a session that has the
// tools. Handing back a usable query is the whole point: a bare "use csl"
// costs a turn while the model guesses at zoekt syntax, which is not the
// same language as grep. The repo is an anchored regex in both forms, since
// both read it as one and an unanchored name also matches its prefixes.
func (s *sweep) advice(repo string) string {
	var b strings.Builder
	what := "a recursive " + s.tool + " re-walks the filesystem"
	if !s.walks {
		what = "a " + s.tool + " over several files reads them from disk"
	}
	fmt.Fprintf(&b, "%s is indexed by csl, and %s for results the index already has.", repo, what)
	repoRE := "^" + regexp.QuoteMeta(repo) + "$"
	query := s.query()
	if query == "" {
		// A file listing with no name to translate.
		fmt.Fprintf(
			&b,
			" Prefer: csl search 'f:<regex>' --repo %s to list files by name (f: takes a regex, not a glob). With the csl MCP tools: csl_ls repo=%q.",
			shellQuote(repoRE),
			repoRE,
		)
		return b.String()
	}
	fmt.Fprintf(&b, " Prefer: csl search %s --repo %s.", shellQuote(query), shellQuote(repoRE))
	b.WriteString(
		" Note zoekt is not grep: OR is `a|b` with no spaces, and two space-separated terms mean AND within one file.",
	)
	fmt.Fprintf(&b, " With the csl MCP tools: csl_search repo=%q query=%q.", repoRE, query)
	return b.String()
}

// query is the zoekt query for the sweep: the content pattern, fd's
// file-name pattern as an f: filter, and the translated file glob. "" when
// there is nothing to search for.
func (s *sweep) query() string {
	var parts []string
	if s.pattern != "" {
		parts = append(parts, zoektTerm(s.pattern))
	}
	if s.name != "" {
		parts = append(parts, "f:"+zoektTerm(s.name))
	}
	if f := zoektFileFilter(s.include); f != "" {
		parts = append(parts, f)
	}
	return strings.Join(parts, " ")
}

// shellQuote single-quotes s for a POSIX shell, so the advised command can
// be pasted as it is whatever the pattern holds.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// zoektTerm renders an RE2 pattern as one zoekt term. Unquoted, a pattern
// with a space reads as an AND of several terms, a double quote opens a
// phrase, and a leading - negates; such a pattern is quoted as a phrase.
// zoekt unescapes one level of backslashes inside quotes, so they double.
func zoektTerm(re string) string {
	if !strings.ContainsAny(re, " \t\"") && !strings.HasPrefix(re, "-") {
		return re
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(re) + `"`
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
