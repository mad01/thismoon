// Package render turns a stored page into a full HTML document by injecting its
// content into the shared core template. The template lives on disk in the
// working directory (seeded from an embedded default) so editing it re-renders
// every page on the next request.
package render

import (
	_ "embed"
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/services/present/internal/store"
)

//go:embed template.html
var defaultPageTemplate string

// templateFile is the core template's filename inside the working directory.
const templateFile = "template.html"

// DefaultTemplate returns the embedded core template.
func DefaultTemplate() string { return defaultPageTemplate }

// TemplatePath is the location of the editable core template.
func TemplatePath(workdir string) string { return filepath.Join(workdir, templateFile) }

// EnsureTemplate writes the embedded default template to the working directory
// when the file is missing or differs from the current embedded version. This
// keeps the on-disk template in sync with binary updates — the template is
// executable code (CSS, JS), not user data, so staleness causes UI regressions.
func EnsureTemplate(workdir string) error {
	path := TemplatePath(workdir)
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == defaultPageTemplate {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultPageTemplate), 0o644)
}

// LoadTemplate reads the core template from the working directory, falling back
// to the embedded default when no file is present.
func LoadTemplate(workdir string) (string, error) {
	data, err := os.ReadFile(TemplatePath(workdir))
	if errors.Is(err, os.ErrNotExist) {
		return defaultPageTemplate, nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Render injects a page into the given template. Title is HTML-escaped; Content
// and Graph are treated as trusted HTML/JS authored by the page creator.
func Render(tmpl string, p store.Page) string {
	graph := ""
	if p.Graph != "" {
		graph = "<script>\n" + p.Graph + "\n</script>"
	}
	r := strings.NewReplacer(
		"{{TITLE}}", html.EscapeString(p.Title),
		"{{CONTENT}}", p.Content,
		"{{REFERENCES}}", referencesHTML(p.References),
		"{{GRAPH_SCRIPT}}", graph,
		"{{LIVE_RELOAD}}", LiveReloadScript(p.ID, p.Version),
	)
	return r.Replace(tmpl)
}

func referencesHTML(refs []store.Reference) string {
	if len(refs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<wk-section class="refs-section" id="references">`)
	b.WriteString(`<wk-section-heading>References</wk-section-heading>`)
	b.WriteString(`<ul class="refs-list">`)
	for _, ref := range refs {
		u, err := url.Parse(ref.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		b.WriteString(`<li class="refs-item"><a href="`)
		b.WriteString(html.EscapeString(ref.URL))
		b.WriteString(`" target="_blank" rel="noopener">`)
		b.WriteString(html.EscapeString(ref.Title))
		b.WriteString(`</a><span class="refs-url">`)
		b.WriteString(html.EscapeString(u.Host + u.Path))
		b.WriteString(`</span></li>`)
	}
	b.WriteString(`</ul></wk-section>`)
	return b.String()
}

// LiveReloadScript returns a script that polls the page's version endpoint and
// reloads the tab when the version changes, so updates appear without manually
// reopening the browser.
func LiveReloadScript(id string, version int) string {
	return fmt.Sprintf(`<script>
(function(){
  const id = %q, known = %d;
  function poll(){
    fetch('/p/' + id + '/version', {cache:'no-store'})
      .then(function(r){ return r.ok ? r.text() : null; })
      .then(function(t){
        if (t !== null && parseInt(t, 10) !== known) { location.reload(); return; }
        setTimeout(poll, 1000);
      })
      .catch(function(){ setTimeout(poll, 1000); });
  }
  setTimeout(poll, 1000);
})();
</script>`, id, version)
}
