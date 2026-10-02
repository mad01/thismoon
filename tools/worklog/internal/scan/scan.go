// Package scan reads local Claude Code session transcripts and distills each
// recent session into a compact digest. It exists so the /worklog-backfill
// skill can reason over a small JSON summary instead of pulling megabytes of
// raw transcript into context.
package scan

import (
	"bufio"
	"cmp"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/confdir"
)

// Session is the compact digest of one transcript file.
type Session struct {
	SessionID string    `json:"session_id"`
	Project   string    `json:"project"`
	Started   time.Time `json:"started"`
	Ended     time.Time `json:"ended"`
	Title     string    `json:"title,omitempty"`
	// Context is the ticket world this session belongs to, derived from the
	// paths it touched (cwd lines and the checkout paths named in tool-call
	// inputs): "personal" (github.com/mad01 → Linear MAD-NN, plus legacy
	// mad01/issues #NN), "internal" (work → Jira TEAM-NN), "mixed", or
	// "unknown". The two worlds must never cross-reference, so Tickets is
	// filtered to match Context.
	Context      string   `json:"context"`
	Repos        []string `json:"repos,omitempty"`
	Branches     []string `json:"branches,omitempty"`
	Tickets      []string `json:"tickets,omitempty"`
	UserMessages int      `json:"user_messages"`
	// Days splits activity by calendar date so a daily consumer can attribute
	// a multi-day session's turns. Keys are dates in the location Scan was
	// given (the CLI passes time.Now(), so local dates). Every dated line of
	// any type (assistant, attachment, system, queue-operation) creates its
	// day, so a day can carry a zero count. A session is only emitted when it
	// has a dated line, so the key is always present; omitempty is a
	// formality.
	Days         map[string]DayActivity `json:"days,omitempty"`
	FirstPrompts []string               `json:"first_prompts,omitempty"`
	LastPrompts  []string               `json:"last_prompts,omitempty"`
}

