package hint

import (
	"encoding/json"
	"strings"
	"testing"
)

// The csl search response shape differs by output mode and arrives either as
// a JSON object or as a JSON string wrapping one. These cover both envelopes
// and both modes, since guessing wrong means the hint silently never fires.
func TestPathsFromResponse(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "content mode, object envelope",
			raw:  `{"lines":[{"path":"services/csl/a.go","repo":"mad01/thismoon","line":1},{"path":"services/csl/b.go","repo":"mad01/thismoon","line":9}],"output_mode":"content"}`,
			want: []string{"services/csl/a.go", "services/csl/b.go"},
		},
		{
			name: "string envelope wrapping the same object",
			raw:  `"{\"lines\":[{\"path\":\"services/keeper-of-facts/x.go\",\"repo\":\"mad01/thismoon\"}]}"`,
			want: []string{"services/keeper-of-facts/x.go"},
		},
		{
			name: "files_with_matches flat list",
			raw:  `{"files":[{"path":"tools/belt/main.go"},{"path":"tools/belt/other.go"}]}`,
			want: []string{"tools/belt/main.go", "tools/belt/other.go"},
		},
		{
			name: "duplicate paths collapse",
			raw:  `{"lines":[{"path":"a/b.go"},{"path":"a/b.go"}]}`,
			want: []string{"a/b.go"},
		},
		{
			name: "no paths at all",
			raw:  `{"total":0,"lines":[]}`,
			want: nil,
		},
		{
			name: "malformed response yields nothing rather than erroring",
			raw:  `not json at all`,
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PathsFromResponse(json.RawMessage(tc.raw))
			if len(got) != len(tc.want) {
				t.Fatalf("PathsFromResponse = %q, want %q", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("path %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestRepoFromResponsePicksTheMajority(t *testing.T) {
	raw := `{"lines":[{"repo":"mad01/thismoon"},{"repo":"mad01/thismoon"},{"repo":"mad01/other"}]}`
	if got := RepoFromResponse(json.RawMessage(raw)); got != "mad01/thismoon" {
		t.Errorf("RepoFromResponse = %q, want mad01/thismoon", got)
	}
	if got := RepoFromResponse(json.RawMessage(`{"lines":[]}`)); got != "" {
		t.Errorf("RepoFromResponse with no repos = %q, want empty", got)
	}
}

func TestPathsFromResponseRejectsProse(t *testing.T) {
	// "file" and "path" keys sometimes carry prose or a bare word; only
	// values that plausibly name a file should count.
	raw := `{"a":{"file":"a sentence with spaces"},"b":{"path":"real/file.go"}}`
	got := PathsFromResponse(json.RawMessage(raw))
	if len(got) != 1 || got[0] != "real/file.go" {
		t.Errorf("PathsFromResponse = %q, want just real/file.go", got)
	}
}

func TestRenderJoinsMultipleHints(t *testing.T) {
	out := Render([]Advice{{Hint: "a", Text: "first"}, {Hint: "b", Text: "second"}})
	if !strings.Contains(out, "belt[a]: first") || !strings.Contains(out, "belt[b]: second") {
		t.Errorf("Render = %q, want both hints attributed", out)
	}
	if Render(nil) != "" {
		t.Error("Render(nil) must be empty so the caller emits no JSON")
	}
}
