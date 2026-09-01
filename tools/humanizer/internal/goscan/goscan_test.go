package goscan

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestScanSourceKinds pins one extraction site per kind: the block's kind,
// label, line, and text all come from the contract the CLI prints, so a
// change in any of them is a change in the output.
func TestScanSourceKinds(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		kind  Kind
		label string
		line  int
		text  string
	}{
		{
			name:  "package doc comment",
			src:   "// Package demo does one thing.\npackage demo\n",
			kind:  KindDoc,
			label: "package demo",
			line:  1,
			text:  "Package demo does one thing.",
		},
		{
			name:  "function doc comment",
			src:   "package demo\n\n// Run starts the widget.\nfunc Run() {}\n",
			kind:  KindDoc,
			label: "func Run",
			line:  3,
			text:  "Run starts the widget.",
		},
		{
			name:  "cobra short field",
			src:   "package demo\n\nvar c = cmd{\n\tShort: \"Build a widget\",\n}\n",
			kind:  KindCobra,
			label: "Short",
			line:  4,
			text:  "Build a widget",
		},
		{
			name:  "mcp tool description across a concatenation",
			src:   "package demo\n\nvar t = tool{\n\tDescription: \"Build a widget. \" +\n\t\t\"Pass the name the caller typed.\",\n}\n",
			kind:  KindMCP,
			label: "Description",
			line:  4,
			text:  "Build a widget. Pass the name the caller typed.",
		},
		{
			name:  "fmt.Errorf message",
			src:   "package demo\n\nfunc f() error { return fmt.Errorf(\"demo: the widget has no name\") }\n",
			kind:  KindError,
			label: "fmt.Errorf",
			line:  3,
			text:  "demo: the widget has no name",
		},
		{
			name:  "errors.New message",
			src:   "package demo\n\nvar e = errors.New(\"demo: the store is read only\")\n",
			kind:  KindError,
			label: "errors.New",
			line:  3,
			text:  "demo: the store is read only",
		},
		{
			name:  "http.Error message",
			src:   "package demo\n\nfunc h() { http.Error(w, \"demo: only POST is allowed\", 405) }\n",
			kind:  KindError,
			label: "http.Error",
			line:  3,
			text:  "demo: only POST is allowed",
		},
		{
			name:  "jsonschema struct tag",
			src:   "package demo\n\ntype in struct {\n\tName string `json:\"name\" jsonschema:\"the widget name\"`\n}\n",
			kind:  KindSchema,
			label: "Name",
			line:  4,
			text:  "the widget name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, err := ScanSource("demo.go", tt.src, Options{Kinds: []Kind{tt.kind}})
			if err != nil {
				t.Fatalf("ScanSource() error = %v", err)
			}
			if len(blocks) != 1 {
				t.Fatalf("ScanSource() returned %d blocks, want 1: %+v", len(blocks), blocks)
			}
			got := blocks[0]
			if got.Kind != tt.kind || got.Label != tt.label || got.Line != tt.line {
				t.Errorf("got kind=%q label=%q line=%d, want kind=%q label=%q line=%d",
					got.Kind, got.Label, got.Line, tt.kind, tt.label, tt.line)
			}
			if got.Text != tt.text {
				t.Errorf("got text %q, want %q", got.Text, tt.text)
			}
			if got.File != "demo.go" {
				t.Errorf("got file %q, want %q", got.File, "demo.go")
			}
		})
	}
}

