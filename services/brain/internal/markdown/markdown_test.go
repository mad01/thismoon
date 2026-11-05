package markdown

import (
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantTitle  string
		wantTags   []string
		wantStatus string
		wantBody   string
	}{
		{
			name: "full frontmatter",
			content: `---
title: "My Note"
tags: [ai, research]
status: validated
---
Body line one.
Body line two.
`,
			wantTitle:  "My Note",
			wantTags:   []string{"ai", "research"},
			wantStatus: "validated",
			wantBody:   "Body line one.\nBody line two.\n",
		},
		{
			name:     "no frontmatter",
			content:  "Just body.\n",
			wantBody: "Just body.\n",
		},
		{
			name: "empty body",
			content: `---
title: "Nothing"
---
`,
			wantTitle: "Nothing",
			wantBody:  "",
		},
		{
			name:     "unterminated fence treated as body",
			content:  "---\nno end here\n",
			wantBody: "---\nno end here\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm, body, err := ParseFrontmatter([]byte(tc.content))
			if err != nil {
				t.Fatalf("ParseFrontmatter: %v", err)
			}
			if fm.Title != tc.wantTitle {
				t.Errorf("Title = %q, want %q", fm.Title, tc.wantTitle)
			}
			if fm.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", fm.Status, tc.wantStatus)
			}
			if len(fm.Tags) != len(tc.wantTags) {
				t.Errorf("Tags = %v, want %v", fm.Tags, tc.wantTags)
			}
			if string(body) != tc.wantBody {
				t.Errorf("body = %q, want %q", string(body), tc.wantBody)
			}
		})
	}
}

func TestEvidenceOf(t *testing.T) {
	tests := []struct {
		name string
		fm   Frontmatter
		want string
	}{
		{
			name: "explicit field wins",
			fm:   Frontmatter{Evidence: "A", Tags: []string{"topic/x", "evidence/C"}},
			want: "A",
		},
		{
			name: "field only",
			fm:   Frontmatter{Evidence: "B"},
			want: "B",
		},
		{
			name: "tag fallback when field empty",
			fm:   Frontmatter{Tags: []string{"topic/x", "evidence/C"}},
			want: "C",
		},
		{
			name: "first evidence tag wins when multiple",
			fm:   Frontmatter{Tags: []string{"evidence/D", "evidence/A"}},
			want: "D",
		},
		{
			name: "neither set",
			fm:   Frontmatter{Tags: []string{"topic/x"}},
			want: "",
		},
		{
			name: "empty frontmatter",
			fm:   Frontmatter{},
			want: "",
		},
		{
			name: "evidence/mixed tag",
			fm:   Frontmatter{Tags: []string{"evidence/mixed"}},
			want: "mixed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvidenceOf(tc.fm); got != tc.want {
				t.Errorf("EvidenceOf = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEvidenceRoundTrip(t *testing.T) {
	// Write a frontmatter with evidence field → render → re-parse → still set.
	original := Frontmatter{
		Title:    "Test",
		Tags:     []string{"topic/a"},
		Evidence: "B",
		Created:  "2026-04-14",
		Updated:  "2026-04-14",
	}
	data, err := Render(original, []byte("body\n"))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(data), "evidence: B") {
		t.Errorf("rendered frontmatter missing 'evidence: B'; got:\n%s", data)
	}
	parsed, _, err := ParseFrontmatter(data)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if parsed.Evidence != "B" {
		t.Errorf("round-trip Evidence = %q, want B", parsed.Evidence)
	}
}

func TestExtractWikilinks(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []Wikilink
	}{
		{
			name:    "simple",
			content: "See [[Foo]] for details.\n",
			want:    []Wikilink{{Target: "Foo", Raw: "[[Foo]]", Line: 1, Column: 5}},
		},
		{
			name:    "aliased",
			content: "Refer to [[Some Topic|the topic]].\n",
			want: []Wikilink{
				{
					Target: "Some Topic",
					Alias:  "the topic",
					Raw:    "[[Some Topic|the topic]]",
					Line:   1,
					Column: 10,
				},
			},
		},
		{
			name:    "with heading",
			content: "[[Note#Section]]\n",
			want: []Wikilink{
				{Target: "Note", Heading: "Section", Raw: "[[Note#Section]]", Line: 1, Column: 1},
			},
		},
		{
			name:    "inside code span is ignored",
			content: "use `[[NotALink]]` inline\nbut [[Real]] counts.\n",
			want:    []Wikilink{{Target: "Real", Raw: "[[Real]]", Line: 2, Column: 5}},
		},
		{
			name:    "inside fenced code is ignored",
			content: "```\n[[NotALink]]\n```\n[[Real]]\n",
			want:    []Wikilink{{Target: "Real", Raw: "[[Real]]", Line: 4, Column: 1}},
		},
		{
			name:    "none",
			content: "No wikilinks here.\n",
			want:    nil,
		},
		{
			name:    "multiple on a line",
			content: "A [[one]] and [[two|2]] on one line.\n",
			want: []Wikilink{
				{Target: "one", Raw: "[[one]]", Line: 1, Column: 3},
				{Target: "two", Alias: "2", Raw: "[[two|2]]", Line: 1, Column: 15},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractWikilinks([]byte(tc.content))
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d: got %#v", len(got), len(tc.want), got)
			}
			for i, w := range tc.want {
				g := got[i]
				if g.Target != w.Target || g.Alias != w.Alias || g.Heading != w.Heading ||
					g.Raw != w.Raw ||
					g.Line != w.Line ||
					g.Column != w.Column {
					t.Errorf("wikilink[%d] = %+v, want %+v", i, g, w)
				}
			}
		})
	}
}

func TestExtractMDLinks(t *testing.T) {
	content := "See [docs](https://example.com/x).\n"
	got := ExtractMDLinks([]byte(content))
	if len(got) != 1 {
		t.Fatalf("want 1 link, got %d", len(got))
	}
	if got[0].Text != "docs" || got[0].URL != "https://example.com/x" {
		t.Errorf("got %+v", got[0])
	}
}

func TestRenderRoundtrip(t *testing.T) {
	fm := Frontmatter{Title: "Hi", Tags: []string{"a"}, Status: "raw"}
	body := []byte("body text\n")
	out, err := Render(fm, body)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got, gotBody, err := ParseFrontmatter(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if got.Title != "Hi" || got.Status != "raw" || len(got.Tags) != 1 || got.Tags[0] != "a" {
		t.Errorf("roundtrip frontmatter lost fields: %+v", got)
	}
	if string(gotBody) != "body text\n" {
		t.Errorf("roundtrip body mismatch: %q", gotBody)
	}
	if !strings.HasPrefix(string(out), "---\n") {
		t.Errorf("render missing opening fence: %q", out)
	}
}
