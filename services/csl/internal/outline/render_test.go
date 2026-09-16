package outline

import (
	"strings"
	"testing"
)

// wantLines compares rendered text against the expected lines and the shared
// contract: no trailing whitespace on any line, no trailing newline.
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

// TestRender_GroupsByFileInRankOrder: files appear in the order of their best
// symbol, each symbol on one line with its parent when nested, and a trailer
// carries the counts.
func TestRender_GroupsByFileInRankOrder(t *testing.T) {
	res := Result{
		Repo: "org/fx", Path: ".", FilesScanned: 12, SymbolsTotal: 4,
		Symbols: []Entry{
			{Name: "Foo", Kind: "struct", File: "a.go", Line: 3, Refs: 2},
			{Name: "Qux", Kind: "struct", File: "sub/d.go", Line: 3, Refs: 1},
			{Name: "Bar", Kind: "method", Parent: "Foo", File: "a.go", Line: 7},
			{Name: "Baz", Kind: "function", File: "sub/d.go", Line: 5},
		},
	}
	wantLines(
		t, Render(res),
		"org/fx/a.go",
		"3 struct Foo  refs=2",
		"7 method Bar (Foo)  refs=0",
		"",
		"org/fx/sub/d.go",
		"3 struct Qux  refs=1",
		"5 function Baz  refs=0",
		"",
		"4 symbols in 2 files; 12 files scanned",
	)
}

func TestRender_TruncatedTrailer(t *testing.T) {
	res := Result{
		Repo: "org/fx", Path: "sub", FilesScanned: 1, SymbolsTotal: 9, Truncated: true,
		Symbols: []Entry{{Name: "Qux", Kind: "struct", File: "sub/d.go", Line: 3, Refs: 1}},
	}
	wantLines(
		t, Render(res),
		"org/fx/sub/d.go",
		"3 struct Qux  refs=1",
		"",
		"1 symbol in 1 file; 1 file scanned",
		"truncated: showing 1 of 9 symbols; raise limit or narrow with path/kinds",
	)
}

func TestRender_Empty(t *testing.T) {
	res := Result{Repo: "org/fx", Path: "docs", FilesScanned: 3, Symbols: []Entry{}}
	wantLines(t, Render(res), "no symbols in org/fx/docs; 3 files scanned")

	res.SymbolsSkipped = 12
	wantLines(t, Render(res),
		"no symbols in org/fx/docs; 3 files scanned; 12 definitions of other kinds skipped "+
			"(set kinds; fields, enumerators, and headings are out by default)")
}

func TestRender_FilesCappedTrailer(t *testing.T) {
	res := Result{
		Repo: "org/fx", Path: ".", FilesScanned: 20000, SymbolsTotal: 1, Truncated: true,
		FilesCapped: true,
		Symbols:     []Entry{{Name: "Qux", Kind: "struct", File: "sub/d.go", Line: 3, Refs: 1}},
	}
	wantLines(
		t,
		Render(res),
		"org/fx/sub/d.go",
		"3 struct Qux  refs=1",
		"",
		"1 symbol in 1 file; 20000 files scanned",
		"truncated: the walk stopped at 20000 files (max_files); definitions and counts cover those files only",
	)
}
