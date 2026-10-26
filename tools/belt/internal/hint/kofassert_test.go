package hint

import (
	"encoding/json"
	"maps"
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
			in: Input{
				Repo: "mad01/thismoon",
				Paths: []string{
					"services/csl/internal/semantic/index.go",
					"services/csl/internal/semantic/chunk.go",
				},
			},
			want: "repo:mad01/thismoon/services/csl/internal/semantic",
		},
		{
			name: "divergent paths fall back to the common prefix",
			in: Input{
				Repo:  "mad01/thismoon",
				Paths: []string{"services/csl/a.go", "services/keeper-of-facts/b.go"},
			},
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

// TestKofAssertionsDedupesBeforeTheCap pins the order of the cut: the
// session's seen set is applied to the ranked list first and the cap after,
// so a second search in the same area surfaces the next assertions in rank
// order instead of going quiet while they sit below the cut.
func TestKofAssertionsDedupesBeforeTheCap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var as []assertion
	for i := range 6 {
		as = append(as, assertion{
			ID:        string(rune('a' + i)),
			Subject:   "repo:mad01/thismoon/services/csl",
			Statement: "claim " + string(rune('a'+i)),
			Status:    "fresh",
			Pins:      []pin{{Repo: "github.com/mad01/thismoon", File: "services/csl/x.go"}},
		})
	}
	h := &KofAssertions{cfg: testConfig(), base: consultServer(t, as).URL}
	in := Input{
		Event:     EventSearch,
		Repo:      "mad01/thismoon",
		Paths:     []string{"services/csl/internal/semantic/a.go"},
		SessionID: "dedupe-session",
	}
	for _, want := range []string{"a,b,c", "d,e,f"} {
		got := h.Check(in)
		if got == nil {
			t.Fatalf("Check = nil, want assertions %s", want)
		}
		if ids := got.Tags["assertions"]; ids != want {
			t.Errorf("assertions tag = %q, want %q", ids, want)
		}
	}
	if got := h.Check(in); got != nil {
		t.Errorf("Check after every assertion was shown = %+v, want nil", got)
	}
}

func TestKofTags(t *testing.T) {
	as := []assertion{
		{ID: "1", Pins: []pin{
			{Repo: "github.com/mad01/thismoon", File: "tools/belt/a.go"},
			{Repo: "github.com/mad01/thismoon", File: "tools/belt/b.go"},
		}},
		{ID: "2", Pins: []pin{{Repo: "github.com/mad01/thismoon", File: "tools/belt/a.go"}}},
		{ID: "3"},
		{ID: "4", Pins: []pin{{File: "scratch/c.go"}}},
	}
	got := kofTags(as)
	want := map[string]string{
		"assertions": "1,2,3,4",
		"pins":       "github.com/mad01/thismoon/tools/belt/a.go,scratch/c.go",
	}
	if !maps.Equal(got, want) {
		t.Errorf("kofTags = %v, want %v (first pin only, deduplicated, repo-less pin bare)",
			got, want)
	}
	if _, ok := kofTags([]assertion{{ID: "1"}})["pins"]; ok {
		t.Error("kofTags set a pins tag for assertions with no pins")
	}
}

func TestQuerySilentWhenKeepIsDown(t *testing.T) {
	if got := queryKof("http://127.0.0.1:1", "repo:x/y"); got != nil {
		t.Errorf("query with kof down = %v, want nil", got)
	}
}

func TestRenderMarksStale(t *testing.T) {
	out := render("repo:x/y", []assertion{
		{
			ID:        "1",
			Statement: "fresh claim",
			Status:    "fresh",
			Pins:      []pin{{File: "a.go", StartLine: 1, EndLine: 5}},
		},
		{ID: "2", Statement: "moved claim", Status: "stale"},
	})
	if !strings.Contains(out, "[stale] moved claim") {
		t.Error("stale assertions must be surfaced with their status visible")
	}
	if !strings.Contains(out, "a.go:1-5") {
		t.Error("pin location should be rendered so the claim can be checked")
	}
}

func TestSeenTakeSuppressesRepeats(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	as := []assertion{{ID: "a"}, {ID: "b"}}

	first := seen.take("session-1", as, 5)
	if len(first) != 2 {
		t.Fatalf("first call returned %d, want 2", len(first))
	}
	if second := seen.take("session-1", as, 5); len(second) != 0 {
		t.Errorf("second call returned %d, want 0 (already surfaced)", len(second))
	}
	if other := seen.take("session-2", as, 5); len(other) != 2 {
		t.Errorf("a different session returned %d, want 2", len(other))
	}
}

// TestSeenTakeRecordsOnlyWhatItKept: an assertion cut by the limit was
// never shown, so it must stay unseen for the next fire.
func TestSeenTakeRecordsOnlyWhatItKept(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	as := []assertion{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	if got := seen.take("s", as, 2); len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("first take = %+v, want a and b", got)
	}
	if got := seen.take("s", as, 2); len(got) != 1 || got[0].ID != "c" {
		t.Errorf("second take = %+v, want only c", got)
	}
}

func TestSeenTakeWithoutSessionID(t *testing.T) {
	as := []assertion{{ID: "a"}, {ID: "b"}}
	if got := seen.take("", as, 5); len(got) != 2 {
		t.Error("with no session id, dedupe is impossible and advice should still flow")
	}
	if got := seen.take("", as, 1); len(got) != 1 {
		t.Error("with no session id the cap still applies")
	}
}

// TestKofAssertionsRepoExcluded pins that an excluded repo is silent before
// any kof query happens.
func TestKofAssertionsRepoExcluded(t *testing.T) {
	h := NewKofAssertions(config.Config{
		Hints: map[string]config.Toggle{
			"kof-assertions": {ExcludeRepos: []string{"github.com/mad01/thismoon"}},
		},
	})
	in := Input{
		Event:     EventSearch,
		Repo:      "mad01/thismoon",
		Paths:     []string{"services/csl/a.go"},
		SessionID: "s",
	}
	if a := h.Check(in); a != nil {
		t.Errorf("excluded repo should be silent, got %+v", a)
	}
}