// DayActivity is one calendar day's share of a session.
type DayActivity struct {
	UserMessages int `json:"user_messages"`
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

// Config carries the ticket-firewall strings. The machine's real values ship
// via the consuming repo's config overlay (worklog's config.yaml); the three
// classification lists have no compiled-in values at all, so an unconfigured
// scan classifies nothing rather than guessing with another machine's markers.
type Config struct {
	LinearPrefixes      []string
	PersonalPathMarkers []string
	InternalPathMarkers []string
	CheckoutRoots       []string
	RepoPathMarkers     []string
}

// defaultConfig holds only the path shapes that describe a checkout layout
// rather than a person: where GOPATH-style trees are rooted and which path
// fragments mean "this is a repo". Which key prefixes and which directories
// belong to the personal or internal world is machine-specific by nature and
// is left empty on purpose — see WithDefaults.
var defaultConfig = Config{
	CheckoutRoots:   []string{"/code/src/"},
	RepoPathMarkers: []string{"/code/", "/workspace/"},
}

// WithDefaults returns c with the two path-shape fields filled from the
// built-in defaults when they are empty. The classification lists pass
// through as written, empty included: with none of them set the firewall has
// nothing to route by, so every session comes back as "unknown" context with
// all of its ticket references surfaced for human review. Scan applies this
// before scanning; `worklog config` applies it to print the settings in
// effect.
func (c Config) WithDefaults() Config {
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

// classifyPath reports which ticket world a path belongs to, whether it is a
// line's cwd or a checkout path named in a tool call. A path can match
// neither (e.g. a tmp dir), which leaves the session "unknown". Beyond the
// configured markers, GOPATH-style checkouts of any non-github.com host count
// as internal: that split is derived, never enumerated (same principle as
// belt's public/internal remote check).
func classifyPath(cfg Config, p string) (personal, internal bool) {
	for _, m := range cfg.PersonalPathMarkers {
		if strings.Contains(p, m) {
			personal = true
			break
		}
	}
	for _, m := range cfg.InternalPathMarkers {
		if strings.Contains(p, m) {
			internal = true
			break
		}
	}
	return personal, internal || internalHostCheckout(cfg, p)
}

// internalHostCheckout reports whether p sits under a GOPATH-style checkout
// of a non-github.com git host: a host-shaped segment (contains a dot)
// directly under a checkout root that isn't github.com.
func internalHostCheckout(cfg Config, p string) bool {
	for _, root := range cfg.CheckoutRoots {
		_, rest, ok := strings.Cut(p, root)
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

// DefaultProjectsDir is the transcript root when $CLAUDE_PROJECTS_DIR is
// unset. The leading ~ is expanded at runtime, never at build time.
const DefaultProjectsDir = "~/.claude/projects"

// ProjectsDir returns $CLAUDE_PROJECTS_DIR, or DefaultProjectsDir expanded.
// An unresolvable home directory is an error rather than a directory relative
// to wherever the process started.
func ProjectsDir() (string, error) {
	if d := os.Getenv("CLAUDE_PROJECTS_DIR"); d != "" {
		return confdir.Expand(d)
	}
	return confdir.Expand(DefaultProjectsDir)
}

// Scan returns digests of every session whose last activity is within the
// window ending at now, newest first. Zero-value cfg fields fall back to the
// built-in defaults. Per-day activity is keyed by the calendar date in now's
// location, so a caller passing time.Now() gets local dates.
func Scan(root string, since time.Duration, now time.Time, cfg Config) ([]Session, error) {
	if root == "" {
		d, err := ProjectsDir()
		if err != nil {
			return nil, err
		}
		root = d
	}
	sc := &scanner{cfg: cfg.WithDefaults(), loc: now.Location(), seen: map[string]bool{}}
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
			s, ok, err := sc.digestFile(f, p.Name())
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
	// Claude Code writes the session title as {"type":"ai-title","aiTitle":…}
	// and rewrites that line as the session evolves. Title is the key the
	// first fixtures used; both are read so older transcripts still resolve.
	AITitle string `json:"aiTitle"`
	Title   string `json:"title"`
	Message *struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// sessionTitle returns the title an ai-title line carries, or "" for any
// other line or an ai-title line with neither key set.
func (r record) sessionTitle() string {
	if r.Type != "ai-title" {
		return ""
	}
	return cmp.Or(r.AITitle, r.Title)
}

// scanner holds what every transcript digest shares: the firewall config, the
// location that keys per-day activity, and the on-disk probes already made.
type scanner struct {
	cfg  Config
	loc  *time.Location
	seen map[string]bool
}

// onDisk reports whether an absolute path exists on this machine, memoised
// per scan.
func (sc *scanner) onDisk(p string) bool {
	hit, ok := sc.seen[p]
	if !ok {
		_, err := os.Stat(p)
		hit = err == nil
		sc.seen[p] = hit
	}
	return hit
}

// checkout resolves a path to the git checkout it sits in, or "" when there
// is none. The path is expanded against the home directory, cut to its
// checkout by checkoutDir, and kept only when that directory has a .git
// entry here. That directory, never the raw token, is what the digest
// classifies and lists: a doc example, a scratch directory that exists but
// is no checkout, or a made-up suffix under a real checkout can name no
// world the checkout itself does not. Tool-call paths and cwd lines both go
// through it; a cwd that resolves to nothing has a fallback (see addCwd).
func (sc *scanner) checkout(p string) string {
	full, err := confdir.Expand(p)
	if err != nil {
		return ""
	}
	dir := checkoutDir(sc.cfg, full)
	if dir == "" || !sc.onDisk(path.Join(dir, ".git")) {
		return ""
	}
	return dir
}

func (sc *scanner) digestFile(path, project string) (Session, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, false, err
	}
	defer f.Close()

	d := newDigest(sc, project)
	lines := bufio.NewScanner(f)
	lines.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	for lines.Scan() {
		var r record
		if json.Unmarshal(lines.Bytes(), &r) != nil {
			continue
		}
		d.add(r)
	}
	if err := lines.Err(); err != nil {
		return Session{}, false, err
	}
	s, ok := d.session()
	return s, ok, nil
}

// digest accumulates one transcript's lines into a Session.
type digest struct {
	*scanner
	s                    Session
	repos, branches      *set
	linear, jira, issues *set
	prompts              []string
	personal, internal   bool
	days                 map[string]DayActivity
}

func newDigest(sc *scanner, project string) *digest {
	return &digest{
		scanner:  sc,
		s:        Session{Project: project},
		repos:    newSet(),
		branches: newSet(),
		linear:   newSet(),
		jira:     newSet(),
		issues:   newSet(),
		days:     map[string]DayActivity{},
	}
}

// add folds one transcript line into the digest.
func (d *digest) add(r record) {
	if r.SessionID != "" {
		d.s.SessionID = r.SessionID
	}
	// The last ai-title line wins: Claude Code rewrites it as a session
	// evolves, so the latest one names the work best.
	if t := r.sessionTitle(); t != "" {
		d.s.Title = t
	}
	day := d.addTime(parseTime(r.Timestamp))
	if r.Cwd != "" {
		d.addCwd(r.Cwd)
	}
	if r.GitBranch != "" {
		d.branches.add(r.GitBranch)
	}
	if r.Message == nil {
		return
	}
	switch r.Type {
	case "user":
		d.addUser(r.Message.Content, day)
	case "assistant":
		d.addToolPaths(r.Message.Content)
	}
}

// addTime widens the session window to ts and returns the day key ts falls
// on, or "" for a line without a timestamp. Every dated line creates its day
// whatever its type, so a day can end up with a zero count.
func (d *digest) addTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	if d.s.Started.IsZero() || ts.Before(d.s.Started) {
		d.s.Started = ts
	}
	if ts.After(d.s.Ended) {
		d.s.Ended = ts
	}
	day := ts.In(d.loc).Format(time.DateOnly)
	if _, ok := d.days[day]; !ok {
		d.days[day] = DayActivity{}
	}
	return day
}

// addUser counts a user turn and mines it for ticket references. Command and
// system noise injected as user turns (see clip) is dropped before counting.
func (d *digest) addUser(content json.RawMessage, day string) {
	txt := userText(content)
	if txt == "" {
		return
	}
	d.s.UserMessages++
	if day != "" {
		a := d.days[day]
		a.UserMessages++
		d.days[day] = a
	}
	d.prompts = append(d.prompts, txt)
	extractKeys(d.cfg, txt, d.linear, d.jira)
	extractIssues(txt, d.issues)
}

// addCwd folds a line's cwd into the digest. A cwd inside a checkout counts
// as that checkout, exactly like a tool-call path: a session run from a
// subdirectory lists the repo, not the subdirectory, and the checkout is
// what the firewall classifies. A cwd that resolves to no checkout is still
// classified as written, since it is where the session really ran, but it
// names a repo only when this machine cannot probe it: a checkout that lives
// on another machine keeps its basename, while the org directory above the
// checkouts or a plain directory under a marker exists here and names
// nothing. Claude Code records cwd absolute, so the probe needs no expansion.
func (d *digest) addCwd(cwd string) {
	if dir := d.checkout(cwd); dir != "" {
		d.addCheckout(dir)
		return
	}
	d.classify(cwd)
	if d.onDisk(cwd) {
		return
	}
	if repo := repoName(d.cfg, cwd); repo != "" {
		d.repos.add(repo)
	}
}

// addToolPaths folds in every git checkout an assistant turn's tool calls
// reach (see checkout). A checkout counts exactly like a cwd line: a session
// started in a tmp dir that edits files under one belongs to its world and
// lists it as a repo.
func (d *digest) addToolPaths(content json.RawMessage) {
	for _, p := range toolInputPaths(content) {
		if dir := d.checkout(p); dir != "" {
			d.addCheckout(dir)
		}
	}
}

// addCheckout classifies a resolved checkout directory and lists it under
// repos by its basename.
func (d *digest) addCheckout(dir string) {
	d.classify(dir)
	d.repos.add(filepath.Base(dir))
}

// classify ORs a path's world into the session's. cwd lines and tool-call
// paths are pooled with no precedence between them: a session that touches
// both worlds through either source is "mixed", which surfaces every ticket
// for human review rather than filing it by whichever source happened to win.
func (d *digest) classify(p string) {
	personal, internal := classifyPath(d.cfg, p)
	d.personal = d.personal || personal
	d.internal = d.internal || internal
}

// session finalises the digest. A transcript with no session id or no dated
// line is not a session.
func (d *digest) session() (Session, bool) {
	if d.s.SessionID == "" || d.s.Ended.IsZero() {
		return Session{}, false
	}
	s := d.s
	s.Context = ticketContext(d.personal, d.internal)
	s.Repos, s.Branches = d.repos.sorted(), d.branches.sorted()
	s.Tickets = resolveTickets(s.Context, d.linear, d.jira, d.issues)
	s.FirstPrompts, s.LastPrompts = head(d.prompts, 3), tail(d.prompts, 3)
	if len(d.days) > 0 {
		s.Days = d.days
	}
	return s, true
}

// repoName is the fallback name for a cwd this machine cannot probe (see
// addCwd): its basename, or "" for tmp/other paths that aren't repos (nothing
// in cfg.RepoPathMarkers matches). It reads the path alone and cannot tell a
// checkout from a directory beside one, which is why it only applies when no
// probe is possible.
func repoName(cfg Config, cwd string) string {
	if cwd == "" || !underRepoMarker(cfg, cwd) {
		return ""
	}
	return filepath.Base(cwd)
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
