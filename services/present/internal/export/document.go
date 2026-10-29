package export

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"net/url"
	"regexp"
	"strings"
	"text/template"

	"github.com/mad01/thismoon/services/present/internal/server"
	"github.com/mad01/thismoon/services/present/internal/store"
	"github.com/mad01/thismoon/webkit"
)

// documentSrc is the exported file's skeleton. It is a text template: every
// value is escaped for its place before it goes in (see newDocument), since
// most of them are whole scripts and stylesheets.
//
//go:embed document.html
var documentSrc string

var documentTmpl = template.Must(template.New("export").Parse(documentSrc))

// fetchGuard runs before webkit.js. An exported page has no server, yet
// webkit polls /webkit/version every few seconds; the guard answers every
// root-relative request with a 404 at once, so the browser makes no request
// that is bound to fail and logs nothing.
const fetchGuard = `(function () {
  var real = window.fetch;
  if (!real) return;
  window.fetch = function (input, init) {
    if (typeof input === 'string' && /^\/(?!\/)/.test(input)) {
      return Promise.resolve(new Response('', { status: 404 }));
    }
    return real.call(this, input, init);
  };
})();`

// document holds the exported file's parts, each escaped for its place.
type document struct {
	Title           string
	Notice          string
	Boot            string
	Fonts           string
	WebkitCSS       string
	Scripts         []cdnScript
	ShellCSS        string
	HeaderTitle     string
	FixationTargets string
	Body            string
	Data            string
	Guard           string
	WebkitJS        string
	VizJS           string
	AppJS           string
}

// pageData is the JSON island app.js reads in place of GET /api/p/{id}. It
// carries no body: app.js takes the one already mounted in #root, so the
// HTML and its images are in the file once.
type pageData struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	Version    int             `json:"version"`
	View       string          `json:"view"`
	HasGraph   bool            `json:"has_graph"`
	Graph      string          `json:"graph"`
	References []dataReference `json:"references"`
}

type dataReference struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// parts is what newDocument assembles besides the shared assets.
type parts struct {
	page    store.Page
	view    string
	body    string
	fonts   string
	scripts []cdnScript
}

// newDocument gathers webkit's and the page view's own assets around the
// page's parts and escapes each for its place in the template.
func newDocument(in parts) (document, error) {
	shell, err := server.StandaloneAssets()
	if err != nil {
		return document{}, err
	}
	kit, err := webkitAssets()
	if err != nil {
		return document{}, err
	}
	data, err := json.Marshal(newPageData(in.page, in.view))
	if err != nil {
		return document{}, fmt.Errorf("export: encode page data: %w", err)
	}
	return document{
		Title:           html.EscapeString(in.page.Title),
		Notice:          notice(in.page, in.scripts, in.fonts != ""),
		Boot:            scriptText(kit["boot.js"]),
		Fonts:           styleText(in.fonts),
		WebkitCSS:       styleText(kit["webkit.css"]),
		Scripts:         in.scripts,
		ShellCSS:        styleText(shell.ShellCSS),
		HeaderTitle:     headerTitle(in.view),
		FixationTargets: html.EscapeString(shell.FixationTargets),
		Body:            in.body,
		Data:            string(data), // json.Marshal escapes <, >, and &
		Guard:           fetchGuard,
		WebkitJS:        scriptText(kit["webkit.js"]),
		VizJS:           scriptText(shell.VizJS),
		AppJS:           scriptText(shell.AppJS),
	}, nil
}

// render executes the template.
func (d document) render() ([]byte, error) {
	var b strings.Builder
	if err := documentTmpl.Execute(&b, d); err != nil {
		return nil, fmt.Errorf("export: render: %w", err)
	}
	return []byte(b.String()), nil
}

// webkitAssets reads the webkit files an exported page inlines.
func webkitAssets() (map[string]string, error) {
	out := map[string]string{}
	for _, name := range []string{"boot.js", "webkit.css", "webkit.js"} {
		b, err := fs.ReadFile(webkit.FS(), name)
		if err != nil {
			return nil, fmt.Errorf("export: webkit %s: %w", name, err)
		}
		out[name] = string(b)
	}
	return out, nil
}

func newPageData(p store.Page, view string) pageData {
	refs := make([]dataReference, 0, len(p.References))
	for _, r := range p.References {
		refs = append(refs, dataReference{Title: r.Title, URL: r.URL})
	}
	return pageData{
		ID: p.ID, Title: p.Title, Version: p.Version, View: view,
		HasGraph: p.HasGraph, Graph: p.Graph, References: refs,
	}
}

func headerTitle(view string) string {
	if view == viewDeck {
		return "Deck"
	}
	return "Brief"
}

// notice is the comment at the head of the file: where the page came from,
// the libraries it loads, and the copyright and licence notices of the
// third-party code it carries, the fonts when it inlines them and the
// Prism code in webkit's bundle always.
func notice(p store.Page, scripts []cdnScript, fonts bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Exported by present from page %s, version %d.\n", p.ID, p.Version)
	if len(scripts) > 0 {
		pkgs := make([]string, 0, len(scripts))
		for _, s := range scripts {
			pkgs = append(pkgs, s.Package)
		}
		fmt.Fprintf(&b, "It loads %s from cdn.jsdelivr.net, each under its own licence.\n",
			strings.Join(pkgs, ", "))
	}
	if fonts {
		fmt.Fprintf(&b, "\nThe latin faces of these font families are inlined below, under the "+
			"SIL Open Font License, Version 1.1, %s\n", oflURL)
		for _, c := range fontCopyrights {
			b.WriteString(c + "\n")
		}
	}
	b.WriteString("\nThe webkit script below includes Prism, https://github.com/PrismJS/prism, " +
		"under this licence:\n\n")
	b.WriteString(prismLicense)
	return b.String()
}

// Inline scripts and stylesheets end at the first closing tag of their
// element, whatever the context, and a "<!--" in a script changes how the
// browser reads what follows. "<\/" and "\x3C" keep both out and mean the
// same in a JS string, regex, or comment; "<\/" does in a CSS string too.
var (
	reScriptClose = regexp.MustCompile(`(?i)</(script)|<!--`)
	reStyleClose  = regexp.MustCompile(`(?i)</(style)`)
)

func scriptText(s string) string {
	return reScriptClose.ReplaceAllStringFunc(s, func(m string) string {
		if m == "<!--" {
			return `\x3C!--`
		}
		return `<\/` + m[2:]
	})
}

func styleText(s string) string {
	return reStyleClose.ReplaceAllStringFunc(s, func(m string) string { return `<\/` + m[2:] })
}

// referencesHTML renders the references block app.js builds, so the page
// shows it with scripts off too: http(s) links only, each with its host
// and path.
func referencesHTML(refs []store.Reference) string {
	var items strings.Builder
	for _, r := range refs {
		u, err := url.Parse(r.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		path := u.EscapedPath()
		if path == "" {
			path = "/"
		}
		fmt.Fprintf(
			&items,
			`<li class="refs-item"><a href="%s" target="_blank" rel="noopener">%s</a><span class="refs-url">%s</span></li>`,
			html.EscapeString(r.URL),
			html.EscapeString(r.Title),
			html.EscapeString(u.Host+path),
		)
	}
	if items.Len() == 0 {
		return ""
	}
	return `<wk-section class="refs-section" id="references"><wk-section-heading>References</wk-section-heading>` +
		`<ul class="refs-list">` + items.String() + `</ul></wk-section>`
}
