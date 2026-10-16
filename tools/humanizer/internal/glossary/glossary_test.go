package glossary

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// repoRoot is the thismoon checkout, four levels above this package.
const repoRoot = "../../../.."

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Entry
	}{
		{
			name: "term, definition, avoid line",
			in:   "**Component**:\nA unit.\n_Avoid_: module, package\n",
			want: []Entry{{Term: "Component", Avoid: []string{"module", "package"}}},
		},
		{
			name: "parenthetical with a comma is stripped before splitting",
			in:   "**Tool**:\n_Avoid_: binary (a file, not a role), utility\n",
			want: []Entry{{Term: "Tool", Avoid: []string{"binary", "utility"}}},
		},
		{
			name: "colon inside the bold and code spans",
			in:   "**Deck:**\n_Avoid_: `slideshow`, presentation.\n",
			want: []Entry{{Term: "Deck", Avoid: []string{"slideshow", "presentation"}}},
		},
		{
			name: "avoid line with no term above is ignored",
			in:   "_Avoid_: orphan\n**Step**:\nOne press.\n",
			want: nil,
		},
		{
			name: "a second avoid line after the first is ignored",
			in:   "**Step**:\n_Avoid_: stage\n_Avoid_: build\n",
			want: []Entry{{Term: "Step", Avoid: []string{"stage"}}},
		},
		{
			name: "nested parentheses",
			in:   "**Weaver**:\n_Avoid_: merger (a role (not a process)), committer\n",
			want: []Entry{{Term: "Weaver", Avoid: []string{"merger", "committer"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSubstitutions(t *testing.T) {
	tests := []struct {
		name    string
		entries []Entry
		want    []Substitution
	}{
		{
			name: "sorted case-insensitively",
			entries: []Entry{
				{Term: "Service", Avoid: []string{"daemon", "App"}},
				{Term: "Tool", Avoid: []string{"binary"}},
			},
			want: []Substitution{
				{Avoid: "App", Use: "Service"},
				{Avoid: "binary", Use: "Tool"},
				{Avoid: "daemon", Use: "Service"},
			},
		},
		{
			name: "a synonym that is itself a term is dropped",
			entries: []Entry{
				{Term: "Component", Avoid: []string{"module"}},
				{Term: "Service", Avoid: []string{"component", "app"}},
			},
			want: []Substitution{
				{Avoid: "app", Use: "Service"},
				{Avoid: "module", Use: "Component"},
			},
		},
		{
			name: "a synonym under two terms lists both in file order",
			entries: []Entry{
				{Term: "Deck", Avoid: []string{"slideshow"}},
				{Term: "Presenting", Avoid: []string{"Slideshow"}},
			},
			want: []Substitution{{Avoid: "slideshow", Use: "Deck or Presenting"}},
		},
		{
			name: "a duplicate under one term collapses",
			entries: []Entry{
				{Term: "Step", Avoid: []string{"stage"}},
				{Term: "Step", Avoid: []string{"Stage"}},
			},
			want: []Substitution{{Avoid: "stage", Use: "Step"}},
		},
		{name: "no entries", entries: nil, want: []Substitution{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Substitutions(tt.entries); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Substitutions = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestRenderGolden pins the generated rule for the fixture CONTEXT.md: the
// metadata header, the vale fields, and the quoted, sorted swap map.
func TestRenderGolden(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "CONTEXT.md"))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	entries, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "Glossary.golden.yml"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got := Render(Substitutions(entries), MultiWordTerms(entries)); !bytes.Equal(got, want) {
		t.Errorf("Render mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderQuotesRegexAndYAML(t *testing.T) {
	got := string(
		Render([]Substitution{{Avoid: "c.f. it's", Use: "Ref's"}}, []string{"Ref's page"}),
	)
	for _, want := range []string{`  'c\.f\. it''s': 'Ref''s'`, `  - '(?i)Ref''s page'`} {
		if !strings.Contains(got, want) {
			t.Errorf("Render = %q, want the line %q", got, want)
		}
	}
	empty := string(Render(nil, nil))
	if !strings.HasSuffix(empty, "swap: {}\n") || strings.Contains(empty, "exceptions:") {
		t.Errorf("Render(nil, nil) = %q, want an empty swap map and no exceptions", empty)
	}
}

func TestMultiWordTerms(t *testing.T) {
	entries := []Entry{
		{Term: "Slide", Avoid: []string{"page"}},
		{Term: "Ephemeral page", Avoid: []string{"temporary page"}},
		{Term: "author key", Avoid: []string{"token"}},
		{Term: "Ephemeral Page", Avoid: []string{"TTL page"}},
	}
	want := []string{"author key", "Ephemeral page"}
	if got := MultiWordTerms(entries); !reflect.DeepEqual(got, want) {
		t.Errorf("MultiWordTerms = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestFromRepo checks the walk: CONTEXT.md files at any depth count, while
// dot-directories, vendor, node_modules, and testdata are skipped.
func TestFromRepo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "CONTEXT.md"), "**Tool**:\n_Avoid_: binary\n")
	writeFile(t, filepath.Join(root, "tools", "x", "CONTEXT.md"), "**Step**:\n_Avoid_: stage\n")
	for _, skipped := range []string{".git", "vendor", "node_modules", "testdata"} {
		writeFile(t, filepath.Join(root, skipped, "CONTEXT.md"), "**Bad**:\n_Avoid_: "+skipped+"\n")
	}
	got, err := FromRepo(root)
	if err != nil {
		t.Fatalf("FromRepo: %v", err)
	}
	want := Render(
		[]Substitution{{Avoid: "binary", Use: "Tool"}, {Avoid: "stage", Use: "Step"}},
		nil,
	)
	if !bytes.Equal(got, want) {
		t.Errorf("FromRepo =\n%s\nwant\n%s", got, want)
	}
}

func TestFromRepoWithoutGlossary(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "# nothing here\n")
	got, err := FromRepo(root)
	if err != nil || got != nil {
		t.Fatalf("FromRepo = %q, %v; want nil, nil", got, err)
	}
}

func TestFromRepoErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	if _, err := FromRepo(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("FromRepo(missing) error = %v, want fs.ErrNotExist", err)
	}
	file := filepath.Join(t.TempDir(), "CONTEXT.md")
	writeFile(t, file, "**Tool**:\n")
	if _, err := FromRepo(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("FromRepo(file) error = %v, want a not-a-directory error", err)
	}
}

// TestFromRepoRealContextFiles runs the generator over this repo's own
// CONTEXT.md files: the output is non-empty and byte-identical across runs.
func TestFromRepoRealContextFiles(t *testing.T) {
	if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err != nil {
		t.Skipf("repo root not found: %v", err)
	}
	first, err := FromRepo(repoRoot)
	if err != nil {
		t.Fatalf("FromRepo: %v", err)
	}
	if !bytes.Contains(first, []byte("swap:\n  '")) {
		t.Fatalf("FromRepo produced no substitutions:\n%s", first)
	}
	second, err := FromRepo(repoRoot)
	if err != nil {
		t.Fatalf("FromRepo second run: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("FromRepo output differs between two runs")
	}
}
