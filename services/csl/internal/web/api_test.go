package web

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

func TestGroupMatches(t *testing.T) {
	repos := map[string]finder.Repo{
		"mad01/thismoon": {Name: "mad01/thismoon", Host: "github.com"},
		"acme/widget":             {Name: "acme/widget", Host: "git.example.com"},
	}
	matches := []search.Match{
		{Repo: "mad01/thismoon", File: "a.go", Line: 10, Text: "x"},
		{Repo: "mad01/thismoon", File: "a.go", Line: 20, Text: "y"},
		{Repo: "mad01/thismoon", File: "b.go", Line: 1, Text: "z"},
		{Repo: "acme/widget", File: "main.go", Line: 5, Text: "w"},
	}

	groups := groupMatches(matches, repos)

	if len(groups) != 2 {
		t.Fatalf("want 2 repo groups, got %d", len(groups))
	}
	// First-appearance order preserved.
	if groups[0].Repo != "mad01/thismoon" || groups[1].Repo != "acme/widget" {
		t.Fatalf("repo order not preserved: %q, %q", groups[0].Repo, groups[1].Repo)
	}
	if groups[0].Host != "github.com" {
		t.Errorf("want host github.com, got %q", groups[0].Host)
	}
	if len(groups[0].Files) != 2 {
		t.Fatalf("want 2 files in first repo, got %d", len(groups[0].Files))
	}
	if got := groups[0].Files[0].File; got != "a.go" {
		t.Errorf("want first file a.go, got %q", got)
	}
	if n := len(groups[0].Files[0].Matches); n != 2 {
		t.Errorf("want 2 matches in a.go, got %d", n)
	}
	wantURL := "https://github.com/mad01/thismoon/blob/HEAD/a.go#L10"
	if got := groups[0].Files[0].Matches[0].RemoteURL; got != wantURL {
		t.Errorf("match remoteURL = %q, want %q", got, wantURL)
	}
	wantFileURL := "https://github.com/mad01/thismoon/blob/HEAD/a.go"
	if got := groups[0].Files[0].FileURL; got != wantFileURL {
		t.Errorf("fileURL = %q, want %q", got, wantFileURL)
	}
}

func TestGroupMatchesLocalPath(t *testing.T) {
	repos := map[string]finder.Repo{
		"mad01/thismoon": {Name: "mad01/thismoon", Path: "/home/u/code/csl", Host: "github.com"},
		"acme/widget":             {Name: "acme/widget" /* no Path */, Host: "git.example.com"},
	}
	matches := []search.Match{
		{Repo: "mad01/thismoon", File: "internal/web/api.go", Line: 1},
		{Repo: "acme/widget", File: "main.go", Line: 1},
	}

	groups := groupMatches(matches, repos)

	if got, want := groups[0].Files[0].LocalPath, "/home/u/code/csl/internal/web/api.go"; got != want {
		t.Errorf("LocalPath = %q, want %q", got, want)
	}
	if got := groups[1].Files[0].LocalPath; got != "" {
		t.Errorf("LocalPath for repo without root = %q, want empty", got)
	}
}

func TestCollapseHome(t *testing.T) {
	tests := []struct {
		name, p, home, want string
	}{
		{"under home", "/home/u/code/x.go", "/home/u", "~/code/x.go"},
		{"equals home", "/home/u", "/home/u", "~"},
		{"outside home", "/etc/hosts", "/home/u", "/etc/hosts"},
		{"prefix but not boundary", "/home/usr2/x", "/home/u", "/home/usr2/x"},
		{"empty home", "/home/u/x", "", "/home/u/x"},
		{"empty path", "", "/home/u", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := collapseHome(tt.p, tt.home); got != tt.want {
				t.Errorf("collapseHome(%q, %q) = %q, want %q", tt.p, tt.home, got, tt.want)
			}
		})
	}
}

func TestGroupMatchesEmpty(t *testing.T) {
	if g := groupMatches(nil, map[string]finder.Repo{}); len(g) != 0 {
		t.Errorf("want empty, got %d groups", len(g))
	}
}

