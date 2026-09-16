package mcpserver

import (
	"strings"
	"testing"
)

// wantLines compares a rendered string against the expected lines and checks
// the shared contract: no trailing whitespace on any line, no trailing newline.
func wantLines(t *testing.T, got string, want ...string) {
	t.Helper()
	if w := strings.Join(want, "\n"); got != w {
		t.Errorf("rendered text differs\n--- got ---\n%s\n--- want ---\n%s", got, w)
	}
	if strings.HasSuffix(got, "\n") {
		t.Error("rendered text ends with a newline")
	}
	for i, line := range strings.Split(got, "\n") {
		if strings.TrimRight(line, " \t") != line {
			t.Errorf("line %d has trailing whitespace: %q", i+1, line)
		}
	}
}

func TestRenderSearchText_Files(t *testing.T) {
	out := buildSearchOutput(filesOutputMode, 50, 0, makeFileMatches(3, "org/repo"))
	wantLines(
		t, renderSearchText(out),
		"org/repo/pkg0/file0.go",
		"org/repo/pkg0/file1.go",
		"org/repo/pkg0/file2.go",
		"",
		"3 files",
	)
}

func TestRenderSearchText_FilesTruncatedNamesNextOffset(t *testing.T) {
	out := buildSearchOutput(filesOutputMode, 50, 10, makeFileMatches(60, "org/repo"))
	got := strings.Split(renderSearchText(out), "\n")
	tail := got[len(got)-2:]
	wantLines(
		t, strings.Join(tail, "\n"),
		"50 files",
		"truncated: showing 50 of 60 files; next offset=60",
	)
	if len(got) != 53 {
		t.Errorf("rendered %d lines, want 50 paths + blank + 2 trailer lines", len(got))
	}
}

func TestRenderSearchText_ContentMergesContextAndGroups(t *testing.T) {
	out := searchOutput{
		OutputMode: contentOutputMode,
		Total:      4,
		Lines: []searchMatchLine{
			{
				Repo:   "org/repo",
				Path:   "a.go",
				Line:   5,
				Text:   "five",
				Before: "three\nfour\n",
				After:  "six\nseven\n",
			},
			{
				Repo:   "org/repo",
				Path:   "a.go",
				Line:   7,
				Text:   "seven",
				Before: "five\nsix\n",
				After:  "eight\nnine",
			},
			{Repo: "org/repo", Path: "a.go", Line: 20, Text: "twenty"},
			{Repo: "org/repo", Path: "b.go", Line: 1, Text: "\tindented"},
		},
	}
	wantLines(
		t, renderSearchText(out),
		"org/repo/a.go",
		"3-three",
		"4-four",
		"5:five",
		"6-six",
		"7:seven",
		"8-eight",
		"9-nine",
		"--",
		"20:twenty",
		"",
		"org/repo/b.go",
		"1:\tindented",
		"",
		"4 lines in 2 files",
	)
}

func TestRenderSearchText_ContentKeepsFileOrderOfFirstAppearance(t *testing.T) {
	out := searchOutput{
		OutputMode: contentOutputMode,
		Total:      3,
		Lines: []searchMatchLine{
			{Repo: "org/repo", Path: "z.go", Line: 2, Text: "z2"},
			{Repo: "org/repo", Path: "a.go", Line: 9, Text: "a9"},
			{Repo: "org/repo", Path: "z.go", Line: 1, Text: "z1"},
		},
	}
	wantLines(
		t, renderSearchText(out),
		"org/repo/z.go",
		"1:z1",
		"2:z2",
		"",
		"org/repo/a.go",
		"9:a9",
		"",
		"3 lines in 2 files",
	)
}

func TestRenderSearchText_ContentTruncatedRepeatsCutFile(t *testing.T) {
	matches := append(
		makeMatches(200, "org/repo", "a.go"),
		makeMatches(200, "org/repo", "b.go")...,
	)
	out := buildSearchOutput(contentOutputMode, 50, 10, matches)
	got := strings.Split(renderSearchText(out), "\n")
	last := got[len(got)-1]
	want := "truncated: showing 300 of 400 lines; next offset=11"
	if last != want {
		t.Fatalf(
			"trailer = %q, want %q (b.go was cut, so the next page must start at it)",
			last,
			want,
		)
	}
}

