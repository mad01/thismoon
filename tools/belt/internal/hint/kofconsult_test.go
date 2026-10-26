package hint

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func gitCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd
}

func TestRepoFromOrigin(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "scp style", url: "git@github.com:mad01/thismoon.git", want: "mad01/thismoon"},
		{name: "https", url: "https://github.com/mad01/thismoon.git", want: "mad01/thismoon"},
		{
			name: "https without .git",
			url:  "https://github.com/mad01/thismoon",
			want: "mad01/thismoon",
		},
		{name: "ssh with port", url: "ssh://git@host.example:2222/org/name.git", want: "org/name"},
		{
			name: "uppercase is normalized",
			url:  "git@github.com:Mad01/ThisMoon.git",
			want: "mad01/thismoon",
		},
		{name: "host with no org", url: "https://host.example/name", want: ""},
		{name: "empty", url: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := repoFromOrigin(tc.url); got != tc.want {
				t.Errorf("repoFromOrigin(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

// The bug this pins: the prefix query for a repo also matches sibling repos
// sharing the name as a prefix, so the boundary after the repo name must be
// checked before advising.
func TestMatchRepoRespectsNameBoundary(t *testing.T) {
	as := []assertion{
		{ID: "root", Subject: "repo:mad01/thismoon"},
		{ID: "deep", Subject: "repo:mad01/thismoon/services/keeper-of-facts"},
		{ID: "sibling", Subject: "repo:mad01/thismoon-arcade/game"},
	}
	got := matchRepo("mad01/thismoon", as)
	if len(got) != 2 || got[0].ID != "root" || got[1].ID != "deep" {
		t.Errorf("matchRepo = %+v, want the root and deep assertions only", got)
	}
}

func consultServer(t *testing.T, as []assertion) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"assertions": as})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Empty session ids skip the seen-file dedupe, which keeps these tests off
// the real ~/.cache/belt.
func consultHint(base string, origin string) *KofConsult {
	return &KofConsult{
		cfg:       testConfig(),
		base:      base,
		originURL: func(string) string { return origin },
	}
}

// claimWords give each consultClaims assertion a word of its own. A word
// every assertion carries is not distinctive, so without one an assertion
// cannot pass the gate.
var claimWords = []string{
	"alpha", "bravo", "charlie", "delta", "foxtrot", "golf", "hotel", "india", "juliet", "kilo",
}

// consultPrompt overlaps "serve is the single writer" on three plain words
// and names every claimWords word.
var consultPrompt = "why is serve the single writer for " + strings.Join(claimWords, " ") + "?"

func TestKofConsultSurfacesRepoAssertions(t *testing.T) {
	srv := consultServer(t, []assertion{
		{
			ID:        "1",
			Subject:   "repo:mad01/thismoon/services/keeper-of-facts",
			Statement: "serve is the single writer",
			Status:    "fresh",
			Pins: []pin{{
				Repo: "github.com/mad01/thismoon", File: "services/keeper-of-facts/serve.go",
			}},
		},
		{
			ID:        "2",
			Subject:   "repo:mad01/thismoon-arcade/game",
			Statement: "serve is the single writer of sibling noise",
			Status:    "fresh",
		},
		{
			ID:        "3",
			Subject:   "repo:mad01/thismoon/tools/belt",
			Statement: "hints never deny",
			Status:    "fresh",
		},
	})
	h := consultHint(srv.URL, "git@github.com:mad01/thismoon.git")
	got := h.Check(Input{Event: EventPrompt, Cwd: t.TempDir(), Prompt: consultPrompt})
	if got == nil {
		t.Fatal("Check = nil, want advice for the assertion the prompt is about")
	}
	if !strings.Contains(got.Text, "serve is the single writer") {
		t.Errorf("advice %q does not carry the assertion statement", got.Text)
	}
	if strings.Contains(got.Text, "sibling noise") {
		t.Errorf("advice %q leaked a sibling repo's assertion", got.Text)
	}
	if strings.Contains(got.Text, "hints never deny") {
		t.Errorf("advice %q carries an assertion the prompt does not mention", got.Text)
	}
	want := map[string]string{
		"assertions": "1",
		"pins":       "github.com/mad01/thismoon/services/keeper-of-facts/serve.go",
	}
	if !maps.Equal(got.Tags, want) {
		t.Errorf("tags = %v, want %v", got.Tags, want)
	}
}

func consultClaims(n int) []assertion {
	var as []assertion
	for i := range n {
		as = append(as, assertion{
			ID:        string(rune('a' + i)),
			Subject:   "repo:mad01/thismoon/services/keeper-of-facts",
			Statement: "serve is the single writer of " + claimWords[i],
			Status:    "fresh",
		})
	}
	return as
}

// TestKofConsultCapsFirstThenLater: the first block a session gets carries
// up to five, later ones up to three, and each later block starts below
// what the session has already seen rather than repeating the top.
func TestKofConsultCapsFirstThenLater(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	h := consultHint(consultServer(t, consultClaims(10)).URL, "git@github.com:mad01/thismoon.git")
	in := Input{Event: EventPrompt, Cwd: t.TempDir(), Prompt: consultPrompt, SessionID: "caps"}
	for _, want := range []string{"a,b,c,d,e", "f,g,h", "i,j"} {
		got := h.Check(in)
		if got == nil {
			t.Fatalf("Check = nil, want assertions %s", want)
		}
		if ids := got.Tags["assertions"]; ids != want {
			t.Errorf("assertions = %q, want %q", ids, want)
		}
	}
	if got := h.Check(in); got != nil {
		t.Errorf("Check once every assertion was shown = %+v, want nil", got)
	}
}

// TestKofConsultFirstCapWaitsForTheFirstBlock: a prompt that surfaces
// nothing does not spend the first cap.
func TestKofConsultFirstCapWaitsForTheFirstBlock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	h := consultHint(consultServer(t, consultClaims(8)).URL, "git@github.com:mad01/thismoon.git")
	in := Input{Event: EventPrompt, Cwd: t.TempDir(), SessionID: "quiet-first"}
	in.Prompt = "rename the release workflow"
	if got := h.Check(in); got != nil {
		t.Fatalf("Check on an unrelated prompt = %+v, want nil", got)
	}
	in.Prompt = consultPrompt
	got := h.Check(in)
	if got == nil || got.Tags["assertions"] != "a,b,c,d,e" {
		t.Errorf("first block after a quiet prompt = %+v, want five assertions", got)
	}
}

