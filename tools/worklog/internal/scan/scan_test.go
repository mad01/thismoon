package scan

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func writeSession(t *testing.T, root, project, name string, lines []string) {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// configured is a machine's firewall config: the three classification lists
// have no built-in values, so every test that exercises classification has to
// supply them the way a real config file does.
var configured = Config{
	LinearPrefixes:      []string{"MAD"},
	PersonalPathMarkers: []string{"github.com/mad01/"},
	InternalPathMarkers: []string{"/workspace/"},
}

func TestScanDigestsAndFilters(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)

	writeSession(t, root, "proj-a", "recent", []string{
		`{"type":"ai-title","title":"worklog design","sessionId":"S1"}`,
		`{"type":"user","sessionId":"S1","cwd":"/Users/x/code/src/github.com/mad01/dotfiles","gitBranch":"main","timestamp":"2026-06-16T09:00:00.000Z","message":{"role":"user","content":"work on MAD-123 and ABC-1234 see https://github.com/mad01/issues/issues/52 and issue #51"}}`,
		`{"type":"user","sessionId":"S1","cwd":"/Users/x/code/src/github.com/mad01/ralph","gitBranch":"main","timestamp":"2026-06-16T10:00:00.000Z","message":{"role":"user","content":[{"type":"text","text":"now the ralph side"}]}}`,
		`{"type":"user","sessionId":"S1","cwd":"/tmp/abc","timestamp":"2026-06-16T10:30:00.000Z","message":{"role":"user","content":"<command-stdout>noise</command-stdout>"}}`,
	})

	writeSession(t, root, "proj-a", "stale", []string{
		`{"type":"user","sessionId":"S0","cwd":"/Users/x/code/old","timestamp":"2026-04-01T09:00:00.000Z","message":{"role":"user","content":"old work"}}`,
	})

	sessions, err := Scan(root, 14*24*time.Hour, now, configured)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 in-window session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.Title != "worklog design" {
		t.Errorf("title = %q", s.Title)
	}
	// cwds are all under github.com/mad01 → personal context. The firewall keeps
	// only personal-world refs: the Linear key MAD-123 plus legacy mad01/issues
	// refs (#52 from the URL, #51 from prose). The stray Jira key ABC-1234 is
	// dropped because a personal task never references Jira.
	if s.Context != "personal" {
		t.Errorf("context = %q, want personal", s.Context)
	}
	wantTickets := map[string]bool{"MAD-123": true, "#52": true, "#51": true}
	if len(s.Tickets) != len(wantTickets) {
		t.Errorf("tickets = %v, want %v", s.Tickets, wantTickets)
	}
	for _, tk := range s.Tickets {
		if !wantTickets[tk] {
			t.Errorf("unexpected ticket %q in %v (firewall leak?)", tk, s.Tickets)
		}
	}
	if len(s.Repos) != 2 { // dotfiles + ralph; /tmp excluded
		t.Errorf("repos = %v", s.Repos)
	}
	if s.UserMessages != 2 { // command-noise turn dropped
		t.Errorf("user messages = %d", s.UserMessages)
	}
}

