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
			if !reflect.DeepEqual(got.Entries, tt.want) {
				t.Errorf("Parse entries = %+v, want %+v", got.Entries, tt.want)
			}
			if len(got.Skips) != 0 {
				t.Errorf("Parse skips = %+v, want none", got.Skips)
			}
		})
	}
}

// TestParseSkips pins the validation of an _Avoid_ line: an unmatched "("
// turns the rest of the line into a note, and a synonym with a non-word edge
// is skipped, because the rule's word boundaries would never match it. Each
// skip carries its line.
func TestParseSkips(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		want      []Entry
		wantSkips []Skip
	}{
		{
			name: "unclosed parenthesis keeps the synonyms before it",
			in:   "**Tool**:\n_Avoid_: binary (a file, trailing note\n",
			want: []Entry{{Term: "Tool", Avoid: []string{"binary"}}},
			wantSkips: []Skip{
				{Line: 2, Synonym: "(a file, trailing note", Reason: reasonUnclosed},
			},
		},
		{
			name: "non-word edges are skipped",
			in:   "# t\n**Push**:\n_Avoid_: --force, [x], force-push, it's-a-key\n",
			want: []Entry{{Term: "Push", Avoid: []string{"force-push", "it's-a-key"}}},
			wantSkips: []Skip{
				{Line: 3, Synonym: "--force", Reason: reasonEdge},
				{Line: 3, Synonym: "[x]", Reason: reasonEdge},
			},
		},
		{
			name:      "a line of only skips yields no entry",
			in:        "**Flag**:\n_Avoid_: -f\n**Next**:\n_Avoid_: after\n",
			want:      []Entry{{Term: "Next", Avoid: []string{"after"}}},
			wantSkips: []Skip{{Line: 2, Synonym: "-f", Reason: reasonEdge}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !reflect.DeepEqual(got.Entries, tt.want) {
				t.Errorf("Parse entries = %+v, want %+v", got.Entries, tt.want)
			}
			if !reflect.DeepEqual(got.Skips, tt.wantSkips) {
				t.Errorf("Parse skips = %+v, want %+v", got.Skips, tt.wantSkips)
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
	ctx, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "Glossary.golden.yml"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got := RuleFor([]Context{ctx}, ""); !bytes.Equal(got, want) {
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

// TestLoad checks the walk: CONTEXT.md files at any depth count, while
// dot-directories, vendor, node_modules, and testdata are skipped. With no
// location, every glossary found applies.
func TestLoad(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "CONTEXT.md"), "**Tool**:\n_Avoid_: binary\n")
	writeFile(t, filepath.Join(root, "tools", "x", "CONTEXT.md"), "**Step**:\n_Avoid_: stage\n")
	for _, skipped := range []string{".git", "vendor", "node_modules", "testdata"} {
		writeFile(t, filepath.Join(root, skipped, "CONTEXT.md"), "**Bad**:\n_Avoid_: "+skipped+"\n")
	}
	repo, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Render(
		[]Substitution{{Avoid: "binary", Use: "Tool"}, {Avoid: "stage", Use: "Step"}},
		nil,
	)
	if got := repo.Rule(""); !bytes.Equal(got, want) {
		t.Errorf("Rule(\"\") =\n%s\nwant\n%s", got, want)
	}
}

func TestLoadWithoutGlossary(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "# nothing here\n")
	repo, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := repo.Rule(""); got != nil {
		t.Fatalf("Rule(\"\") = %q, want nil", got)
	}
}

func TestLoadErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	if _, err := Load(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load(missing) error = %v, want fs.ErrNotExist", err)
	}
	file := filepath.Join(t.TempDir(), "CONTEXT.md")
	writeFile(t, file, "**Tool**:\n")
	if _, err := Load(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("Load(file) error = %v, want a not-a-directory error", err)
	}
}

// scopedFixture is a two-context tree: a root glossary plus alpha/ and
// beta/ glossaries. alpha bans "version"; beta defines Worker, which the
// root glossary bans as a synonym of Unit.
const scopedFixture = "../../testdata/ste/scoped"

// swapKeys returns the banned synonyms of a rendered rule, in rule order.
func swapKeys(t *testing.T, rule []byte) []string {
	t.Helper()
	_, swap, ok := strings.Cut(string(rule), "swap:\n")
	if !ok {
		return nil
	}
	var keys []string
	for _, line := range strings.Split(strings.TrimSpace(swap), "\n") {
		key, _, _ := strings.Cut(strings.TrimSpace(line), ":")
		keys = append(keys, strings.Trim(key, "'"))
	}
	return keys
}

// TestRuleScopedByContext is the scope contract over the fixture tree: a
// file gets the glossaries on its own path only, so a synonym alpha bans is
// not flagged in beta's files, and a synonym that is beta's own term is not
// flagged there even though the root glossary bans it.
func TestRuleScopedByContext(t *testing.T) {
	repo, err := Load(scopedFixture)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tests := []struct {
		target string
		want   []string
	}{
		{target: "README.md", want: []string{"manifest", "subtask", "worker"}},
		{target: ".", want: []string{"manifest", "subtask", "worker"}},
		{
			target: "alpha/notes.md",
			want:   []string{"manifest", "slideshow", "subtask", "version", "worker"},
		},
		{target: "alpha", want: []string{"manifest", "slideshow", "subtask", "version", "worker"}},
		{target: "beta/notes.md", want: []string{"manifest", "runner", "subtask"}},
		{target: "betamax/notes.md", want: []string{"manifest", "subtask", "worker"}},
		{
			target: "",
			want:   []string{"manifest", "runner", "slideshow", "subtask", "version"},
		},
	}
	for _, tt := range tests {
		t.Run("target "+tt.target, func(t *testing.T) {
			if got := swapKeys(t, repo.Rule(tt.target)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Rule(%q) bans %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

func TestRepoTarget(t *testing.T) {
	repo, err := Load(scopedFixture)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	abs, err := filepath.Abs(filepath.Join(scopedFixture, "beta", "notes.md"))
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{in: abs, want: "beta/notes.md"},
		{in: "beta/notes.md", want: "beta/notes.md"},
		{in: "alpha/new-file.md", want: "alpha/new-file.md"},
		{in: ".", want: "."},
		{in: filepath.Join(t.TempDir(), "brief.md"), want: ""},
		{in: "../outside.md", want: ""},
	}
	for _, tt := range tests {
		if got := repo.Target(tt.in); got != tt.want {
			t.Errorf("Target(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRepoTargetThroughSymlink resolves a path that does not exist yet
// against a repo reached through a symlink, as t.TempDir is on macOS.
func TestRepoTargetThroughSymlink(t *testing.T) {
	realDir := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	writeFile(t, filepath.Join(realDir, "deck", "CONTEXT.md"), "**Step**:\n_Avoid_: stage\n")
	repo, err := Load(link)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, in := range []string{
		filepath.Join(link, "deck", "new", "slides.md"),
		filepath.Join(realDir, "deck", "new", "slides.md"),
	} {
		if got := repo.Target(in); got != "deck/new/slides.md" {
			t.Errorf("Target(%q) = %q, want deck/new/slides.md", in, got)
		}
	}
}

func TestRepoWarnings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "x", "CONTEXT.md"), "**Push**:\n_Avoid_: --force, shove\n")
	repo, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{`x/CONTEXT.md:2: skipped "--force": ` + reasonEdge}
	if got := repo.Warnings(); !reflect.DeepEqual(got, want) {
		t.Errorf("Warnings = %q, want %q", got, want)
	}
}

// TestLoadRealContextFiles runs the generator over this repo's own
// CONTEXT.md files: the output is non-empty and byte-identical across runs.
func TestLoadRealContextFiles(t *testing.T) {
	if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err != nil {
		t.Skipf("repo root not found: %v", err)
	}
	first, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rule := first.Rule("")
	if !bytes.Contains(rule, []byte("swap:\n  '")) {
		t.Fatalf("Rule produced no substitutions:\n%s", rule)
	}
	second, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load second run: %v", err)
	}
	if !bytes.Equal(rule, second.Rule("")) {
		t.Error("Rule output differs between two runs")
	}
}