func TestRenderSearchText_ContentTruncated(t *testing.T) {
	out := buildSearchOutput(contentOutputMode, 50, 0, makeMatches(500, "org/repo", "main.go"))
	got := strings.Split(renderSearchText(out), "\n")
	if len(got) != 304 {
		t.Fatalf("rendered %d lines, want header + 300 + blank + 2 trailer lines", len(got))
	}
	wantLines(t, strings.Join(got[:2], "\n"), "org/repo/main.go", "1:match line")
	wantLines(
		t, strings.Join(got[301:], "\n"),
		"",
		"300 lines in 1 file",
		"truncated: showing 300 of 500 lines; next offset=1",
	)
	for _, line := range got[1:301] {
		if line == "--" {
			t.Fatal("contiguous lines must not be separated by --")
		}
	}
}

func TestRenderSearchText_ZeroResults(t *testing.T) {
	out := searchOutput{OutputMode: filesOutputMode, ZeroHint: &searchZeroHint{
		ParsedQuery:     `substr:"foo"`,
		ReposSearched:   2,
		ReposDiscovered: 5,
		ReposIndexed:    3,
		NewestIndexedAt: "2026-09-01T00:00:00Z",
		OldestIndexedAt: "2026-08-01T00:00:00Z",
		Notes:           []string{"first note", "second note"},
	}}
	wantLines(
		t, renderSearchText(out),
		"no matches",
		`parsed query: substr:"foo"`,
		"repos searched: 2 (3 indexed, 5 discovered)",
		"index age: newest 2026-09-01T00:00:00Z, oldest 2026-08-01T00:00:00Z",
		"- first note",
		"- second note",
	)
}

func TestRenderSearchText_ZeroResultsMinimalHint(t *testing.T) {
	wantLines(t, renderSearchText(searchOutput{OutputMode: filesOutputMode}), "no matches")

	out := searchOutput{
		OutputMode: contentOutputMode,
		ZeroHint:   &searchZeroHint{Notes: []string{"n"}},
	}
	wantLines(
		t, renderSearchText(out),
		"no matches",
		"repos searched: 0 (0 indexed, 0 discovered)",
		"- n",
	)
}

func TestRenderSemanticText(t *testing.T) {
	out := semanticSearchOutput{Available: true, Hits: []semanticHit{
		{
			Repo:      "org/repo",
			Path:      "a.go",
			StartLine: 10,
			EndLine:   20,
			Score:     0.834,
			Kind:      "func",
			Snippet:   "func A() {\n}\n",
		},
		{Repo: "org/repo", Path: "b.md", StartLine: 1, EndLine: 3, Score: 0.5},
	}}
	wantLines(
		t, renderSemanticText(out),
		"org/repo/a.go:10-20 score=0.83 kind=func",
		"func A() {",
		"}",
		"",
		"org/repo/b.md:1-3 score=0.50",
		"",
		"2 hits",
	)
}

func TestRenderSemanticText_Unavailable(t *testing.T) {
	wantLines(t, renderSemanticText(semanticSearchOutput{Note: "build it"}), "build it")
	wantLines(t, renderSemanticText(semanticSearchOutput{}), "semantic search unavailable")
	wantLines(t, renderSemanticText(semanticSearchOutput{Available: true}), "0 hits")
}

func TestRenderHybridText(t *testing.T) {
	out := hybridSearchOutput{SemanticAvailable: true, Hits: []hybridHit{
		{
			Repo: "org/repo", Path: "a.go", Score: 0.03125,
			LexRank: 3, LexLine: 71, LexText: "\tretry(req)",
			SemRank: 1, SemStart: 40, SemEnd: 60, SemScore: 0.834, Snippet: "func retry() {}\n",
		},
		{Repo: "org/repo", Path: "b.go", Score: 0.0161, LexRank: 1, LexLine: 5, LexText: "x"},
		{
			Repo:     "org/repo",
			Path:     "c.go",
			Score:    0.0159,
			SemRank:  2,
			SemStart: 1,
			SemEnd:   9,
			SemScore: 0.7,
		},
	}}
	wantLines(
		t, renderHybridText(out),
		"org/repo/a.go score=0.0312 lex=#3 L71 sem=#1 L40-60 0.83",
		"L71:\tretry(req)",
		"func retry() {}",
		"",
		"org/repo/b.go score=0.0161 lex=#1 L5",
		"L5:x",
		"",
		"org/repo/c.go score=0.0159 sem=#2 L1-9 0.70",
		"",
		"3 hits",
	)
}

func TestRenderHybridText_LexicalOnlyKeepsHits(t *testing.T) {
	out := hybridSearchOutput{Note: "build it", Hits: []hybridHit{
		{Repo: "org/repo", Path: "b.go", Score: 0.0161, LexRank: 1, LexLine: 5},
	}}
	wantLines(
		t, renderHybridText(out),
		"lexical-only results; build it",
		"org/repo/b.go score=0.0161 lex=#1 L5",
		"",
		"1 hit",
	)
}

