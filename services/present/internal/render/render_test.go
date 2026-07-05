package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/store"
)

func TestRenderSubstitutesPlaceholders(t *testing.T) {
	tmpl := `<title>{{TITLE}}</title><body>{{CONTENT}}{{GRAPH_SCRIPT}}{{LIVE_RELOAD}}</body>`
	p := store.Page{ID: "abc1234567", Title: "Hello & Co", Content: "<p>body</p>", Version: 3}
	out := Render(tmpl, p)

	if !strings.Contains(out, "<title>Hello &amp; Co</title>") {
		t.Errorf("title not escaped/substituted: %s", out)
	}
	if !strings.Contains(out, "<p>body</p>") {
		t.Errorf("content not injected: %s", out)
	}
	if strings.Contains(out, "{{") {
		t.Errorf("unsubstituted placeholder remains: %s", out)
	}
}

func TestRenderEmptyGraphOmitsScript(t *testing.T) {
	tmpl := `X{{GRAPH_SCRIPT}}Y`
	out := Render(tmpl, store.Page{ID: "id00000000"})
	if out != "XY" {
		t.Errorf("empty graph should produce no script block, got %q", out)
	}
}

func TestRenderGraphWrappedInScript(t *testing.T) {
	tmpl := `{{GRAPH_SCRIPT}}`
	out := Render(tmpl, store.Page{ID: "id00000000", Graph: "initGraph();"})
	if !strings.Contains(out, "<script>") || !strings.Contains(out, "initGraph();") {
		t.Errorf("graph not wrapped in script: %q", out)
	}
}

func TestLiveReloadScriptCarriesIDAndVersion(t *testing.T) {
	s := LiveReloadScript("feedface01", 7)
	if !strings.Contains(s, `"feedface01"`) {
		t.Errorf("id missing from live reload script: %s", s)
	}
	if !strings.Contains(s, "known = 7") {
		t.Errorf("version missing from live reload script: %s", s)
	}
	if !strings.Contains(s, "/p/") || !strings.Contains(s, "location.reload()") {
		t.Errorf("live reload script missing poll/reload logic: %s", s)
	}
}

func TestEnsureAndLoadTemplate(t *testing.T) {
	dir := t.TempDir()

	// Missing file -> LoadTemplate falls back to embedded default.
	got, err := LoadTemplate(dir)
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	if got != DefaultTemplate() {
		t.Fatal("LoadTemplate should fall back to embedded default when file absent")
	}

	// EnsureTemplate writes the default to disk.
	if err := EnsureTemplate(dir); err != nil {
		t.Fatalf("EnsureTemplate: %v", err)
	}
	if _, err := os.Stat(TemplatePath(dir)); err != nil {
		t.Fatalf("template file not written: %v", err)
	}

	// A stale template is overwritten with the current embedded version.
	stale := "STALE {{CONTENT}}"
	if err := os.WriteFile(TemplatePath(dir), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureTemplate(dir); err != nil {
		t.Fatalf("EnsureTemplate over stale: %v", err)
	}
	got, _ = LoadTemplate(dir)
	if got != DefaultTemplate() {
		t.Fatalf(
			"stale template should be replaced with embedded default, got %q",
			got[:min(len(got), 40)],
		)
	}

	// An up-to-date template is left alone (no unnecessary write).
	if err := EnsureTemplate(dir); err != nil {
		t.Fatalf("EnsureTemplate on current: %v", err)
	}
	got, _ = LoadTemplate(dir)
	if got != DefaultTemplate() {
		t.Fatal("current template should remain unchanged")
	}
}

func TestRenderReferencesEmptyOmitsBlock(t *testing.T) {
	tmpl := `X{{REFERENCES}}Y`
	out := Render(tmpl, store.Page{ID: "id00000000"})
	if out != "XY" {
		t.Errorf("empty refs should produce no block, got %q", out)
	}
}

func TestRenderReferencesProducesLinks(t *testing.T) {
	tmpl := `{{REFERENCES}}`
	p := store.Page{
		ID: "id00000000",
		References: []store.Reference{
			{Title: "dotfiles", URL: "https://github.com/mad01/dotfiles"},
			{Title: "Go docs", URL: "https://pkg.go.dev/std"},
		},
	}
	out := Render(tmpl, p)
	if !strings.Contains(out, `id="references"`) {
		t.Error("missing references section id")
	}
	if !strings.Contains(out, "dotfiles") || !strings.Contains(out, "github.com/mad01/dotfiles") {
		t.Error("missing first reference")
	}
	if !strings.Contains(out, "Go docs") || !strings.Contains(out, "pkg.go.dev/std") {
		t.Error("missing second reference")
	}
}

func TestRenderReferencesSkipsInvalidURLs(t *testing.T) {
	tmpl := `{{REFERENCES}}`
	p := store.Page{
		ID: "id00000000",
		References: []store.Reference{
			{Title: "valid", URL: "https://example.com"},
			{Title: "no scheme", URL: "not-a-url"},
			{Title: "ftp", URL: "ftp://files.example.com"},
		},
	}
	out := Render(tmpl, p)
	if !strings.Contains(out, "valid") {
		t.Error("valid ref missing")
	}
	if strings.Contains(out, "no scheme") || strings.Contains(out, "not-a-url") {
		t.Error("invalid URL should be skipped")
	}
	if strings.Contains(out, "ftp") {
		t.Error("ftp URL should be skipped")
	}
}

// Guard: the embedded default template carries every placeholder Render fills,
// so a future template edit that drops one is caught.
func TestDefaultTemplateHasPlaceholders(t *testing.T) {
	tmpl := DefaultTemplate()
	for _, ph := range []string{"{{TITLE}}", "{{CONTENT}}", "{{REFERENCES}}", "{{GRAPH_SCRIPT}}", "{{LIVE_RELOAD}}"} {
		if !strings.Contains(tmpl, ph) {
			t.Errorf("default template missing placeholder %s", ph)
		}
	}
	// Sanity: it should reference the template filename location helper too.
	_ = filepath.Base(TemplatePath("x"))
}
