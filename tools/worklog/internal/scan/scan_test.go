package scan

import (
	"os"
	"path/filepath"
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

func TestClassifyCwd(t *testing.T) {
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
			p, i := classifyCwd(cfg, tc.cwd)
			if p != tc.personal || i != tc.internal {
				t.Errorf("classifyCwd(%q) = (%v, %v), want (%v, %v)",
					tc.cwd, p, i, tc.personal, tc.internal)
			}
		})
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

	if p, _ := classifyCwd(cfg, "/Users/x/code/src/github.com/someone/repo"); !p {
		t.Error("configured personal marker not honored")
	}
	if _, i := classifyCwd(cfg, "/Users/x/dayjob/repo"); !i {
		t.Error("configured internal marker not honored")
	}
	if p, _ := classifyCwd(cfg, "/Users/x/code/src/github.com/mad01/dotfiles"); p {
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

	personal, internal := classifyCwd(cfg, "/Users/x/code/src/github.com/mad01/dotfiles")
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
	if _, i := classifyCwd(cfg, "/Users/x/checkouts/git.internal.example/org/repo"); !i {
		t.Error("configured checkout root not honored for internal-host detection")
	}
	if _, i := classifyCwd(cfg, "/Users/x/code/src/git.internal.example/org/repo"); i {
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
