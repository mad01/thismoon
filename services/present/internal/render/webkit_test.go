package render_test

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/render"
)

// The brief template must source its chrome (palette, header, controls) from the
// shared webkit bundle, not from inlined copies. An empty workdir makes
// LoadTemplate fall back to the embedded default template.
func TestEmbeddedTemplateUsesWebkit(t *testing.T) {
	tmpl, err := render.LoadTemplate(t.TempDir())
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}

	for _, want := range []string{
		`/webkit/webkit.css`,
		`/webkit/webkit.js`,
		`<wk-header`,
		`wk-themechange`,
	} {
		if !strings.Contains(tmpl, want) {
			t.Errorf("embedded template missing webkit reference %q", want)
		}
	}

	// The old inlined chrome must be gone — it now lives in webkit.
	for _, bad := range []string{
		`brief-theme`,
		`id="themeToggle"`,
		`FONT_STACKS`,
		`id="bionicToggle"`,
	} {
		if strings.Contains(tmpl, bad) {
			t.Errorf("embedded template still contains migrated-out chrome %q", bad)
		}
	}
}