func TestRenderReadText(t *testing.T) {
	out := readOutput{Repo: "org/repo", Path: "main.go", TotalLines: 100, Lines: []readLine{
		{Number: 50, Text: "a"},
		{Number: 51, Text: "\tb"},
		{Number: 52, Text: ""},
	}}
	wantLines(
		t, renderReadText(out),
		"org/repo/main.go lines 50-52 of 100",
		"50:a",
		"51:\tb",
		"52:",
	)

	out.Truncated = true
	got := renderReadText(out)
	if !strings.HasSuffix(got, "\ntruncated: read more with start_line/end_line") {
		t.Errorf("truncated read lacks the paging hint:\n%s", got)
	}

	empty := readOutput{Repo: "org/repo", Path: "main.go", TotalLines: 3}
	wantLines(t, renderReadText(empty), "org/repo/main.go no lines in range (3 lines)")
}

func TestRenderLsText(t *testing.T) {
	out := lsOutput{Repo: "org/repo", Path: ".", Total: 3, Entries: []lsEntry{
		{Path: "docs", Dir: true},
		{Path: "internal", Dir: true},
		{Path: "main.go", Bytes: 42},
	}}
	wantLines(
		t, renderLsText(out),
		"org/repo (3 entries)",
		"docs/",
		"internal/",
		"main.go  42",
	)

	sub := lsOutput{
		Repo:           "org/repo",
		Path:           "internal",
		Total:          1,
		Truncated:      true,
		TotalAvailable: 900,
		Entries:        []lsEntry{{Path: "internal/a.go", Bytes: 7}},
	}
	wantLines(
		t, renderLsText(sub),
		"org/repo/internal (1 entry)",
		"internal/a.go  7",
		"truncated: showing 1 of 900 entries; narrow with path/glob",
	)
}

func TestRenderCountText(t *testing.T) {
	wantLines(t, renderCountText(countOutput{Total: 7}), "total: 7")
	out := countOutput{
		Total:  7,
		Groups: []countGroup{{Group: "org/a", Count: 5}, {Group: "org/b", Count: 2}},
	}
	wantLines(
		t, renderCountText(out),
		"total: 7",
		"org/a  5",
		"org/b  2",
	)
}

func TestRenderDoctorText(t *testing.T) {
	out := doctorOutput{
		OK:   true,
		Note: "no config file at ~/.config/csl/config.yaml",
		Checks: []doctorCheck{
			{Name: "config", Status: "skipped", Detail: "nothing configured"},
			{Name: "index", Status: "ok"},
		},
	}
	wantLines(
		t, renderDoctorText(out),
		"ok: no config file at ~/.config/csl/config.yaml",
		"[skip] config: nothing configured",
		"[ok] index",
	)

	failed := doctorOutput{Checks: []doctorCheck{
		{Name: "shards", Status: "fail", Detail: "2 corrupt; run csl doctor --repair"},
		{Name: "odd", Status: "unknown"},
	}}
	wantLines(
		t, renderDoctorText(failed),
		"FAIL",
		"[FAIL] shards: 2 corrupt; run csl doctor --repair",
		"[unknown] odd",
	)
}

// TestRenderSearchText_ContentMarksSymbolHits: a sym: hit's line ends with
// the kind token the semantic renderer uses, plus the parent when nested;
// plain hits and context lines are untouched.
func TestRenderSearchText_ContentMarksSymbolHits(t *testing.T) {
	out := searchOutput{
		OutputMode: contentOutputMode,
		Total:      3,
		Lines: []searchMatchLine{
			{
				Repo:   "org/repo",
				Path:   "a.go",
				Line:   4,
				Text:   "func Hello() {",
				Kind:   "function",
				Before: "// doc\n",
			},
			{
				Repo:   "org/repo",
				Path:   "a.go",
				Line:   9,
				Text:   "func (p *Point) Hello() {",
				Kind:   "method",
				Parent: "Point",
			},
			{Repo: "org/repo", Path: "b.go", Line: 1, Text: "Hello()"},
		},
	}
	wantLines(
		t, renderSearchText(out),
		"org/repo/a.go",
		"3-// doc",
		"4:func Hello() {  kind=function",
		"--",
		"9:func (p *Point) Hello() {  kind=method parent=Point",
		"",
		"org/repo/b.go",
		"1:Hello()",
		"",
		"3 lines in 2 files",
	)
}
