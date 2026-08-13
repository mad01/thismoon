// Package scan reads local Claude Code session transcripts and distills each
// recent session into a compact digest. It exists so the /worklog-backfill
// skill can reason over a small JSON summary instead of pulling megabytes of
// raw transcript into context.
package scan

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// Session is the compact digest of one transcript file.
type Session struct {
	SessionID string    `json:"session_id"`
	Project   string    `json:"project"`
	Started   time.Time `json:"started"`
	Ended     time.Time `json:"ended"`
	Title     string    `json:"title,omitempty"`
	// Context is the ticket world this session belongs to, derived from cwd:
	// "personal" (github.com/mad01 → Linear MAD-NN, plus legacy mad01/issues
	// #NN), "internal" (work → Jira TEAM-NN), "mixed", or "unknown". The two
	// worlds must never cross-reference, so Tickets is filtered to match Context.
	Context      string   `json:"context"`
	Repos        []string `json:"repos,omitempty"`
	Branches     []string `json:"branches,omitempty"`
	Tickets      []string `json:"tickets,omitempty"`
	UserMessages int      `json:"user_messages"`
	FirstPrompts []string `json:"first_prompts,omitempty"`
	LastPrompts  []string `json:"last_prompts,omitempty"`
}

var (
	// Tracker keys shaped TEAM-NN: Jira (ABC-1234) and Linear (MAD-123) both
	// fit this. Prefix decides the world — see linearPrefixes.
	keyRe = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-\d+\b`)
	// GitHub issue/PR URLs: .../issues/52 or .../pull/52.
	issueURLRe = regexp.MustCompile(`github\.com/[\w.-]+/[\w.-]+/(?:issues|pull)/(\d+)`)
	// Bare references in prose: "issue #51", "PR #15".
	issueHashRe = regexp.MustCompile(`(?:^|\s)#(\d+)\b`)
)

// Config carries the ticket-firewall strings. Zero-value fields fall back to
// the built-in defaults below, so scanning works with no config file present.
// The machine's real values ship via the consuming repo's config overlay
// (~/.config/worklog/config.yaml).
type Config struct {
	LinearPrefixes      []string
	PersonalPathMarkers []string
	InternalPathMarkers []string
	CheckoutRoots       []string
	RepoPathMarkers     []string
}

var defaultConfig = Config{
	LinearPrefixes:      []string{"MAD"},
	PersonalPathMarkers: []string{"github.com/mad01/"},
	InternalPathMarkers: []string{"/workspace/"},
	CheckoutRoots:       []string{"/code/src/"},
	RepoPathMarkers:     []string{"/code/", "/workspace/"},
}

// WithDefaults returns c with every empty field filled from the built-in
// defaults. Scan applies it before scanning; `worklog config` applies it to
// print the settings actually in effect.
func (c Config) WithDefaults() Config {
	if len(c.LinearPrefixes) == 0 {
		c.LinearPrefixes = defaultConfig.LinearPrefixes
	}
	if len(c.PersonalPathMarkers) == 0 {
		c.PersonalPathMarkers = defaultConfig.PersonalPathMarkers
	}
	if len(c.InternalPathMarkers) == 0 {
		c.InternalPathMarkers = defaultConfig.InternalPathMarkers
	}
	if len(c.CheckoutRoots) == 0 {
		c.CheckoutRoots = defaultConfig.CheckoutRoots
	}
	if len(c.RepoPathMarkers) == 0 {
		c.RepoPathMarkers = defaultConfig.RepoPathMarkers
	}
	return c
}

// isLinearPrefix reports whether a TEAM-NN key prefix belongs to the personal
// (Linear) world. Any other key is treated as Jira (internal).
func (c Config) isLinearPrefix(prefix string) bool {
	return slices.Contains(c.LinearPrefixes, prefix)
}

func keyPrefix(key string) string {
	if i := strings.IndexByte(key, '-'); i > 0 {
		return key[:i]
	}
	return key
}

// extractKeys captures every TEAM-NN tracker key and routes it by prefix:
// Linear keys (personal) into linear, everything else (Jira, internal) into jira.
func extractKeys(cfg Config, txt string, linear, jira *set) {
	for _, k := range keyRe.FindAllString(txt, -1) {
		if cfg.isLinearPrefix(keyPrefix(k)) {
			linear.add(k)
		} else {
			jira.add(k)
		}
	}
}

func extractIssues(txt string, into *set) {
	for _, m := range issueURLRe.FindAllStringSubmatch(txt, -1) {
		into.add("#" + m[1])
	}
	for _, m := range issueHashRe.FindAllStringSubmatch(txt, -1) {
		into.add("#" + m[1])
	}
}

// ticketContext collapses the per-cwd flags into the session's world.
func ticketContext(personal, internal bool) string {
	switch {
	case personal && internal:
		return "mixed"
	case personal:
		return "personal"
	case internal:
		return "internal"
	default:
		return "unknown"
	}
}

// classifyCwd reports which ticket world a working directory belongs to. A path
// can match neither (e.g. a tmp dir), which leaves the session "unknown".
// Beyond the configured markers, GOPATH-style checkouts of any non-github.com
// host count as internal — that split is derived, never enumerated (same
// principle as belt's public/internal remote check).
func classifyCwd(cfg Config, cwd string) (personal, internal bool) {
	for _, m := range cfg.PersonalPathMarkers {
		if strings.Contains(cwd, m) {
			personal = true
			break
		}
	}
	for _, m := range cfg.InternalPathMarkers {
		if strings.Contains(cwd, m) {
			internal = true
			break
		}
	}
	return personal, internal || internalHostCheckout(cfg, cwd)
}