// matchesAcrossFiles builds one match in each of n distinct files in one repo.
func matchesAcrossFiles(repo string, n int) []search.Match {
	out := make([]search.Match, n)
	for i := range n {
		out[i] = search.Match{
			Repo: repo,
			File: "f" + string(rune('a'+i%26)) + string(rune('0'+i/26)) + ".go",
			Line: i + 1,
			Text: "x",
		}
	}
	return out
}

func TestBuildSearchResponse(t *testing.T) {
	repoMap := map[string]finder.Repo{
		"mad01/thismoon": {Name: "mad01/thismoon", Host: "github.com"},
	}

	t.Run("counts and metadata", func(t *testing.T) {
		matches := []search.Match{
			{Repo: "mad01/thismoon", File: "a.go", Line: 1, Text: "x"},
			{Repo: "mad01/thismoon", File: "a.go", Line: 2, Text: "y"},
			{Repo: "mad01/thismoon", File: "b.go", Line: 1, Text: "z"},
		}
		resp := buildSearchResponse("foo", "files_with_matches", 50, matches, repoMap)
		if resp.Total != 3 {
			t.Errorf("Total = %d, want 3", resp.Total)
		}
		if resp.Files != 2 {
			t.Errorf("Files = %d, want 2", resp.Files)
		}
		if resp.Limit != 50 {
			t.Errorf("Limit = %d, want 50", resp.Limit)
		}
		if resp.Query != "foo" || resp.Mode != "files_with_matches" {
			t.Errorf("query/mode not echoed: %q %q", resp.Query, resp.Mode)
		}
	})

	t.Run("not truncated when files below limit", func(t *testing.T) {
		resp := buildSearchResponse(
			"q",
			"files_with_matches",
			50,
			matchesAcrossFiles("mad01/thismoon", 10),
			repoMap,
		)
		if resp.Files != 10 {
			t.Fatalf("Files = %d, want 10", resp.Files)
		}
		if resp.Truncated {
			t.Errorf("Truncated = true, want false (10 files < limit 50)")
		}
	})

	t.Run("truncated when files reach limit", func(t *testing.T) {
		resp := buildSearchResponse(
			"q",
			"files_with_matches",
			5,
			matchesAcrossFiles("mad01/thismoon", 5),
			repoMap,
		)
		if resp.Files != 5 {
			t.Fatalf("Files = %d, want 5", resp.Files)
		}
		if !resp.Truncated {
			t.Errorf("Truncated = false, want true (5 files == limit 5)")
		}
	})

	t.Run("empty result is not truncated", func(t *testing.T) {
		resp := buildSearchResponse("q", "files_with_matches", 50, nil, repoMap)
		if resp.Total != 0 || resp.Files != 0 {
			t.Errorf("want empty counts, got total=%d files=%d", resp.Total, resp.Files)
		}
		if resp.Truncated {
			t.Errorf("Truncated = true, want false for empty result")
		}
		if resp.Repos == nil {
			t.Errorf("Repos is nil; want empty slice so it marshals as [] not null")
		}
	})

	t.Run("marshals expected JSON keys", func(t *testing.T) {
		resp := buildSearchResponse("q", "files_with_matches", 50, nil, repoMap)
		b, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, key := range []string{`"total":`, `"files":`, `"limit":`, `"truncated":`, `"repos":[]`} {
			if !strings.Contains(s, key) {
				t.Errorf("JSON missing %s in %s", key, s)
			}
		}
	})
}

