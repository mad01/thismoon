package outline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// fixtureRepo lays out a small Go repo. Reference counts, with test files
// skipped: Foo is referenced by b.go, sub/d.go and docs/README.md (3),
// NewFoo by b.go (1), Qux (defined under sub/) by b.go and the README (2);
// FooBar, Bar, Baz, useFoo and x have none. a_test.go references Foo and
// defines TestFoo; sub/e.go carries a field and the README a heading, both
// out of the default kinds; .git and node_modules hold decoys the walk
// must skip.
func fixtureRepo(t *testing.T) finder.Repo {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"a.go": "package fx\n\ntype Foo struct{}\n\nfunc NewFoo() *Foo { return &Foo{} }\n\n" +
			"func (f *Foo) Bar() {}\n",
		"b.go":                  "package fx\n\nvar x Foo\n\nfunc useFoo() { _ = NewFoo(); var _ Qux }\n",
		"c.go":                  "package fx\n\ntype FooBar struct{}\n",
		"sub/d.go":              "package sub\n\ntype Qux struct{}\n\nfunc Baz() Foo { return Foo{} }\n",
		"a_test.go":             "package fx\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) { _ = Foo{} }\n",
		"docs/README.md":        "# Guide\n\nFoo and Qux.\n",
		"sub/e.go":              "package sub\n\ntype Rec struct{ Field int }\n",
		".git/HEAD":             "ref: refs/heads/main\n",
		"node_modules/dep/x.go": "package dep\n\ntype Foo struct{}\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return finder.Repo{Name: "org/fx", Path: root}
}

// names lists the ranked names for a compact order assertion.
func names(entries []Entry) string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return strings.Join(out, " ")
}