// internalHostCheckout reports whether cwd sits under a GOPATH-style checkout
// of a non-github.com git host: a host-shaped segment (contains a dot)
// directly under a checkout root that isn't github.com.
func internalHostCheckout(cfg Config, cwd string) bool {
	for _, root := range cfg.CheckoutRoots {
		_, rest, ok := strings.Cut(cwd, root)
		if !ok {
			continue
		}
		host, _, _ := strings.Cut(rest, "/")
		if strings.Contains(host, ".") && host != "github.com" {
			return true
		}
	}
	return false
}

// resolveTickets enforces the firewall: a personal task keys on Linear MAD-NN
// (plus legacy mad01/issues #NN) only, an internal task on JIRA keys only —
// never both. Mixed/unknown sessions surface everything for human review during
// backfill.
func resolveTickets(context string, linear, jira, issues *set) []string {
	switch context {
	case "personal":
		return sortedUnion(linear, issues)
	case "internal":
		return jira.sorted()
	default: // mixed | unknown
		return sortedUnion(linear, jira, issues)
	}
}

func sortedUnion(sets ...*set) []string {
	out := newSet()
	for _, s := range sets {
		for k := range s.m {
			out.add(k)
		}
	}
	return out.sorted()
}

// ProjectsDir is ~/.claude/projects, overridable via $CLAUDE_PROJECTS_DIR.
func ProjectsDir() string {
	if d := os.Getenv("CLAUDE_PROJECTS_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// Scan returns digests of every session whose last activity is within the
// window ending at now, newest first. Zero-value cfg fields fall back to the
// built-in defaults.
func Scan(root string, since time.Duration, now time.Time, cfg Config) ([]Session, error) {
	if root == "" {
		root = ProjectsDir()
	}
	cfg = cfg.WithDefaults()
	cutoff := now.Add(-since)
	projects, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Session
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		files, _ := filepath.Glob(filepath.Join(root, p.Name(), "*.jsonl"))
		for _, f := range files {
			s, ok, err := digestFile(cfg, f, p.Name())
			if err != nil {
				return nil, err
			}
			if ok && s.Ended.After(cutoff) {
				out = append(out, s)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ended.After(out[j].Ended) })
	return out, nil
}

type record struct {
	Type      string `json:"type"`
	Cwd       string `json:"cwd"`
	GitBranch string `json:"gitBranch"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
	Message   *struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

func digestFile(cfg Config, path, project string) (Session, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, false, err
	}
	defer f.Close()

	s := Session{Project: project}
	repos, branches := newSet(), newSet()
	linear, jira, issues := newSet(), newSet(), newSet()
	var prompts []string
	var hasPersonal, hasInternal bool

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var r record
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		if r.SessionID != "" {
			s.SessionID = r.SessionID
		}
		if r.Type == "ai-title" && r.Title != "" {
			s.Title = r.Title
		}
		if ts := parseTime(r.Timestamp); !ts.IsZero() {
			if s.Started.IsZero() || ts.Before(s.Started) {
				s.Started = ts
			}
			if ts.After(s.Ended) {
				s.Ended = ts
			}
		}
		if r.Cwd != "" {
			p, i := classifyCwd(cfg, r.Cwd)
			hasPersonal = hasPersonal || p
			hasInternal = hasInternal || i
		}
		if repo := repoName(cfg, r.Cwd); repo != "" {
			repos.add(repo)
		}
		if r.GitBranch != "" {
			branches.add(r.GitBranch)
		}
		if r.Type == "user" && r.Message != nil {
			if txt := userText(r.Message.Content); txt != "" {
				s.UserMessages++
				prompts = append(prompts, txt)
				extractKeys(cfg, txt, linear, jira)
				extractIssues(txt, issues)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Session{}, false, err
	}
	if s.SessionID == "" || s.Ended.IsZero() {
		return Session{}, false, nil
	}
	s.Context = ticketContext(hasPersonal, hasInternal)
	s.Repos, s.Branches = repos.sorted(), branches.sorted()
	s.Tickets = resolveTickets(s.Context, linear, jira, issues)
	s.FirstPrompts, s.LastPrompts = head(prompts, 3), tail(prompts, 3)
	return s, true, nil
}

// repoName returns the repo basename for a checkout path, or "" for tmp/other
// paths that aren't repos (nothing in cfg.RepoPathMarkers matches).
func repoName(cfg Config, cwd string) string {
	if cwd == "" {
		return ""
	}
	for _, m := range cfg.RepoPathMarkers {
		if strings.Contains(cwd, m) {
			return filepath.Base(cwd)
		}
	}
	return ""
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func userText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return clip(str)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return clip(strings.Join(parts, " "))
	}
	return ""
}

func clip(s string) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	// Skip command/system noise injected as user turns.
	if strings.HasPrefix(s, "<") || s == "" {
		return ""
	}
	if len(s) > 240 {
		s = s[:240] + "…"
	}
	return s
}

func head(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

func tail(xs []string, n int) []string {
	if len(xs) > n {
		return xs[len(xs)-n:]
	}
	return xs
}

type set struct{ m map[string]struct{} }

func newSet() *set          { return &set{m: map[string]struct{}{}} }
func (s *set) add(x string) { s.m[x] = struct{}{} }
func (s *set) sorted() []string {
	out := make([]string, 0, len(s.m))
	for k := range s.m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