func TestExtensionFacets(t *testing.T) {
	t.Run("counts files and matches per extension", func(t *testing.T) {
		matches := []search.Match{
			{Repo: "r", File: "a.go", Line: 1},
			{Repo: "r", File: "a.go", Line: 2},
			{Repo: "r", File: "b.go", Line: 1},
			{Repo: "r", File: "README.md", Line: 1},
		}
		facets := extensionFacets(matches)
		if len(facets) != 2 {
			t.Fatalf("want 2 facets, got %d: %+v", len(facets), facets)
		}
		// Sorted by file count descending.
		if facets[0].Ext != ".go" || facets[0].Files != 2 || facets[0].Matches != 3 {
			t.Errorf("facet[0] = %+v, want ext=.go files=2 matches=3", facets[0])
		}
		if facets[0].Label != "Go" {
			t.Errorf("facet[0].Label = %q, want Go", facets[0].Label)
		}
		if facets[1].Ext != ".md" || facets[1].Files != 1 || facets[1].Matches != 1 {
			t.Errorf("facet[1] = %+v, want ext=.md files=1 matches=1", facets[1])
		}
	})

	t.Run("same file counted once across repos", func(t *testing.T) {
		matches := []search.Match{
			{Repo: "r1", File: "main.go", Line: 1},
			{Repo: "r2", File: "main.go", Line: 1},
		}
		facets := extensionFacets(matches)
		if len(facets) != 1 || facets[0].Files != 2 {
			t.Fatalf("want 1 facet with 2 files (distinct repos), got %+v", facets)
		}
	})

	t.Run("extension is lowercased", func(t *testing.T) {
		facets := extensionFacets([]search.Match{{Repo: "r", File: "A.GO", Line: 1}})
		if len(facets) != 1 || facets[0].Ext != ".go" {
			t.Fatalf("want lowercased .go, got %+v", facets)
		}
	})

	t.Run("unknown extension uses the suffix as label", func(t *testing.T) {
		facets := extensionFacets([]search.Match{{Repo: "r", File: "x.zig", Line: 1}})
		if len(facets) != 1 || facets[0].Label != ".zig" {
			t.Fatalf("want label .zig, got %+v", facets)
		}
	})

	t.Run("files without extension grouped under empty ext", func(t *testing.T) {
		facets := extensionFacets([]search.Match{{Repo: "r", File: "Makefile", Line: 1}})
		if len(facets) != 1 || facets[0].Ext != "" || facets[0].Label != "no ext" {
			t.Fatalf("want ext=\"\" label=\"no ext\", got %+v", facets)
		}
	})

	t.Run("ties broken by extension for determinism", func(t *testing.T) {
		matches := []search.Match{
			{Repo: "r", File: "a.md", Line: 1},
			{Repo: "r", File: "b.go", Line: 1},
		}
		facets := extensionFacets(matches)
		if facets[0].Ext != ".go" || facets[1].Ext != ".md" {
			t.Fatalf("tie not broken by ext asc: %+v", facets)
		}
	})

	t.Run("empty input yields empty slice", func(t *testing.T) {
		if f := extensionFacets(nil); f == nil || len(f) != 0 {
			t.Fatalf("want non-nil empty slice, got %#v", f)
		}
	})
}

func TestBuildSearchResponseFacets(t *testing.T) {
	repoMap := map[string]finder.Repo{"r": {Name: "r"}}
	matches := []search.Match{
		{Repo: "r", File: "a.go", Line: 1, Text: "x"},
		{Repo: "r", File: "doc.md", Line: 1, Text: "y"},
	}
	resp := buildSearchResponse("q", "files_with_matches", 50, matches, repoMap)
	if len(resp.Facets) != 2 {
		t.Fatalf("want 2 facets in response, got %+v", resp.Facets)
	}

	b, err := json.Marshal(buildSearchResponse("q", "files_with_matches", 50, nil, repoMap))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"facets":[]`) {
		t.Errorf("empty response JSON missing \"facets\":[] — got %s", b)
	}
}

func TestClampInt(t *testing.T) {
	tests := []struct {
		in            string
		def, min, max int
		want          int
	}{
		{"", 50, 1, 500, 50},
		{"abc", 50, 1, 500, 50},
		{"100", 50, 1, 500, 100},
		{"0", 50, 1, 500, 1},
		{"9999", 50, 1, 500, 500},
		{"-5", 0, 0, 20, 0},
	}
	for _, tt := range tests {
		if got := clampInt(tt.in, tt.def, tt.min, tt.max); got != tt.want {
			t.Errorf(
				"clampInt(%q, %d, %d, %d) = %d, want %d",
				tt.in,
				tt.def,
				tt.min,
				tt.max,
				got,
				tt.want,
			)
		}
	}
}