func TestKofConsultWithoutSessionIDKeepsTheFirstCap(t *testing.T) {
	h := consultHint(consultServer(t, consultClaims(8)).URL, "git@github.com:mad01/thismoon.git")
	in := Input{Event: EventPrompt, Cwd: t.TempDir(), Prompt: consultPrompt}
	for range 2 {
		if got := h.Check(in); got == nil || !strings.Contains(got.Text, "5 stored assertion(s)") {
			t.Errorf("Check without a session id = %+v, want five assertions every time", got)
		}
	}
}

// TestKofConsultSilentOnAShortPrompt pins that a prompt too short to carry
// a topic never reaches kof: the stub fails the test if it is asked.
func TestKofConsultSilentOnAShortPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("kof was queried for a prompt below the length gate")
	}))
	t.Cleanup(srv.Close)
	h := consultHint(srv.URL, "git@github.com:mad01/thismoon.git")
	for _, prompt := range []string{"", "   ", "continue", "yes, do it"} {
		if got := h.Check(Input{Event: EventPrompt, Cwd: t.TempDir(), Prompt: prompt}); got != nil {
			t.Errorf("Check(%q) = %+v, want nil", prompt, got)
		}
	}
}

func TestKofConsultSilentOutsideARepo(t *testing.T) {
	h := consultHint("http://127.0.0.1:1", "")
	in := Input{Event: EventPrompt, Cwd: t.TempDir(), Prompt: consultPrompt}
	if got := h.Check(in); got != nil {
		t.Errorf("Check outside a repo = %+v, want nil", got)
	}
}

func TestKofConsultSilentWhenKofIsDown(t *testing.T) {
	h := consultHint("http://127.0.0.1:1", "git@github.com:mad01/thismoon.git")
	in := Input{Event: EventPrompt, Cwd: t.TempDir(), Prompt: consultPrompt}
	if got := h.Check(in); got != nil {
		t.Errorf("Check with kof down = %+v, want nil", got)
	}
}

// TestKofConsultRunsOnThePromptOnly: the hint moved from SessionStart to
// UserPromptSubmit, so `belt hint session-start` must no longer run it.
func TestKofConsultRunsOnThePromptOnly(t *testing.T) {
	has := func(event string) bool {
		return slices.ContainsFunc(ForEvent(event, testConfig()), func(h Hint) bool {
			return h.ID() == "kof-consult"
		})
	}
	if has(EventSessionStart) {
		t.Error("kof-consult is registered on session-start")
	}
	if !has(EventPrompt) {
		t.Error("kof-consult is not registered on prompt")
	}
}

func TestGitOriginURLReadsTheRealRemote(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := gitCommand(dir, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("remote", "add", "origin", "git@github.com:mad01/thismoon.git")
	for name, read := range map[string]func(string) string{
		"gitOriginURL":     gitOriginURL,
		"boundedOriginURL": boundedOriginURL,
	} {
		if got := read(dir); got != "git@github.com:mad01/thismoon.git" {
			t.Errorf("%s = %q, want the configured origin", name, got)
		}
		if got := read(t.TempDir()); got != "" {
			t.Errorf("%s outside a repo = %q, want empty", name, got)
		}
	}
}

// TestKofConsultRepoExcluded pins that an excluded repo is silent before any
// kof query happens: the stub fails the test if it is asked. Exclusion
// matches the canonical identity, read from the same origin URL as the
// org/name kof subjects use, in either remote form.
func TestKofConsultRepoExcluded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("kof was queried for an excluded repo")
	}))
	t.Cleanup(srv.Close)
	for _, origin := range []string{
		"git@github.com:mad01/thismoon.git",
		"https://github.com/mad01/thismoon",
	} {
		h := consultHint(srv.URL, origin)
		h.cfg = config.Config{Hints: map[string]config.Toggle{
			"kof-consult": {ExcludeRepos: []string{"github.com/mad01/thismoon"}},
		}}
		in := Input{Event: EventPrompt, Cwd: "/", SessionID: "s", Prompt: consultPrompt}
		if a := h.Check(in); a != nil {
			t.Errorf("excluded repo with origin %s should be silent, got %+v", origin, a)
		}
	}
}