// TestScanFileEveryKind is the acceptance case: one Go file yields all five
// categories in source order.
func TestScanFileEveryKind(t *testing.T) {
	files, err := Scan(filepath.Join("testdata", "sample", "cmd.go"), Options{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Scan() returned %d files, want 1", len(files))
	}
	seen := map[Kind]bool{}
	prev := 0
	for _, b := range files[0].Blocks {
		seen[b.Kind] = true
		if b.Line < prev {
			t.Errorf("blocks out of source order: line %d after %d", b.Line, prev)
		}
		prev = b.Line
	}
	for _, k := range []Kind{KindDoc, KindCobra, KindError} {
		if !seen[k] {
			t.Errorf("cmd.go yielded no %q block", k)
		}
	}

	tools, err := Scan(filepath.Join("testdata", "sample", "tools.go"), Options{})
	if err != nil {
		t.Fatalf("Scan(tools.go) error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("Scan(tools.go) returned %d files, want 1", len(tools))
	}
	seen = map[Kind]bool{}
	for _, b := range tools[0].Blocks {
		seen[b.Kind] = true
	}
	for _, k := range []Kind{KindMCP, KindSchema} {
		if !seen[k] {
			t.Errorf("tools.go yielded no %q block", k)
		}
	}
}

// TestScanDirRecursive walks the fixture tree: nested packages come along,
// vendor, testdata, and generated files stay out.
func TestScanDirRecursive(t *testing.T) {
	files, err := Scan(filepath.Join("testdata", "sample"), Options{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, filepath.ToSlash(f.Path))
	}
	want := []string{
		"testdata/sample/cmd.go",
		"testdata/sample/nested/deep.go",
		"testdata/sample/tools.go",
	}
	for _, w := range want {
		if !slices.Contains(paths, w) {
			t.Errorf("walk missed %s; got %v", w, paths)
		}
	}
	for _, skipped := range []string{"vendor", "testdata/sample/testdata", "api.pb.go", "marked.go"} {
		for _, p := range paths {
			if strings.Contains(p, skipped) {
				t.Errorf("walk should have skipped %s, got %s", skipped, p)
			}
		}
	}
}

// TestScanKindFilter checks the filter the MCP wrapper and --kind share.
func TestScanKindFilter(t *testing.T) {
	files, err := Scan(
		filepath.Join("testdata", "sample", "cmd.go"),
		Options{Kinds: []Kind{KindError}},
	)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Scan() returned %d files, want 1", len(files))
	}
	for _, b := range files[0].Blocks {
		if b.Kind != KindError {
			t.Errorf("kind filter let through a %q block: %+v", b.Kind, b)
		}
	}
	if len(files[0].Blocks) == 0 {
		t.Error("kind filter dropped every block")
	}
}

// TestScanMissingPath reports the path in the error, since the CLI prints
// it straight to the user.
func TestScanMissingPath(t *testing.T) {
	_, err := Scan(filepath.Join(t.TempDir(), "absent.go"), Options{})
	if err == nil {
		t.Fatal("Scan() on a missing path returned no error")
	}
	if !strings.Contains(err.Error(), "absent.go") {
		t.Errorf("error %q does not name the path", err)
	}
}

// TestRenderSourceLine pins the line map detection findings travel back
// through: every document line resolves to the source line behind it.
func TestRenderSourceLine(t *testing.T) {
	blocks := []Block{
		{File: "a.go", Line: 10, Kind: KindDoc, Text: "first line\nsecond line"},
		{File: "a.go", Line: 30, Kind: KindError, Text: "a single line"},
	}
	doc := Render(blocks)
	wantText := "first line\nsecond line\n\na single line\n"
	if doc.Text != wantText {
		t.Errorf("Render() text = %q, want %q", doc.Text, wantText)
	}
	tests := []struct{ docLine, srcLine int }{
		{1, 10},
		{2, 11},
		{4, 30},
		{0, 0},
		{99, 0},
	}
	for _, tt := range tests {
		if got := doc.SourceLine(tt.docLine); got != tt.srcLine {
			t.Errorf("SourceLine(%d) = %d, want %d", tt.docLine, got, tt.srcLine)
		}
	}
}

func TestParseKind(t *testing.T) {
	if k, ok := ParseKind("cobra"); !ok || k != KindCobra {
		t.Errorf("ParseKind(cobra) = %q, %v", k, ok)
	}
	if _, ok := ParseKind("prose"); ok {
		t.Error("ParseKind(prose) reported a known kind")
	}
}