// TestBuild_RanksByRefsThenKindThenName is the ranking contract: most
// referenced first, then containers before callables before members, then
// name.
func TestBuild_RanksByRefsThenKindThenName(t *testing.T) {
	res, err := Build(fixtureRepo(t), Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.Repo != "org/fx" || res.Path != "." {
		t.Errorf("repo/path = %q/%q, want org/fx/.", res.Repo, res.Path)
	}
	if res.FilesScanned != 6 {
		t.Errorf(
			"FilesScanned = %d, want 6 (a, b, c, docs/README, sub/d, sub/e; tests and decoys skipped)",
			res.FilesScanned,
		)
	}
	want := "Foo Qux NewFoo FooBar Rec Bar Baz useFoo x"
	if got := names(res.Symbols); got != want {
		t.Errorf("ranked names = %q, want %q", got, want)
	}
	if res.SymbolsTotal != 9 || res.SymbolsSkipped != 2 || res.Truncated {
		t.Errorf("SymbolsTotal = %d, SymbolsSkipped = %d, Truncated = %v, want 9/2/false",
			res.SymbolsTotal, res.SymbolsSkipped, res.Truncated)
	}

	first := res.Symbols[0]
	if first.Kind != "struct" || first.File != "a.go" || first.Line != 3 || first.Refs != 3 {
		t.Errorf("Foo = %+v, want struct at a.go:3 with 3 refs", first)
	}
	bar := res.Symbols[5]
	if bar.Parent != "Foo" || bar.Kind != "method" || bar.Line != 7 {
		t.Errorf("Bar = %+v, want method of Foo at line 7", bar)
	}
}

// TestBuild_PathNarrowsDefinitionsNotReferences: path picks the files that
// define, while references still come from the whole repo.
func TestBuild_PathNarrowsDefinitionsNotReferences(t *testing.T) {
	res, err := Build(fixtureRepo(t), Options{Path: "sub/"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.Path != "sub" {
		t.Errorf("Path = %q, want sub", res.Path)
	}
	if got := names(res.Symbols); got != "Qux Rec Baz" {
		t.Errorf("ranked names = %q, want %q", got, "Qux Rec Baz")
	}
	if res.Symbols[0].Refs != 2 {
		t.Errorf(
			"Qux refs = %d, want 2 from b.go and the README outside the path",
			res.Symbols[0].Refs,
		)
	}
	if res.FilesScanned != 6 {
		t.Errorf("FilesScanned = %d, want 6 (references scan the whole repo)", res.FilesScanned)
	}
}

// TestBuild_IncludeTests: test files are out of both the definitions and
// the reference count unless asked for.
func TestBuild_IncludeTests(t *testing.T) {
	repo := fixtureRepo(t)
	without, err := Build(repo, Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if strings.Contains(names(without.Symbols), "TestFoo") {
		t.Error("TestFoo is listed with tests excluded")
	}

	with, err := Build(repo, Options{IncludeTests: true})
	if err != nil {
		t.Fatalf("Build with tests: %v", err)
	}
	if !strings.Contains(names(with.Symbols), "TestFoo") {
		t.Errorf("TestFoo missing with tests included: %q", names(with.Symbols))
	}
	if with.Symbols[0].Name != "Foo" || with.Symbols[0].Refs != 4 {
		t.Errorf("Foo = %+v, want 4 refs once a_test.go counts", with.Symbols[0])
	}
	if with.FilesScanned != 7 {
		t.Errorf("FilesScanned = %d, want 7", with.FilesScanned)
	}
}

// TestBuild_LimitTruncates: the limit cuts the ranked list and says so.
func TestBuild_LimitTruncates(t *testing.T) {
	res, err := Build(fixtureRepo(t), Options{Limit: 2})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := names(res.Symbols); got != "Foo Qux" {
		t.Errorf("ranked names = %q, want %q", got, "Foo Qux")
	}
	if !res.Truncated || res.SymbolsTotal != 9 {
		t.Errorf("Truncated = %v, SymbolsTotal = %d, want true/9", res.Truncated, res.SymbolsTotal)
	}
}

// TestBuild_KindsFilter keeps only the requested kinds and refuses unknown
// ones with the vocabulary.
func TestBuild_KindsFilter(t *testing.T) {
	res, err := Build(fixtureRepo(t), Options{Kinds: []string{"struct", " method "}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := names(res.Symbols); got != "Foo Qux FooBar Rec Bar" {
		t.Errorf("ranked names = %q, want %q", got, "Foo Qux FooBar Rec Bar")
	}

	// Members and headings appear only when asked for.
	res, err = Build(fixtureRepo(t), Options{Kinds: []string{"field", "section"}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := names(res.Symbols); got != "Field Guide" {
		t.Errorf("ranked names = %q, want %q", got, "Field Guide")
	}
	if res.SymbolsSkipped != 9 {
		t.Errorf("SymbolsSkipped = %d, want 9", res.SymbolsSkipped)
	}

	_, err = Build(fixtureRepo(t), Options{Kinds: []string{"widget"}})
	if err == nil || !strings.Contains(err.Error(), `unknown kind "widget"`) ||
		!strings.Contains(err.Error(), "class, enum, interface") {
		t.Errorf("unknown kind error = %v, want it to name the kind and the vocabulary", err)
	}
}

// TestBuild_BadPath: a missing or escaping path is an error, not an empty
// outline.
func TestBuild_BadPath(t *testing.T) {
	repo := fixtureRepo(t)
	for _, p := range []string{"nope", "../x", "/etc"} {
		if _, err := Build(repo, Options{Path: p}); err == nil {
			t.Errorf("Build(path=%q) succeeded, want an error", p)
		}
	}
}

func TestClampLimit(t *testing.T) {
	for in, want := range map[int]int{0: DefaultLimit, -5: DefaultLimit, 7: 7, 9999: MaxLimit} {
		if got := clampLimit(in); got != want {
			t.Errorf("clampLimit(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestKinds(t *testing.T) {
	got := strings.Join(Kinds(), " ")
	want := "class enum interface namespace struct type typealias " +
		"function method methodSpec const enumerator field section var"
	if got != want {
		t.Errorf("Kinds() = %q, want %q", got, want)
	}
	got = strings.Join(DefaultKinds(), " ")
	want = "class enum interface namespace struct type typealias function method methodSpec const var"
	if got != want {
		t.Errorf("DefaultKinds() = %q, want %q", got, want)
	}
}

// TestBuild_MaxFilesCapsTheWalk: the walk stops at max_files in path order
// and the result says so; definitions and counts cover the visited files.
func TestBuild_MaxFilesCapsTheWalk(t *testing.T) {
	res, err := Build(fixtureRepo(t), Options{MaxFiles: 2})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.FilesScanned != 2 || !res.FilesCapped || !res.Truncated {
		t.Errorf("scanned %d, capped %v, truncated %v; want 2, true, true",
			res.FilesScanned, res.FilesCapped, res.Truncated)
	}
	if got := names(res.Symbols); got != "Foo NewFoo Bar useFoo x" {
		t.Errorf("ranked names = %q, want the definitions of a.go and b.go only", got)
	}
	if res.Symbols[0].Refs != 1 {
		t.Errorf("Foo refs = %d, want 1 (only b.go was scanned)", res.Symbols[0].Refs)
	}
}