// TestSessionTitle pins the ai-title contract: Claude Code writes the title
// under "aiTitle" and rewrites the line as the session evolves, so the last
// one wins. The older "title" key is still read so legacy fixtures resolve.
func TestSessionTitle(t *testing.T) {
	// userLine gives each fixture the session id and timestamp Scan needs to
	// emit a session at all.
	const userLine = `{"type":"user","sessionId":"S1","cwd":"/tmp/x","timestamp":"2026-06-16T09:00:00.000Z","message":{"role":"user","content":"hi"}}`
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name: "aiTitle key is read",
			lines: []string{
				`{"type":"ai-title","aiTitle":"Herding space and agent naming","sessionId":"S1"}`,
				userLine,
			},
			want: "Herding space and agent naming",
		},
		{
			name: "last ai-title line wins",
			lines: []string{
				`{"type":"ai-title","aiTitle":"first draft","sessionId":"S1"}`,
				userLine,
				`{"type":"ai-title","aiTitle":"scan title fix","sessionId":"S1"}`,
			},
			want: "scan title fix",
		},
		{
			name: "legacy title key still resolves",
			lines: []string{
				`{"type":"ai-title","title":"worklog design","sessionId":"S1"}`,
				userLine,
			},
			want: "worklog design",
		},
		{
			name: "empty ai-title line keeps the earlier title",
			lines: []string{
				`{"type":"ai-title","aiTitle":"kept","sessionId":"S1"}`,
				userLine,
				`{"type":"ai-title","aiTitle":"","sessionId":"S1"}`,
			},
			want: "kept",
		},
		{
			name:  "no ai-title line",
			lines: []string{userLine},
			want:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeSession(t, root, "proj", "s", tc.lines)
			sessions, err := Scan(root, 14*24*time.Hour, now, configured)
			if err != nil {
				t.Fatal(err)
			}
			if len(sessions) != 1 {
				t.Fatalf("sessions = %d, want 1", len(sessions))
			}
			if got := sessions[0].Title; got != tc.want {
				t.Errorf("title = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveTicketsFirewall(t *testing.T) {
	cases := []struct {
		name    string
		context string
		text    string
		want    []string
	}{
		{
			name:    "personal keeps linear + legacy issues, drops jira",
			context: "personal",
			text:    "MAD-123 and ABC-1234 and issue #51",
			want:    []string{"#51", "MAD-123"},
		},
		{
			name:    "internal keeps jira, drops linear + issues",
			context: "internal",
			text:    "MAD-123 and ABC-1234 and issue #51",
			want:    []string{"ABC-1234"},
		},
		{
			name:    "mixed surfaces everything for review",
			context: "mixed",
			text:    "MAD-7 and ABC-9 and #3",
			want:    []string{"#3", "ABC-9", "MAD-7"},
		},
		{
			name:    "unknown surfaces everything",
			context: "unknown",
			text:    "MAD-7 and ABC-9",
			want:    []string{"ABC-9", "MAD-7"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			linear, jira, issues := newSet(), newSet(), newSet()
			extractKeys(configured.WithDefaults(), tc.text, linear, jira)
			extractIssues(tc.text, issues)
			got := resolveTickets(tc.context, linear, jira, issues)
			if len(got) != len(tc.want) {
				t.Fatalf("tickets = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("tickets = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestClassifyPath(t *testing.T) {
	cfg := configured.WithDefaults()
	cases := []struct {
		name     string
		cwd      string
		personal bool
		internal bool
	}{
		{"personal checkout", "/Users/x/code/src/github.com/mad01/dotfiles", true, false},
		{"workspace path", "/Users/x/workspace/some-service", false, true},
		{
			"non-github host checkout",
			"/Users/x/code/src/git.internal.example/org/repo",
			false,
			true,
		},
		{"other github org", "/Users/x/code/src/github.com/other/repo", false, false},
		{"tmp dir", "/tmp/scratch", false, false},
		{"host segment without dot", "/Users/x/code/src/local/repo", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, i := classifyPath(cfg, tc.cwd)
			if p != tc.personal || i != tc.internal {
				t.Errorf("classifyPath(%q) = (%v, %v), want (%v, %v)",
					tc.cwd, p, i, tc.personal, tc.internal)
			}
		})
	}
}

// line encodes one transcript line the way Claude Code writes it.
func line(t *testing.T, fields map[string]any) string {
	t.Helper()
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// userLine is a user turn with a plain-string prompt.
func userLine(t *testing.T, cwd, ts, text string) string {
	t.Helper()
	return line(t, map[string]any{
		"type": "user", "sessionId": "S1", "cwd": cwd, "timestamp": ts,
		"message": map[string]any{"role": "user", "content": text},
	})
}

// toolLine is an assistant turn carrying one tool_use block.
func toolLine(t *testing.T, cwd, ts, tool string, input map[string]any) string {
	t.Helper()
	return line(t, map[string]any{
		"type": "assistant", "sessionId": "S1", "cwd": cwd, "timestamp": ts,
		"message": map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "text", "text": "on it"},
			map[string]any{"type": "tool_use", "id": "t1", "name": tool, "input": input},
		}},
	})
}

// fakeHome points HOME at a temp dir and returns it with two builders: mkdir
// makes a plain directory under it, checkout makes one with a .git entry.
// A path only counts as a checkout when that entry exists on this machine,
// so tests that exercise the resolver lay their checkouts out for real.
func fakeHome(t *testing.T) (home string, mkdir, checkout func(rel string) string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	mkdir = func(rel string) string {
		t.Helper()
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	checkout = func(rel string) string {
		t.Helper()
		mkdir(rel + "/.git")
		return filepath.Join(home, rel)
	}
	return home, mkdir, checkout
}

// TestToolPathsAndDays covers what the digest reads beyond cwd: checkout paths
// named in tool-call inputs, and the per-day split of user messages. The
// checkouts are real directories under a temp home (HOME is pointed there for
// the ~/ case).
func TestToolPathsAndDays(t *testing.T) {
	_, mkdir, checkout := fakeHome(t)
	tmp := mkdir(".tmp/k3j9x")
	bin := mkdir("code/bin")
	mkdir("workspace/docs") // exists under the internal marker, no .git
	dotfiles := checkout("code/src/github.com/mad01/dotfiles")
	ralph := checkout("code/src/github.com/mad01/ralph")
	billing := checkout("workspace/billing-api")
	utc := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	plusTwo := utc.In(time.FixedZone("UTC+2", 2*60*60))
	twoDays := []string{
		// 23:30 UTC on the 15th is 01:30 on the 16th at UTC+2.
		userLine(t, dotfiles, "2026-06-15T23:30:00Z", "start"),
		userLine(t, dotfiles, "2026-06-16T10:00:00Z", "continue"),
		// A day with tool activity but no user turn still gets an entry.
		toolLine(t, dotfiles, "2026-06-17T06:00:00Z", "Read",
			map[string]any{"file_path": dotfiles + "/README.md"}),
		userLine(t, dotfiles, "2026-06-17T09:00:00Z", "<command-stdout>noise</command-stdout>"),
		userLine(t, dotfiles, "2026-06-17T09:30:00Z", "wrap up"),
	}
	cases := []struct {
		name         string
		now          time.Time
		lines        []string
		wantRepos    []string
		wantContext  string
		wantTickets  []string
		wantMessages int
		wantDays     map[string]DayActivity
	}{
		{
			name: "tmp cwd, tool inputs name the checkouts",
			now:  utc,
			lines: []string{
				userLine(t, tmp, "2026-06-16T09:00:00Z", "fix the scanner"),
				// The file need not exist: the probe is at the checkout.
				toolLine(t, tmp, "2026-06-16T09:01:00Z", "Read",
					map[string]any{"file_path": dotfiles + "/recipes/mise/config.toml"}),
				// A ~/ path expands against HOME for the probe.
				toolLine(
					t,
					tmp,
					"2026-06-16T09:02:00Z",
					"Bash",
					map[string]any{
						"command":     "cd ~/code/src/github.com/mad01/ralph && make build",
						"description": "Build",
					},
				),
				toolLine(t, tmp, "2026-06-16T09:03:00Z", "Bash",
					map[string]any{"command": "git -C " + dotfiles + " status --short"}),
				// Not paths: a csl repo slug and a URL name no checkout.
				toolLine(t, tmp, "2026-06-16T09:04:00Z", "mcp__csl__csl_read",
					map[string]any{"repo": "mad01/thismoon", "file": "README.md"}),
				toolLine(
					t,
					tmp,
					"2026-06-16T09:05:00Z",
					"WebFetch",
					map[string]any{
						"url":    "https://github.com/mad01/ralph/pull/5",
						"prompt": "sum up",
					},
				),
			},
			wantRepos:    []string{"dotfiles", "ralph"},
			wantContext:  "personal",
			wantMessages: 1,
			wantDays:     map[string]DayActivity{"2026-06-16": {UserMessages: 1}},
		},
		{
			name: "cwd and tool paths disagree: mixed",
			now:  utc,
			lines: []string{
				userLine(t, dotfiles, "2026-06-16T09:00:00Z", "compare against the billing repo"),
				toolLine(t, dotfiles, "2026-06-16T09:01:00Z", "Agent", map[string]any{
					"prompt": "Read " + billing + "/README.md and report back.", "subagent_type": "Explore",
				}),
			},
			wantRepos:    []string{"billing-api", "dotfiles"},
			wantContext:  "mixed",
			wantMessages: 1,
			wantDays:     map[string]DayActivity{"2026-06-16": {UserMessages: 1}},
		},
		{
			// Reviewer's case A: the directory exists under the internal marker
			// but is no checkout, so it must not set context or drop the
			// Linear key by filing the session internal.
			name: "an existing non-checkout under a marker sets no context",
			now:  utc,
			lines: []string{
				userLine(t, tmp, "2026-06-16T09:00:00Z", "track MAD-999"),
				toolLine(t, tmp, "2026-06-16T09:01:00Z", "Write", map[string]any{
					"file_path": tmp + "/notes.md",
					"content":   "Scratch notes go in ~/workspace/docs",
				}),
			},
			wantContext:  "unknown",
			wantTickets:  []string{"MAD-999"},
			wantMessages: 1,
			wantDays:     map[string]DayActivity{"2026-06-16": {UserMessages: 1}},
		},
		{
			// Reviewer's case B: a made-up suffix under a real checkout must
			// classify the checkout, not the token's marker-shaped tail.
			name: "a made-up suffix under a real checkout classifies the checkout",
			now:  utc,
			lines: []string{
				userLine(t, dotfiles, "2026-06-16T09:00:00Z", "edit the doc"),
				toolLine(t, dotfiles, "2026-06-16T09:01:00Z", "Edit", map[string]any{
					"file_path":  dotfiles + "/README.md",
					"old_string": "x",
					"new_string": "see " + dotfiles + "/no/such/workspace/file.md",
				}),
			},
			wantRepos:    []string{"dotfiles"},
			wantContext:  "personal",
			wantMessages: 1,
			wantDays:     map[string]DayActivity{"2026-06-16": {UserMessages: 1}},
		},
		{
			name: "paths that resolve to no checkout are ignored",
			now:  utc,
			lines: []string{
				userLine(t, dotfiles, "2026-06-16T09:00:00Z", "document the markers"),
				toolLine(t, dotfiles, "2026-06-16T09:01:00Z", "Write", map[string]any{
					"file_path": dotfiles + "/config.md",
					"content":   "internal example: /Users/example/workspace/foo/bar and ~/workspace/acme/svc",
				}),
				toolLine(t, dotfiles, "2026-06-16T09:02:00Z", "Bash",
					map[string]any{"command": "ls " + ralph + "-gone"}),
				// Exists, sits under a repo path marker, but has no .git.
				toolLine(t, dotfiles, "2026-06-16T09:03:00Z", "Bash",
					map[string]any{"command": bin + "/worklog version"}),
			},
			wantRepos:    []string{"dotfiles"},
			wantContext:  "personal",
			wantMessages: 1,
			wantDays:     map[string]DayActivity{"2026-06-16": {UserMessages: 1}},
		},
		{
			name:         "lines across two dates, keyed in now's location",
			now:          plusTwo,
			lines:        twoDays,
			wantRepos:    []string{"dotfiles"},
			wantContext:  "personal",
			wantMessages: 3,
			wantDays: map[string]DayActivity{
				"2026-06-16": {UserMessages: 2},
				"2026-06-17": {UserMessages: 1},
			},
		},
		{
			name:         "same lines keyed in UTC",
			now:          utc,
			lines:        twoDays,
			wantRepos:    []string{"dotfiles"},
			wantContext:  "personal",
			wantMessages: 3,
			wantDays: map[string]DayActivity{
				"2026-06-15": {UserMessages: 1},
				"2026-06-16": {UserMessages: 1},
				"2026-06-17": {UserMessages: 1},
			},
		},
		{
			name: "cwd-only session at the checkout roots",
			now:  utc,
			lines: []string{
				userLine(t, dotfiles, "2026-06-16T09:00:00Z", "work on MAD-123"),
				userLine(t, ralph, "2026-06-16T10:00:00Z", "now the ralph side"),
				userLine(
					t,
					"/tmp/abc",
					"2026-06-16T10:30:00Z",
					"<command-stdout>noise</command-stdout>",
				),
			},
			wantRepos:    []string{"dotfiles", "ralph"},
			wantContext:  "personal",
			wantTickets:  []string{"MAD-123"},
			wantMessages: 2,
			wantDays:     map[string]DayActivity{"2026-06-16": {UserMessages: 2}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeSession(t, root, "proj", "s", tc.lines)
			sessions, err := Scan(root, 14*24*time.Hour, tc.now, configured)
			if err != nil {
				t.Fatal(err)
			}
			if len(sessions) != 1 {
				t.Fatalf("sessions = %d, want 1", len(sessions))
			}
			s := sessions[0]
			if !slices.Equal(s.Repos, tc.wantRepos) {
				t.Errorf("repos = %v, want %v", s.Repos, tc.wantRepos)
			}
			if s.Context != tc.wantContext {
				t.Errorf("context = %q, want %q", s.Context, tc.wantContext)
			}
			if !slices.Equal(s.Tickets, tc.wantTickets) {
				t.Errorf("tickets = %v, want %v", s.Tickets, tc.wantTickets)
			}
			if s.UserMessages != tc.wantMessages {
				t.Errorf("user messages = %d, want %d", s.UserMessages, tc.wantMessages)
			}
			if !maps.Equal(s.Days, tc.wantDays) {
				t.Errorf("days = %v, want %v", s.Days, tc.wantDays)
			}
		})
	}
}

// TestCwdNamesItsCheckout pins how a line's cwd is named and classified
// (MAD-367). A cwd inside a checkout counts as that checkout, like a
// tool-call path, so a session run from a subdirectory lists the repo and
// not the subdirectory, and the firewall reads the checkout rather than the
// cwd's own segments. A cwd that resolves to no checkout is classified as
// written but names a repo only when this machine cannot probe it: a
// checkout that lives elsewhere keeps its basename, a directory that exists
// here and is no checkout names nothing.
func TestCwdNamesItsCheckout(t *testing.T) {
	home, mkdir, checkout := fakeHome(t)
	dotfiles := checkout("code/src/github.com/mad01/dotfiles")
	billing := checkout("workspace/billing-api")
	docs := mkdir("workspace/docs")
	cases := []struct {
		name        string
		cwd         string
		wantRepos   []string
		wantContext string
	}{
		{"the checkout root", dotfiles, []string{"dotfiles"}, "personal"},
		{
			"a subdirectory of a checkout",
			dotfiles + "/tools/worklog",
			[]string{"dotfiles"},
			"personal",
		},
		{
			"a subdirectory of an internal checkout",
			billing + "/cmd/api",
			[]string{"billing-api"},
			"internal",
		},
		{
			// The checkout is classified, not the cwd: a subdirectory whose
			// name matches the internal marker leaves a personal checkout
			// personal instead of mixed.
			"a marker-shaped subdirectory classifies the checkout",
			dotfiles + "/workspace/notes",
			[]string{"dotfiles"},
			"personal",
		},
		// No checkout resolves for the rest. Only the cwd this machine cannot
		// probe keeps its basename; the directories that exist here name
		// nothing, while their world is still read off the path.
		{
			"a checkout this machine lacks keeps its basename",
			"/Users/x/code/src/github.com/mad01/ralph",
			[]string{"ralph"},
			"personal",
		},
		{
			"the org directory names no repo",
			filepath.Join(home, "code/src/github.com/mad01"),
			nil,
			"unknown",
		},
		{
			"the checkout root's parent names no repo",
			filepath.Join(home, "code/src"),
			nil,
			"unknown",
		},
		{
			"a plain directory under a marker names no repo",
			mkdir("code/bin"),
			nil,
			"unknown",
		},
		{
			"a scratch directory under the internal marker keeps its world",
			docs,
			nil,
			"internal",
		},
		{"a tmp dir names no repo", mkdir(".tmp/k3j9x"), nil, "unknown"},
	}
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeSession(t, root, "proj", "s", []string{
				userLine(t, tc.cwd, "2026-06-16T09:00:00Z", "hello"),
			})
			sessions, err := Scan(root, 14*24*time.Hour, now, configured)
			if err != nil {
				t.Fatal(err)
			}
			if len(sessions) != 1 {
				t.Fatalf("sessions = %d, want 1", len(sessions))
			}
			s := sessions[0]
			if !slices.Equal(s.Repos, tc.wantRepos) {
				t.Errorf("repos = %v, want %v", s.Repos, tc.wantRepos)
			}
			if s.Context != tc.wantContext {
				t.Errorf("context = %q, want %q", s.Context, tc.wantContext)
			}
		})
	}
}

// TestAbsoluteMarkerMatchesTildePath pins that a tool-call path is classified
// after expansion: an internal_path_markers entry written as an absolute path
// must match a ~/ path into the same checkout.
func TestAbsoluteMarkerMatchesTildePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "workspace/billing-api/.git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		LinearPrefixes:      []string{"MAD"},
		InternalPathMarkers: []string{filepath.Join(home, "workspace") + "/"},
	}
	root := t.TempDir()
	writeSession(t, root, "proj", "s", []string{
		userLine(t, "/tmp/abc", "2026-06-16T09:00:00Z", "look at the service"),
		toolLine(t, "/tmp/abc", "2026-06-16T09:01:00Z", "Read",
			map[string]any{"file_path": "~/workspace/billing-api/README.md"}),
	})
	now := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	sessions, err := Scan(root, 14*24*time.Hour, now, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if got := sessions[0].Context; got != "internal" {
		t.Errorf("context = %q, want internal (absolute marker vs ~/ path)", got)
	}
	if got := sessions[0].Repos; !slices.Equal(got, []string{"billing-api"}) {
		t.Errorf("repos = %v, want [billing-api]", got)
	}
}

func TestConfigOverrides(t *testing.T) {
	cfg := Config{
		LinearPrefixes:      []string{"XYZ"},
		PersonalPathMarkers: []string{"github.com/someone/"},
		InternalPathMarkers: []string{"/dayjob/"},
	}.WithDefaults()

	linear, jira := newSet(), newSet()
	extractKeys(cfg, "XYZ-12 and MAD-34", linear, jira)
	if got := linear.sorted(); len(got) != 1 || got[0] != "XYZ-12" {
		t.Errorf("linear keys = %v, want [XYZ-12]", got)
	}
	if got := jira.sorted(); len(got) != 1 || got[0] != "MAD-34" {
		t.Errorf("jira keys = %v, want [MAD-34]", got)
	}

	if p, _ := classifyPath(cfg, "/Users/x/code/src/github.com/someone/repo"); !p {
		t.Error("configured personal marker not honored")
	}
	if _, i := classifyPath(cfg, "/Users/x/dayjob/repo"); !i {
		t.Error("configured internal marker not honored")
	}
	if p, _ := classifyPath(cfg, "/Users/x/code/src/github.com/mad01/dotfiles"); p {
		t.Error("a path matching no configured marker should not be personal")
	}
}

// TestUnconfiguredClassificationIsInert pins the shipped defaults: with no
// config file worklog classifies nothing, so a session comes back "unknown"
// with every ticket reference surfaced rather than filed into somebody else's
// idea of personal and internal.
func TestUnconfiguredClassificationIsInert(t *testing.T) {
	cfg := Config{}.WithDefaults()

	if len(cfg.PersonalPathMarkers)+len(cfg.InternalPathMarkers)+len(cfg.LinearPrefixes) != 0 {
		t.Fatalf("classification defaults present, want none: %+v", cfg)
	}

	personal, internal := classifyPath(cfg, "/Users/x/code/src/github.com/mad01/dotfiles")
	if personal || internal {
		t.Errorf("classifyCwd = (%v, %v), want both false", personal, internal)
	}
	if got := ticketContext(personal, internal); got != "unknown" {
		t.Errorf("context = %q, want unknown", got)
	}

	linear, jira, issues := newSet(), newSet(), newSet()
	extractKeys(cfg, "MAD-1 and ABC-2 and #3", linear, jira)
	extractIssues("MAD-1 and ABC-2 and #3", issues)
	got := resolveTickets("unknown", linear, jira, issues)
	want := []string{"#3", "ABC-2", "MAD-1"}
	if len(got) != len(want) {
		t.Fatalf("tickets = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("tickets = %v, want %v", got, want)
		}
	}

	// The path-shape defaults still apply: repo detection is not part of the
	// firewall and works out of the box.
	if repoName(cfg, "/Users/x/code/src/github.com/mad01/dotfiles") != "dotfiles" {
		t.Error("repo detection should still work on the built-in path markers")
	}
}

func TestCheckoutRootsOverride(t *testing.T) {
	cfg := Config{CheckoutRoots: []string{"/checkouts/"}}.WithDefaults()
	if _, i := classifyPath(cfg, "/Users/x/checkouts/git.internal.example/org/repo"); !i {
		t.Error("configured checkout root not honored for internal-host detection")
	}
	if _, i := classifyPath(cfg, "/Users/x/code/src/git.internal.example/org/repo"); i {
		t.Error("default checkout root should be replaced by the configured one")
	}
}

func TestRepoName(t *testing.T) {
	def := Config{}.WithDefaults()
	cases := []struct {
		name string
		cfg  Config
		cwd  string
		want string
	}{
		{"code checkout", def, "/Users/x/code/src/github.com/mad01/dotfiles", "dotfiles"},
		{"workspace checkout", def, "/Users/x/workspace/some-service", "some-service"},
		{"tmp dir is not a repo", def, "/tmp/scratch", ""},
		{"empty cwd", def, "", ""},
		{
			"configured marker",
			Config{RepoPathMarkers: []string{"/repos/"}}.WithDefaults(),
			"/Users/x/repos/thing",
			"thing",
		},
		{
			"configured marker replaces default",
			Config{RepoPathMarkers: []string{"/repos/"}}.WithDefaults(),
			"/Users/x/code/src/github.com/mad01/dotfiles",
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := repoName(tc.cfg, tc.cwd); got != tc.want {
				t.Errorf("repoName(%q) = %q, want %q", tc.cwd, got, tc.want)
			}
		})
	}
}
