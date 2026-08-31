package hint

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func testConfig() config.Config { return config.Config{} }

func TestSubjectForRequiresDepth(t *testing.T) {
	tests := []struct {
		name  string
		in    Input
		want  string
		empty bool
	}{
		{
			name: "shared directory becomes the subject",
			in:   Input{Repo: "mad01/thismoon", Paths: []string{"services/csl/internal/semantic/index.go", "services/csl/internal/semantic/chunk.go"}},
			want: "repo:mad01/thismoon/services/csl/internal/semantic",
		},
		{
			name: "divergent paths fall back to the common prefix",
			in:   Input{Repo: "mad01/thismoon", Paths: []string{"services/csl/a.go", "services/keeper-of-facts/b.go"}},
			want: "repo:mad01/thismoon/services",
		},
		{
			// A repo-root subject would match every assertion in a monorepo.
			name:  "repo-root hits are too broad to advise on",
			in:    Input{Repo: "mad01/thismoon", Paths: []string{"README.md", "Makefile"}},
			empty: true,
		},
		{
			name:  "no repo means no subject",
			in:    Input{Paths: []string{"services/csl/a.go"}},
			empty: true,
		},
		{
			name:  "no hits means no subject",
			in:    Input{Repo: "mad01/thismoon"},
			empty: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := subjectFor(tc.in)
			if tc.empty {
				if got != "" {
					t.Errorf("subjectFor = %q, want empty", got)
				}
				return
			}
			if got != tc.want {
				t.Errorf("subjectFor = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestQueryDropsRetracted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"assertions": []assertion{
			{ID: "1", Subject: "repo:x/y/a", Statement: "one", Status: "fresh"},
			{ID: "2", Subject: "repo:x/y/a", Statement: "gone", Status: "retracted"},
		}})
	}))
	defer srv.Close()

	got := queryKof(srv.URL, "repo:x/y/")
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("query = %+v, want only the non-retracted assertion", got)
	}
}

// The bug this pins: assertions are labelled at the component level while
// searches return hits deeper inside it, and kof matches subjects by prefix.
// Querying the hit's own subject finds nothing, so the query has to widen to
// the repo and narrow again by ranking.
func TestRepoPrefixWidensToTheRepo(t *testing.T) {
	got := repoPrefix("repo:mad01/thismoon/services/csl/internal/semantic")
	if got != "repo:mad01/thismoon/" {
		t.Errorf("repoPrefix = %q, want repo:mad01/thismoon/", got)
	}
}

func TestRankPrefersCloserSubjects(t *testing.T) {
	hit := "repo:mad01/thismoon/services/csl/internal/semantic"
	as := []assertion{
		{ID: "far", Subject: "repo:mad01/thismoon/services/keeper-of-facts"},
		{ID: "near", Subject: "repo:mad01/thismoon/services/csl"},
		{ID: "exact", Subject: "repo:mad01/thismoon/services/csl/internal/semantic"},
		{ID: "root", Subject: "repo:mad01/thismoon"},
	}
	got := rank(hit, as)
	if len(got) == 0 {
		t.Fatal("rank dropped everything; the component-level assertion should survive")
	}
	if got[0].ID != "exact" {
		t.Errorf("rank put %q first, want the exact-subject assertion", got[0].ID)
	}
	for _, a := range got {
		if a.ID == "far" {
			t.Error("an assertion sharing only the generic 'services' segment should be dropped")
		}
		if a.ID == "root" {
			t.Error("a repo-root assertion has no location overlap and should be dropped")
		}
	}
}

func TestRankCapsOutput(t *testing.T) {
	hit := "repo:x/y/a/b"
	var as []assertion
	for i := range 6 {
		as = append(as, assertion{ID: string(rune('a' + i)), Subject: "repo:x/y/a/b"})
	}
	if got := rank(hit, as); len(got) != maxAssertions {
		t.Errorf("rank returned %d, want the cap of %d", len(got), maxAssertions)
	}
}

func TestQuerySilentWhenKeepIsDown(t *testing.T) {
	if got := queryKof("http://127.0.0.1:1", "repo:x/y"); got != nil {
		t.Errorf("query with kof down = %v, want nil", got)
	}
}

func TestRenderMarksStale(t *testing.T) {
	out := render("repo:x/y", []assertion{
		{ID: "1", Statement: "fresh claim", Status: "fresh", Pins: []struct {
			File      string `json:"file"`
			StartLine int    `json:"start_line"`
			EndLine   int    `json:"end_line"`
		}{{File: "a.go", StartLine: 1, EndLine: 5}}},
		{ID: "2", Statement: "moved claim", Status: "stale"},
	})
	if !strings.Contains(out, "[stale] moved claim") {
		t.Error("stale assertions must be surfaced with their status visible")
	}
	if !strings.Contains(out, "a.go:1-5") {
		t.Error("pin location should be rendered so the claim can be checked")
	}
}

func TestSeenFilterSuppressesRepeats(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	as := []assertion{{ID: "a"}, {ID: "b"}}

	first := seen.filter("session-1", as)
	if len(first) != 2 {
		t.Fatalf("first call returned %d, want 2", len(first))
	}
	if second := seen.filter("session-1", as); len(second) != 0 {
		t.Errorf("second call returned %d, want 0 (already surfaced)", len(second))
	}
	if other := seen.filter("session-2", as); len(other) != 2 {
		t.Errorf("a different session returned %d, want 2", len(other))
	}
}

func TestSeenFilterWithoutSessionID(t *testing.T) {
	as := []assertion{{ID: "a"}}
	if got := seen.filter("", as); len(got) != 1 {
		t.Error("with no session id, dedupe is impossible and advice should still flow")
	}
}

// TestKofAssertionsRepoExcluded pins that an excluded repo is silent before
// any kof query happens.
func TestKofAssertionsRepoExcluded(t *testing.T) {
	h := NewKofAssertions(config.Config{
		Hints: map[string]config.Toggle{"kof-assertions": {ExcludeRepos: []string{"github.com/mad01/thismoon"}}},
	})
	in := Input{Event: EventSearch, Repo: "mad01/thismoon", Paths: []string{"services/csl/a.go"}, SessionID: "s"}
	if a := h.Check(in); a != nil {
		t.Errorf("excluded repo should be silent, got %+v", a)
	}
}
