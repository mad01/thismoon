package ask

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMessageNumbersLinesAndNamesFiles(t *testing.T) {
	msg := Message("why?", []File{{Path: "org/repo/a.go", Lines: []string{"one", "two"}}})
	for _, want := range []string{
		`<file path="org/repo/a.go" lines="2">`,
		"1: one\n2: two\n</file>",
		"Question: why?",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"a\nb\n", []string{"a", "b"}},
		{"a\nb", []string{"a", "b"}},
		{"a\n\n\n", []string{"a"}},
		{"", []string{""}},
	}
	for _, tc := range tests {
		if got := SplitLines([]byte(tc.in)); !slices.Equal(got, tc.want) {
			t.Errorf("SplitLines(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLimitsResolve(t *testing.T) {
	got, err := Limits{}.Resolve()
	if err != nil || got != (Limits{MaxFiles: DefaultMaxFiles, MaxLines: DefaultMaxLines}) {
		t.Errorf("zero Limits resolve to %+v, %v; want the defaults", got, err)
	}
	got, err = Limits{MaxFiles: MaxFilesCeiling, MaxLines: 1}.Resolve()
	if err != nil || got != (Limits{MaxFiles: MaxFilesCeiling, MaxLines: 1}) {
		t.Errorf("in-range Limits resolve to %+v, %v; want them unchanged", got, err)
	}
	for _, bad := range []Limits{
		{MaxFiles: MaxFilesCeiling + 1},
		{MaxFiles: -1},
		{MaxLines: MaxLinesCeiling + 1},
	} {
		if _, err := bad.Resolve(); err == nil {
			t.Errorf("%+v resolved, want an error naming the ceiling", bad)
		}
	}
}

func TestUnionTakesTurnsAndDedupes(t *testing.T) {
	a, b, c, d := Hit{"o/r", "a"}, Hit{"o/r", "b"}, Hit{"o/r", "c"}, Hit{"o/s", "d"}
	got := Union([][]Hit{{a, b, c}, {d, a}, nil})
	if want := []Hit{a, d, b, c}; !slices.Equal(got, want) {
		t.Errorf("Union = %v, want %v", got, want)
	}
}

// repoTree writes files under a temp repo root and returns the root.
func repoTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestBuildCapsAndSkips(t *testing.T) {
	root := repoTree(t, map[string]string{
		"small.go": "a\nb\n",
		"big.go":   strings.Repeat("x\n", 10),
		"tiny.go":  "z\n",
		"last.go":  "q\n",
		"bin.dat":  "a\x00b",
	})
	roots := map[string]string{"o/r": root}
	hits := []Hit{
		{"o/r", "small.go"},
		{"o/r", "small.go"}, // a repeat is read once and not counted again
		{"o/r", "missing.go"},
		{"o/r", "../outside.go"},
		{"o/r", "bin.dat"},
		{"other/repo", "x.go"},
		{"o/r", "big.go"}, // 10 lines with 3 left: skipped, the next still fits
		{"o/r", "tiny.go"},
		{"o/r", "last.go"}, // past max_files: skipped unread
	}
	c := Build(hits, roots, Limits{MaxFiles: 2, MaxLines: 5})

	var kept []string
	for _, f := range c.Files {
		kept = append(kept, f.Path)
	}
	if want := []string{"small.go", "tiny.go"}; !slices.Equal(kept, want) {
		t.Fatalf("kept %v, want %v", kept, want)
	}
	if c.Lines != 3 || c.Chars != len("a\nb\n")+len("z\n") || c.FilesTotal != 8 {
		t.Errorf("lines=%d chars=%d files_total=%d, want 3, 6, 8", c.Lines, c.Chars, c.FilesTotal)
	}
	reasons := map[string]string{}
	for _, s := range c.Skipped {
		reasons[s.Path] = s.Reason
	}
	for path, want := range map[string]string{
		"missing.go":    "not in the checkout",
		"../outside.go": "outside",
		"bin.dat":       "binary",
		"x.go":          "not among the discovered repos",
		"big.go":        "10 lines, 3 left under max_lines (5)",
		"last.go":       "over max_files (2)",
	} {
		if !strings.Contains(reasons[path], want) {
			t.Errorf("skip reason for %s = %q, want it to mention %q", path, reasons[path], want)
		}
	}
	files := workerFiles(c.Files)
	if files[0].Path != "o/r/small.go" || !slices.Equal(files[0].Lines, []string{"a", "b"}) {
		t.Errorf("worker file = %+v, want o/r/small.go with its two lines", files[0])
	}
}

func TestParseFileList(t *testing.T) {
	names := []string{"org/repo", "org/repo/nested", "solo"}
	hits, err := ParseFileList([]string{
		"org/repo/cmd/main.go",
		"",
		"  org/repo/nested/x.go  ",
		"solo/README.md",
		noMatchesLine,
	}, names)
	if err != nil {
		t.Fatal(err)
	}
	want := []Hit{{"org/repo", "cmd/main.go"}, {"org/repo/nested", "x.go"}, {"solo", "README.md"}}
	if !slices.Equal(hits, want) {
		t.Errorf("ParseFileList = %v, want %v", hits, want)
	}

	for _, bad := range []string{"12:3:func main() {", "elsewhere/repo/a.go", "org/repo/"} {
		if _, err := ParseFileList([]string{bad}, names); err == nil {
			t.Errorf("ParseFileList(%q) = nil error, want one naming the line", bad)
		}
	}
}
