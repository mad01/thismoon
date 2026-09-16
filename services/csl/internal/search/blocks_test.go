package search

import (
	"strings"
	"testing"
)

// wantRendered compares a file's rendered lines against the expected ones.
func wantRendered(t *testing.T, got []string, want ...string) {
	t.Helper()
	if g, w := strings.Join(got, "\n"), strings.Join(want, "\n"); g != w {
		t.Errorf("rendered text differs\n--- got ---\n%s\n--- want ---\n%s", g, w)
	}
}

// TestBlocks_NearbyHitsShareOneWindow: two hits whose context windows
// overlap become one block in which every line prints once, the shared line
// prints as a match, and the match's text wins over the context copy.
func TestBlocks_NearbyHitsShareOneWindow(t *testing.T) {
	files := Blocks([]Match{
		{
			Repo: "org/repo", File: "a.go", Line: 5, Text: "five",
			Before: "three\nfour\n", After: "six\nseven\n",
		},
		{
			Repo: "org/repo", File: "a.go", Line: 7, Text: "seven",
			Before: "five\nsix\n", After: "eight\nnine",
		},
	})
	if len(files) != 1 || len(files[0].Blocks) != 1 {
		t.Fatalf("got %d files / %d blocks, want 1 / 1", len(files), len(files[0].Blocks))
	}
	wantRendered(
		t, files[0].Render(),
		"org/repo/a.go",
		"3-three",
		"4-four",
		"5:five",
		"6-six",
		"7:seven",
		"8-eight",
		"9-nine",
	)
}

// TestBlocks_FarHitsSplitWithSeparator: hits whose windows do not touch stay
// separate blocks, rendered with `--` between them under one header.
func TestBlocks_FarHitsSplitWithSeparator(t *testing.T) {
	files := Blocks([]Match{
		{Repo: "org/repo", File: "a.go", Line: 2, Text: "two", After: "three\n"},
		{Repo: "org/repo", File: "a.go", Line: 20, Text: "twenty", Before: "nineteen\n"},
	})
	if len(files) != 1 || len(files[0].Blocks) != 2 {
		t.Fatalf("got %d files / %d blocks, want 1 / 2", len(files), len(files[0].Blocks))
	}
	wantRendered(
		t, files[0].Render(),
		"org/repo/a.go",
		"2:two",
		"3-three",
		"--",
		"19-nineteen",
		"20:twenty",
	)
}

// TestBlocks_AdjacentWindowsMerge: windows that end and start on consecutive
// lines (no overlap, no gap) still form one block.
func TestBlocks_AdjacentWindowsMerge(t *testing.T) {
	files := Blocks([]Match{
		{Repo: "org/repo", File: "a.go", Line: 2, Text: "two", After: "three\n"},
		{Repo: "org/repo", File: "a.go", Line: 5, Text: "five", Before: "four\n"},
	})
	if n := len(files[0].Blocks); n != 1 {
		t.Fatalf("got %d blocks, want 1", n)
	}
	wantRendered(t, files[0].Render(), "org/repo/a.go", "2:two", "3-three", "4-four", "5:five")
}

// TestBlocks_FilesKeepFirstAppearanceOrder: files group by repo/path in the
// order the ranked matches first mention them, even when a later match
// belongs to an earlier file.
func TestBlocks_FilesKeepFirstAppearanceOrder(t *testing.T) {
	files := Blocks([]Match{
		{Repo: "org/repo", File: "z.go", Line: 2, Text: "z2"},
		{Repo: "org/other", File: "z.go", Line: 9, Text: "o9"},
		{Repo: "org/repo", File: "z.go", Line: 1, Text: "z1"},
	})
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	wantRendered(t, files[0].Render(), "org/repo/z.go", "1:z1", "2:z2")
	wantRendered(t, files[1].Render(), "org/other/z.go", "9:o9")
}

// TestBlocks_SymbolHitKeepsKind: a sym: hit's kind and parent survive the
// merge and render as the trailing marker, even when the line was first seen
// as another hit's context.
func TestBlocks_SymbolHitKeepsKind(t *testing.T) {
	files := Blocks([]Match{
		{Repo: "org/repo", File: "a.go", Line: 3, Text: "// doc", After: "func Hello() {\n"},
		{Repo: "org/repo", File: "a.go", Line: 4, Text: "func Hello() {", Kind: "function"},
		{
			Repo: "org/repo", File: "a.go", Line: 9, Text: "func (p *Point) Hello() {",
			Kind: "method", Parent: "Point",
		},
	})
	wantRendered(
		t, files[0].Render(),
		"org/repo/a.go",
		"3:// doc",
		"4:func Hello() {  kind=function",
		"--",
		"9:func (p *Point) Hello() {  kind=method parent=Point",
	)
	if l := files[0].Blocks[0].Lines[1]; !l.Match || l.Kind != "function" {
		t.Errorf("line 4 = %+v, want a function match", l)
	}
}

func TestBlocks_Empty(t *testing.T) {
	if got := Blocks(nil); len(got) != 0 {
		t.Fatalf("Blocks(nil) = %v, want empty", got)
	}
}

func TestContextLines(t *testing.T) {
	tests := []struct {
		block string
		want  []string
	}{
		{block: "", want: nil},
		{block: "a", want: []string{"a"}},
		{block: "a\n", want: []string{"a"}},
		{block: "a\nb\n", want: []string{"a", "b"}},
		{block: "\n", want: []string{""}},
	}
	for _, tt := range tests {
		got := contextLines(tt.block)
		if strings.Join(got, "|") != strings.Join(tt.want, "|") || len(got) != len(tt.want) {
			t.Errorf("contextLines(%q) = %q, want %q", tt.block, got, tt.want)
		}
	}
}
