package hint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/cslindex"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

// preferCSLMarkerPrefix starts the seen-file id that keeps the hint to one
// word per checkout per agent per session.
const preferCSLMarkerPrefix = "prefer-csl:"

// PreferCSL advises the csl CLI command, and the csl_search call after it,
// when a bash command did a multi-file search inside a repo csl already
// indexes, or inside a linked worktree of one. It fires after the fact: the
// grep has run and its output stands, so the cost of a false positive is one
// line of ignored advice rather than a stalled session (docs/adr/0008). It
// speaks once per checkout per agent per session: a hint that repeats on
// every grep becomes wallpaper.
type PreferCSL struct {
	cfg config.Config
	// resolveRepo maps a swept path to its canonical host/owner/repo
	// identity via the working tree's origin remote, for exclude_repos
	// matching and for naming a linked worktree's repo. The
	// filesystem-derived shard name is not an identity: a worktree under any
	// parent dir would dodge a path-based match. Injectable for tests.
	resolveRepo func(target string) string
	// indexed lists the repos csl has a shard for. Injectable for tests.
	indexed func() map[string]bool
}

func NewPreferCSL(cfg config.Config) *PreferCSL {
	return &PreferCSL{cfg: cfg, resolveRepo: repoIdentityForPath, indexed: cslindex.Repos}
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

// Check advises on the first segment of the command that sweeps a checkout
// csl can search and this agent has not had the hint for. Each segment's
// paths resolve against the directory it runs in, so a search after
// `cd <repo> &&` names that repo and not the session's cwd. Without a
// session id nothing is recorded, and the hint speaks on every match, the
// seen store's degrade-to-not-seen rule.
func (h *PreferCSL) Check(in Input) *Advice {
	if in.Command == "" {
		return nil
	}
	repos := h.indexed()
	if len(repos) == 0 {
		return nil
	}
	seenPath := seen.path(in.SessionID)
	done := map[string]bool{}
	if seenPath != "" {
		done = seen.load(seenPath)
	}
	dirs := workDirs{cur: in.Cwd}
	for _, seg := range segments(in.Command) {
		cmd, dir := dirs.step(seg.cmd)
		sw := parseSweep(cmd, dir)
		if sw == nil {
			continue
		}
		sw.takePipe(seg.pipe)
		t, ok := h.firstSearchable(sw.targets, repos, in.AgentID, done)
		if !ok {
			continue
		}
		if marker := t.marker(in.AgentID); seenPath != "" && !strings.ContainsAny(marker, "\r\n") {
			seen.record(seenPath, []string{marker})
		}
		return &Advice{Hint: h.ID(), Text: sw.advice(t)}
	}
	return nil
}

// cslTarget is the checkout an advised csl call searches: a main checkout by
// the org/repo name csl indexes it under, or a linked worktree by its root,
// which csl searches from an index of its own.
type cslTarget struct {
	repo     string // csl's org/repo name; for a worktree, its main checkout's
	worktree string // the linked worktree's root; "" for a main checkout
}

// marker is the seen-file id for this checkout and agent, built the way
// read-size builds its own (readSizeMarker): the agent id keeps one
// subagent's nudge from spending the main thread's, the repo name is
// lowercased because csl matches repo names regardless of case, and the id
// is trimmed because the seen store trims each line on load. A worktree is
// keyed by its root, so each worktree gets its own word.
func (t cslTarget) marker(agentID string) string {
	key := strings.ToLower(t.repo)
	if t.worktree != "" {
		key = "worktree:" + t.worktree
	}
	return strings.TrimSpace(preferCSLMarkerPrefix + agentID + ":" + key)
}

// firstSearchable names the first target that sits in a checkout csl can
// search, the config does not exclude, and done holds no marker for.
func (h *PreferCSL) firstSearchable(
	targets []string,
	repos map[string]bool,
	agentID string,
	done map[string]bool,
) (cslTarget, bool) {
	for _, target := range targets {
		t, ok := h.locate(target, repos, agentID, done)
		if ok {
			return t, true
		}
	}
	return cslTarget{}, false
}

// locate resolves one target. A linked worktree is checked first, by files
// alone, and its marker before its origin remote, so a worktree already
// advised costs no git call. Its repo comes from that remote, since the
// directory names of ~/.worktrees/<repo>/<slug> name no shard.
func (h *PreferCSL) locate(
	target string,
	repos map[string]bool,
	agentID string,
	done map[string]bool,
) (cslTarget, bool) {
	if root, _, ok := cslindex.LinkedWorktree(target); ok {
		t := cslTarget{worktree: root}
		if done[t.marker(agentID)] {
			return cslTarget{}, false
		}
		identity := h.resolveRepo(target)
		_, name, ok := strings.Cut(identity, "/") // host/owner/repo to owner/repo
		if !ok || !repos[strings.ToLower(name)] || h.cfg.HintRepoExcluded(h.ID(), identity) {
			return cslTarget{}, false
		}
		t.repo = name
		return t, true
	}
	repo, ok := isIndexed(target, repos)
	t := cslTarget{repo: repo}
	if !ok || done[t.marker(agentID)] || h.cfg.HintRepoExcluded(h.ID(), h.resolveRepo(target)) {
		return cslTarget{}, false
	}
	return t, true
}

// advice renders the replacement, the CLI command first and the MCP call
// after it: the CLI runs in any session with csl on PATH and has no
// per-call result cap, and the MCP form serves a session that has the
// tools. Handing back a usable query is the whole point: a bare "use csl"
// costs a turn while the model guesses at zoekt syntax, which is not the
// same language as grep. A main checkout's repo is an anchored regex in both
// forms, since both read it as one and an unanchored name also matches its
// prefixes. A worktree is named by its root, so the call searches the
// worktree's own index and not its main checkout on the default branch.
func (s *sweep) advice(t cslTarget) string {
	var b strings.Builder
	what := "a recursive " + s.tool + " re-walks the filesystem"
	if !s.walks {
		what = "a " + s.tool + " over several files reads them from disk"
	}
	if t.worktree != "" {
		fmt.Fprintf(
			&b,
			"%s is a linked worktree of %s, which csl indexes, and %s where csl searches the worktree from an index of its own.",
			t.worktree,
			t.repo,
			what,
		)
	} else {
		fmt.Fprintf(&b, "%s is indexed by csl, and %s for results the index already has.", t.repo, what)
	}
	cliScope, mcpScope := t.scopes()
	query := s.query()
	if query == "" {
		// A file listing with no name to translate.
		mcp := "csl_ls " + mcpScope
		if t.worktree != "" {
			mcp = fmt.Sprintf("csl_search %s query=%q", mcpScope, "f:<regex>")
		}
		fmt.Fprintf(
			&b,
			" Prefer: csl search 'f:<regex>' %s to list files by name (f: takes a regex, not a glob). With the csl MCP tools: %s.",
			cliScope,
			mcp,
		)
		return b.String()
	}
	fmt.Fprintf(&b, " Prefer: csl search %s %s.", shellQuote(query), cliScope)
	b.WriteString(
		" Note zoekt is not grep: OR is `a|b` with no spaces, and two space-separated terms mean AND within one file.",
	)
	fmt.Fprintf(&b, " With the csl MCP tools: csl_search %s query=%q.", mcpScope, query)
	return b.String()
}

// scopes renders the flag and the MCP argument that point a csl call at the
// target: --repo with the anchored repo regex, or --worktree with the root.
func (t cslTarget) scopes() (cli, mcp string) {
	if t.worktree != "" {
		return "--worktree " + shellQuote(t.worktree), fmt.Sprintf("worktree=%q", t.worktree)
	}
	repoRE := "^" + regexp.QuoteMeta(t.repo) + "$"
	return "--repo " + shellQuote(repoRE), fmt.Sprintf("repo=%q", repoRE)
}

// query is the zoekt query for the sweep: the content pattern, fd's
// file-name pattern as an f: filter, and the translated file glob. "" when
// there is nothing to search for.
func (s *sweep) query() string {
	var parts []string
	if s.pattern != "" {
		parts = append(parts, cslindex.ZoektTerm(s.pattern))
	}
	if s.name != "" {
		parts = append(parts, "f:"+cslindex.ZoektTerm(s.name))
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
