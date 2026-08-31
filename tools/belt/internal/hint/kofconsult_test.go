package hint

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
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
		cfg:         testConfig(),
		base:        base,
		originURL:   func(string) string { return origin },
		resolveRepo: func(string) string { return "" },
	}
}

func TestKofConsultSurfacesRepoAssertions(t *testing.T) {
	srv := consultServer(t, []assertion{
		{
			ID:        "1",
			Subject:   "repo:mad01/thismoon/services/keeper-of-facts",
			Statement: "serve is the single writer",
			Status:    "fresh",
		},
		{
			ID:        "2",
			Subject:   "repo:mad01/thismoon-arcade/game",
			Statement: "sibling noise",
			Status:    "fresh",
		},
	})
	h := consultHint(srv.URL, "git@github.com:mad01/thismoon.git")
	got := h.Check(Input{Event: EventSessionStart, Cwd: t.TempDir()})
	if got == nil {
		t.Fatal("Check = nil, want advice for the repo's assertion")
	}
	if !strings.Contains(got.Text, "serve is the single writer") {
		t.Errorf("advice %q does not carry the assertion statement", got.Text)
	}
	if strings.Contains(got.Text, "sibling noise") {
		t.Errorf("advice %q leaked a sibling repo's assertion", got.Text)
	}
}

func TestKofConsultCapsAssertions(t *testing.T) {
	var as []assertion
	for i := range 8 {
		as = append(as, assertion{
			ID:        string(rune('a' + i)),
			Subject:   "repo:mad01/thismoon/services/keeper-of-facts",
			Statement: "claim",
			Status:    "fresh",
		})
	}
	h := consultHint(consultServer(t, as).URL, "git@github.com:mad01/thismoon.git")
	got := h.Check(Input{Event: EventSessionStart, Cwd: t.TempDir()})
	if got == nil {
		t.Fatal("Check = nil, want capped advice")
	}
	if want := "5 stored assertion(s)"; !strings.Contains(got.Text, want) {
		t.Errorf("advice %q does not contain %q", got.Text, want)
	}
}

func TestKofConsultSilentOutsideARepo(t *testing.T) {
	h := consultHint("http://127.0.0.1:1", "")
	if got := h.Check(Input{Event: EventSessionStart, Cwd: t.TempDir()}); got != nil {
		t.Errorf("Check outside a repo = %+v, want nil", got)
	}
}

func TestKofConsultSilentWhenKeepIsDown(t *testing.T) {
	h := consultHint("http://127.0.0.1:1", "git@github.com:mad01/thismoon.git")
	if got := h.Check(Input{Event: EventSessionStart, Cwd: t.TempDir()}); got != nil {
		t.Errorf("Check with kof down = %+v, want nil", got)
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
	if got := gitOriginURL(dir); got != "git@github.com:mad01/thismoon.git" {
		t.Errorf("gitOriginURL = %q, want the configured origin", got)
	}
	if got := gitOriginURL(t.TempDir()); got != "" {
		t.Errorf("gitOriginURL outside a repo = %q, want empty", got)
	}
}

// TestKofConsultRepoExcluded pins that an excluded repo is silent before any
// kof query happens — no server is stubbed here, so reaching the query would
// hang or error rather than pass. Exclusion matches the canonical identity,
// not the org/name kof subjects use.
func TestKofConsultRepoExcluded(t *testing.T) {
	h := NewKofConsult(config.Config{
		Hints: map[string]config.Toggle{"kof-consult": {ExcludeRepos: []string{"github.com/mad01/thismoon"}}},
	})
	h.originURL = func(string) string { return "git@github.com:mad01/thismoon.git" }
	h.resolveRepo = func(string) string { return "github.com/mad01/thismoon" }
	if a := h.Check(Input{Event: EventSessionStart, Cwd: "/", SessionID: "s"}); a != nil {
		t.Errorf("excluded repo should be silent, got %+v", a)
	}
}
