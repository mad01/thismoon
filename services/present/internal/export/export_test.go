package export

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/mad01/thismoon/services/present/internal/images"
	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// fakePages serves pages from a map, the one store read an export makes.
type fakePages map[string]store.Page

func (f fakePages) Get(_ context.Context, id string) (store.Page, error) {
	p, ok := f[id]
	if !ok {
		return store.Page{}, store.ErrNotFound
	}
	return p, nil
}

// cacheFile is the content the fake asset cache holds for a library: its
// own name, so a test can tell the integrity hashes apart.
func cacheFile(name string) []byte { return []byte("/* " + name + " */\n") }

// newWorkdir lays out an asset cache with every library and one latin font
// face, the way scripts/cache-assets.sh leaves it.
func newWorkdir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, lib := range libraries {
		writeFile(t, filepath.Join(dir, "assets", "js", lib.file), cacheFile(lib.name))
	}
	css := "/* latin-ext */\n@font-face {\n  font-family: 'Inter';\n  src: url(/assets/fonts/ext.woff2) format('woff2');\n}\n" +
		"/* latin */\n@font-face {\n  font-family: 'Inter';\n  src: url(/assets/fonts/latin.woff2) format('woff2');\n}\n"
	writeFile(t, filepath.Join(dir, "assets", "css", "fonts.css"), []byte(css))
	writeFile(t, filepath.Join(dir, "assets", "fonts", "latin.woff2"), []byte("wOF2latin"))
	writeFile(t, filepath.Join(dir, "assets", "fonts", "ext.woff2"), []byte("wOF2ext"))
	return dir
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// compiled renders a Doc the way the MCP tools store it.
func compiled(t *testing.T, doc string) string {
	t.Helper()
	c, err := render.Compile([]byte(doc), "Test page")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c.HTML
}

func export(t *testing.T, workdir string, p store.Page, deck bool) Result {
	t.Helper()
	res, err := New(
		fakePages{p.ID: p},
		workdir,
	).Export(context.Background(), Request{ID: p.ID, Deck: deck})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	return res
}

var reScriptSrc = regexp.MustCompile(
	`<script src="([^"]+)" integrity="(sha384-[^"]+)" crossorigin="anonymous">`,
)

// libraryTags returns the src and integrity of every library tag in out.
func libraryTags(out []byte) map[string]string {
	tags := map[string]string{}
	for _, m := range reScriptSrc.FindAllStringSubmatch(string(out), -1) {
		tags[m[1]] = m[2]
	}
	return tags
}

const proseDoc = `{"summary":"Why the cache went stale.","sections":[{"h":"What happened","blocks":[` +
	`{"t":"p","text":"The resolver kept a **dead** upstream."},{"t":"list","items":["restart","verify"]}]}]}`

func TestProsePageCarriesItsBodyAndNoLibrary(t *testing.T) {
	body := compiled(t, proseDoc)
	p := store.Page{
		ID: "prose", Title: "Stale <cache>", Version: 3, Content: body, HasBrief: true,
		References: []store.Reference{{Title: "Runbook", URL: "https://example.com/runbook"}},
	}
	res := export(t, newWorkdir(t), p, false)
	out := string(res.HTML)

	// The body is in the file as markup, so it reads with scripts off.
	mounted, err := wellFormed(body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `<main id="root"><div class="brief">`+mounted) {
		t.Error("the rendered body is not mounted in #root")
	}
	if !strings.Contains(
		out,
		`<a href="https://example.com/runbook" rel="noopener" target="_blank">Runbook</a>`,
	) {
		t.Error("the references block is not in the body")
	}
	if tags := libraryTags(res.HTML); len(tags) != 0 || len(res.Libraries) != 0 {
		t.Errorf("a prose page loads libraries: %v %v", tags, res.Libraries)
	}
	for _, want := range []string{
		`<html lang="en" data-present-mode="export">`,
		`<title>Stale &lt;cache&gt;</title>`,
		`<script type="application/json" id="present-export">{"id":"prose","title":"Stale \u003ccache\u003e","version":3,"view":"brief"`,
		"function renderExport()",                  // app.js
		"window.PresentViz",                        // viz.js
		"url(data:font/woff2;base64,d09GMmxhdGlu)", // the latin face, inlined
	} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q", want)
		}
	}
	if strings.Contains(out, "ext.woff2") || strings.Contains(out, "/assets/") {
		t.Error("export names a cached file instead of inlining the latin face only")
	}
	if res.Rendition != "brief" || len(res.Notes) != 0 {
		t.Errorf("rendition %q, notes %v", res.Rendition, res.Notes)
	}
}

// An export names no server for a control to use: its header lists only the
// controls that run in the browser, and its page data carries no speak URL,
// share, or deck remote (app.js builds none of them under standalone either).
func TestExportNamesNoServer(t *testing.T) {
	p := store.Page{ID: "prose", Title: "T", Content: compiled(t, proseDoc), HasDeck: true, Deck: "D"}
	for _, deck := range []bool{false, true} {
		out := string(export(t, newWorkdir(t), p, deck).HTML)
		if !strings.Contains(out, `<wk-header title="`) ||
			!strings.Contains(out, `controls="font,fixation,size,theme,help"`) {
			t.Errorf("deck=%v: the header lists a control beyond font, fixation, size, theme and help", deck)
		}
		island := regexp.MustCompile(`id="present-export">([^<]*)</script>`).FindStringSubmatch(out)
		if island == nil {
			t.Fatalf("deck=%v: no page data", deck)
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(island[1]), &data); err != nil {
			t.Fatal(err)
		}
		for key := range data {
			switch key {
			case "id", "title", "version", "view", "has_graph", "graph", "references":
			default:
				t.Errorf("deck=%v: page data carries %q", deck, key)
			}
		}
	}
}

const graphDoc = `{"sections":[{"h":"Topology","blocks":[{"t":"graph"}]}]}`

func graphPage(t *testing.T, layout string) store.Page {
	t.Helper()
	js, _, err := render.CompileGraph(render.GraphInput{
		Nodes:  []render.GraphNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		Edges:  []render.GraphEdge{{From: "a", To: "b"}},
		Layout: layout,
	})
	if err != nil {
		t.Fatal(err)
	}
	return store.Page{
		ID:       "graph",
		Title:    "Graph",
		Content:  compiled(t, graphDoc),
		Graph:    js,
		HasGraph: true,
	}
}

func TestGraphPageLoadsCytoscapeAndDagreOnly(t *testing.T) {
	res := export(t, newWorkdir(t), graphPage(t, ""), false)
	want := []string{
		"https://cdn.jsdelivr.net/npm/cytoscape@3.31.0/dist/cytoscape.min.js",
		"https://cdn.jsdelivr.net/npm/dagre@0.8.5/dist/dagre.min.js",
		"https://cdn.jsdelivr.net/npm/cytoscape-dagre@2.5.0/cytoscape-dagre.js",
	}
	if !slices.Equal(res.Libraries, want) {
		t.Fatalf("libraries = %v, want %v", res.Libraries, want)
	}
	tags := libraryTags(res.HTML)
	if len(tags) != len(want) {
		t.Fatalf("library tags = %v, want %d", tags, len(want))
	}
	for i, url := range want {
		lib := libraries[i]
		if got := tags[url]; got != integrity(cacheFile(lib.name)) {
			t.Errorf("%s integrity = %q, want the hash of the cached file", url, got)
		}
	}
}

func TestGraphWithAnELKEngineAddsELK(t *testing.T) {
	res := export(t, newWorkdir(t), graphPage(t, "elk-mrtree"), false)
	if len(res.Libraries) != 5 || !strings.Contains(res.Libraries[4], "cytoscape-elk@2.3.0") {
		t.Errorf("libraries = %v, want cytoscape, dagre and ELK with their adapters", res.Libraries)
	}
}

func TestLocalImageIsInlined(t *testing.T) {
	workdir := newWorkdir(t)
	png := []byte("\x89PNG\r\n\x1a\nfake")
	name := images.Name(png, "png")
	writeFile(t, filepath.Join(images.New(workdir).Dir(), name), png)
	doc := `{"sections":[{"h":"Screens","blocks":[{"t":"image","src":"` + images.Served(
		name,
	) + `","alt":"login"}]}]}`
	p := store.Page{ID: "img", Title: "Images", Content: compiled(t, doc), HasBrief: true}

	out := string(export(t, workdir, p, false).HTML)
	if strings.Contains(out, "/img/") {
		t.Error("the exported page still names the served image path")
	}
	if !strings.Contains(out, `src="data:image/png;base64,iVBORw0KGgpmYWtl"`) {
		t.Error("the image is not inlined as a data URI")
	}
}

func TestMissingImageFails(t *testing.T) {
	name := images.Name([]byte("gone"), "png")
	doc := `{"sections":[{"h":"S","blocks":[{"t":"image","src":"` + images.Served(
		name,
	) + `","alt":"x"}]}]}`
	p := store.Page{ID: "img", Title: "I", Content: compiled(t, doc)}
	_, err := New(
		fakePages{"img": p},
		newWorkdir(t),
	).Export(context.Background(), Request{ID: "img"})
	if err == nil || !strings.Contains(err.Error(), name) {
		t.Errorf("err = %v, want one naming the missing image", err)
	}
}

func TestRendition(t *testing.T) {
	cases := []struct {
		name     string
		page     store.Page
		deck     bool
		want     string
		wantBody string
		wantErr  error
	}{
		{"brief", store.Page{Content: "B", Deck: "D", HasDeck: true}, false, viewBrief, "B", nil},
		{
			"deck asked for",
			store.Page{Content: "B", Deck: "D", HasDeck: true},
			true,
			viewDeck,
			"D",
			nil,
		},
		{"deck-only page", store.Page{Deck: "D", HasDeck: true}, false, viewDeck, "D", nil},
		{"no deck", store.Page{Content: "B"}, true, "", "", store.ErrNoDeck},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view, body, err := rendition(tc.page, tc.deck)
			if !errors.Is(err, tc.wantErr) || view != tc.want || body != tc.wantBody {
				t.Errorf(
					"rendition = %q %q %v, want %q %q %v",
					view,
					body,
					err,
					tc.want,
					tc.wantBody,
					tc.wantErr,
				)
			}
		})
	}
}

func TestNeeds(t *testing.T) {
	chart := func(kind string) string {
		return `<script type="application/json" class="chart-spec">{"kind":"` + kind + `"}</script>`
	}
	cases := []struct {
		name  string
		body  string
		graph string
		want  []string
	}{
		{"prose", "<p>text</p>", "", nil},
		{"graph script without a placement", "<p>text</p>", "initGraph()", nil},
		{"bar chart", chart("bar"), "", []string{"chart.js"}},
		{"sankey", chart("sankey"), "", []string{"chart.js", "chartjs-chart-sankey"}},
		{"ribbon", chart("ribbon"), "", []string{"d3"}},
		{
			"diagram",
			`<script type="application/json" class="diagram-spec">{}</script>`,
			"",
			[]string{"elkjs", "d3"},
		},
		{"terminal", `<div class="present-terminal" data-session="dev"></div>`, "", nil},
		{
			"raw graph script that builds a chart",
			"<canvas></canvas>",
			"new Chart(el, cfg)",
			[]string{"chart.js"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := needs(tc.body, tc.graph)
			want := map[string]bool{}
			addAll(want, tc.want...)
			if len(got) != len(want) {
				t.Fatalf("needs = %v, want %v", got, want)
			}
			for k := range want {
				if !got[k] {
					t.Errorf("needs lacks %s: %v", k, got)
				}
			}
		})
	}
}

func TestMissingLibraryFails(t *testing.T) {
	workdir := newWorkdir(t)
	if err := os.Remove(filepath.Join(workdir, "assets", "js", libraries[0].file)); err != nil {
		t.Fatal(err)
	}
	_, err := New(
		fakePages{"graph": graphPage(t, "")},
		workdir,
	).Export(context.Background(), Request{ID: "graph"})
	if err == nil || !strings.Contains(err.Error(), "make cache") {
		t.Errorf("err = %v, want one pointing at make cache", err)
	}
}

func TestMissingFontsDegrade(t *testing.T) {
	workdir := newWorkdir(t)
	if err := os.Remove(filepath.Join(workdir, "assets", "css", "fonts.css")); err != nil {
		t.Fatal(err)
	}
	p := store.Page{ID: "prose", Title: "P", Content: compiled(t, proseDoc)}
	res := export(t, workdir, p, false)
	if len(res.Notes) != 1 || strings.Contains(string(res.HTML), "@font-face") {
		t.Errorf(
			"notes %v; a cache without fonts should export with system fonts and say so",
			res.Notes,
		)
	}
}

func TestInlineTextCannotCloseItsElement(t *testing.T) {
	if got := scriptText(`a = "</script><!--"; b = /<\/SCRIPT>/`); strings.Contains(
		strings.ToLower(got),
		"</script",
	) ||
		strings.Contains(got, "<!--") {
		t.Errorf("scriptText left a closing tag or comment opener: %s", got)
	}
	if got := styleText(`content: "</style>"`); strings.Contains(got, "</style") {
		t.Errorf("styleText left a closing tag: %s", got)
	}
}

// The library table and scripts/cache-assets.sh must name the same URL for
// every library, and the script must pin the sha256 of what it fetches
// there. The cached file an export hashes is then the file the CDN serves
// at the URL the export loads.
func TestLibrariesMatchTheAssetCache(t *testing.T) {
	fetched := cacheFetches(t)
	for _, lib := range libraries {
		f, ok := fetched[lib.file]
		if !ok {
			t.Errorf("cache-assets.sh never fetches %s", lib.file)
			continue
		}
		if want := cdnBase + lib.npm; f.url != want {
			t.Errorf("%s: cache-assets.sh fetches %s, export loads %s", lib.file, f.url, want)
		}
		if f.sha256 == "" {
			t.Errorf("%s: cache-assets.sh pins no sha256 for it", lib.file)
		}
	}
}

// With an asset cache on this machine ($PRESENT_WORKDIR or
// ~/.config/present), every cached library must hash to its pin, and the
// export must load it from its URL with the integrity hash of those bytes.
// The test skips without a cache, and skips a library the cache lacks.
func TestIntegrityOfTheRealAssetCache(t *testing.T) {
	assets := realAssets(t)
	fetched := cacheFetches(t)
	checked := 0
	for _, lib := range libraries {
		data, err := os.ReadFile(filepath.Join(assets, "js", lib.file))
		if errors.Is(err, fs.ErrNotExist) {
			t.Logf("%s is not in %s, skipped", lib.file, assets)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		checked++
		if got, want := fmt.Sprintf("%x", sha256.Sum256(data)), fetched[lib.file].sha256; got != want {
			t.Errorf("%s: cached sha256 %s, cache-assets.sh pins %s: run make cache-force",
				lib.file, got, want)
		}
		scripts, err := cdnScripts(assets, map[string]bool{lib.name: true})
		if err != nil {
			t.Fatal(err)
		}
		want := cdnScript{
			Package:   strings.SplitN(lib.npm, "/", 2)[0],
			URL:       cdnBase + lib.npm,
			Integrity: integrity(data),
		}
		if len(scripts) != 1 || scripts[0] != want {
			t.Errorf("%s: export tags %+v, want %+v", lib.name, scripts, want)
		}
	}
	if checked == 0 {
		t.Skipf("no library is in the asset cache at %s", assets)
	}
}

// realAssets is this machine's asset cache, the one present serve uses; the
// test skips when there is none.
func realAssets(t *testing.T) string {
	t.Helper()
	workdir := os.Getenv("PRESENT_WORKDIR")
	if workdir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("no home directory: %v", err)
		}
		workdir = filepath.Join(home, ".config", "present")
	}
	assets := filepath.Join(workdir, "assets")
	if _, err := os.Stat(filepath.Join(assets, "js")); err != nil {
		t.Skipf("no asset cache at %s", assets)
	}
	return assets
}

// cacheFetch is one js download in cache-assets.sh: the URL and the pinned
// sha256, empty when the script pins none.
type cacheFetch struct{ url, sha256 string }

var (
	reScriptVar = regexp.MustCompile(`(?m)^([A-Z0-9_]+_(?:VERSION|SHA256))="([^"]+)"$`)
	reFileVar   = regexp.MustCompile(`(?m)^([a-z0-9_]+_file)="\$ASSETS/js/([^"]+)"$`)
	reFetch     = regexp.MustCompile(
		`(?m)^fetch "([^"]+)" "\$([a-z0-9_]+_file)"(?: "\$([A-Z0-9_]+_SHA256)")?`,
	)
	reVarRef = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)
)

// cacheFetches maps each cached js file name to the download that fills it.
func cacheFetches(t *testing.T) map[string]cacheFetch {
	t.Helper()
	b, err := os.ReadFile("../../scripts/cache-assets.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(b)
	vars := map[string]string{}
	for _, m := range reScriptVar.FindAllStringSubmatch(script, -1) {
		vars[m[1]] = m[2]
	}
	expand := func(s string) string {
		return reVarRef.ReplaceAllStringFunc(
			s,
			func(ref string) string { return vars[reVarRef.FindStringSubmatch(ref)[1]] },
		)
	}
	files := map[string]string{}
	for _, m := range reFileVar.FindAllStringSubmatch(script, -1) {
		files[m[1]] = expand(m[2])
	}
	out := map[string]cacheFetch{}
	for _, m := range reFetch.FindAllStringSubmatch(script, -1) {
		if name, ok := files[m[2]]; ok {
			out[name] = cacheFetch{url: expand(m[1]), sha256: vars[m[3]]}
		}
	}
	if len(out) == 0 {
		t.Fatal("found no js fetch in cache-assets.sh; the patterns no longer match")
	}
	return out
}

// An html block can hold markup that would reach past the body in a file:
// an unclosed comment or element, a stray end tag, a script left in its
// escaped state, plaintext, or a comment a reader without scripts sees in
// a noscript block. None may swallow the page data or the scripts after
// it, whether the browser runs scripts or not.
func TestBodyCannotReachPastItself(t *testing.T) {
	cases := map[string]string{
		"comment opener as noscript text": `<noscript>&lt;!-- shown as text</noscript>`,
		"unclosed comment":                `<p>before</p><!-- never closed`,
		"stray script end tag":            `<p>one</p></script><p>two</p>`,
		"script left escaped":             `<script><!--<script>var a = 1;`,
		"plaintext":                       `<plaintext>the rest`,
		"comment in a noscript":           `<noscript><!-- never closed</noscript>`,
		"unclosed rcdata/table":           `<table><tr><td><textarea>`,
		"unclosed style element":          `<style>p { color: red }`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			quoted, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			doc := `{"sections":[{"h":"Raw","blocks":[{"t":"html","text":` + string(quoted) +
				`},{"t":"p","text":"after the block"}]}]}`
			p := store.Page{ID: "raw", Title: "Raw", Content: compiled(t, doc), HasBrief: true}
			out := export(t, newWorkdir(t), p, false).HTML
			for _, scripting := range []bool{true, false} {
				checkFileIntact(t, out, scripting)
			}
		})
	}
}

// checkFileIntact parses an exported file the way a browser does and fails
// unless the page data and the four inline scripts after it are whole.
func checkFileIntact(t *testing.T, out []byte, scripting bool) {
	t.Helper()
	root, err := html.ParseWithOptions(
		bytes.NewReader(out),
		html.ParseOptionEnableScripting(scripting),
	)
	if err != nil {
		t.Fatal(err)
	}
	var scripts []*html.Node
	for n := range root.Descendants() {
		if n.Type == html.ElementNode && n.DataAtom == atom.Script && n.Namespace == "" {
			scripts = append(scripts, n)
		}
	}
	island := slices.IndexFunc(
		scripts,
		func(n *html.Node) bool { return attr(n, "id") == "present-export" },
	)
	if island < 0 {
		t.Fatalf("scripting %v: the page data is gone", scripting)
	}
	var data pageData
	if err := json.Unmarshal([]byte(text(scripts[island])), &data); err != nil || data.ID != "raw" {
		t.Errorf("scripting %v: page data %q: %v", scripting, text(scripts[island]), err)
	}
	after := scripts[island+1:]
	if len(after) != 4 ||
		!strings.Contains(text(after[0]), "real.call(this, input, init)") ||
		!strings.Contains(text(after[2]), "window.PresentViz") ||
		!strings.Contains(text(after[3]), "function renderExport()") ||
		!strings.HasSuffix(strings.TrimSpace(text(after[3])), "})();") {
		t.Errorf("scripting %v: the scripts after the page data are not whole", scripting)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func text(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return b.String()
}

// A library tag waits for the body and webkit.js: the browser paints the
// prose and builds the header controls first, so a CDN that hangs holds up
// only viz.js and app.js, which draw with the libraries.
func TestLibraryTagsFollowTheBody(t *testing.T) {
	res := export(t, newWorkdir(t), graphPage(t, ""), false)
	out := string(res.HTML)
	if head := out[:strings.Index(out, "</head>")]; strings.Contains(head, "<script src=") {
		t.Error("a library tag sits in the head")
	}
	island := strings.Index(out, `id="present-export"`)
	guard := strings.Index(out, "real.call(this, input, init)")
	kit := strings.Index(out, "var Webkit = ")
	viz := strings.Index(out, "window.PresentViz")
	if island < 0 || guard < island || kit < guard || viz < kit {
		t.Fatalf(
			"want the page data (%d), the guard (%d), webkit.js (%d), and viz.js (%d) in that order",
			island,
			guard,
			kit,
			viz,
		)
	}
	for _, url := range res.Libraries {
		at := strings.Index(out, `<script src="`+url+`"`)
		if at < kit || at > viz {
			t.Errorf("%s loads at byte %d, want it between webkit.js (%d) and viz.js (%d)",
				url, at, kit, viz)
		}
	}
}

// The head comment carries the notices the inlined third-party code needs:
// each font family's copyright line and the OFL, and Prism's MIT licence.
// THIRD-PARTY.md lists the same lines.
func TestHeadCommentCarriesTheNotices(t *testing.T) {
	p := store.Page{ID: "prose", Title: "P", Version: 2, Content: compiled(t, proseDoc)}
	out := string(export(t, newWorkdir(t), p, false).HTML)
	start, end := strings.Index(out, "<!--"), strings.Index(out, "-->")
	if start < 0 || end < start || end > strings.Index(out, "<script") {
		t.Fatal("no comment before the first script")
	}
	comment := out[start:end]
	wants := append(
		slices.Clone(fontCopyrights),
		oflURL,
		"Exported by present from page prose, version 2.",
	)
	for _, want := range append(wants, strings.Split(prismLicense, "\n")...) {
		if !strings.Contains(comment, want) {
			t.Errorf("head comment lacks %q", want)
		}
	}
	third, err := os.ReadFile("../../THIRD-PARTY.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range append(slices.Clone(fontCopyrights), oflURL, "Copyright (c) 2012 Lea Verou") {
		if !strings.Contains(string(third), want) {
			t.Errorf("THIRD-PARTY.md lacks %q", want)
		}
	}
}

// A served page mounts the body through innerHTML, which never runs a
// script, so a script an html block carries must not run in an export
// either. The renderer's own JSON islands keep their type and text.
func TestBodyScriptsDoNotRun(t *testing.T) {
	raw := `<script id="classic">document.documentElement.dataset.ran = "1"</script>` +
		`<script id="module" type="module">import "./x.js"</script>` +
		`<script id="upper" type=" TEXT/JAVASCRIPT ">var a = 1</script>` +
		`<svg><script id="svg">document.title = "ran"</script></svg>` +
		`<script id="island" type="application/json">{"a":"<b>"}</script>`
	quoted, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	doc := `{"sections":[{"h":"Raw","blocks":[{"t":"html","text":` + string(quoted) + `}]}]}`
	p := store.Page{ID: "raw", Title: "Raw", Content: compiled(t, doc), HasBrief: true}
	root, err := html.Parse(bytes.NewReader(export(t, newWorkdir(t), p, false).HTML))
	if err != nil {
		t.Fatal(err)
	}
	scripts := map[string]*html.Node{}
	for n := range root.Descendants() {
		if n.Type == html.ElementNode && n.Data == "script" && attr(n, "id") != "" {
			scripts[attr(n, "id")] = n
		}
	}
	for _, id := range []string{"classic", "module", "upper", "svg"} {
		n := scripts[id]
		if n == nil {
			t.Errorf("script %s is gone", id)
			continue
		}
		if got := attr(n, "type"); got != "text/plain" {
			t.Errorf("script %s has type %q, want text/plain", id, got)
		}
	}
	island := scripts["island"]
	if island == nil || attr(island, "type") != "application/json" ||
		text(island) != `{"a":"<b>"}` {
		t.Errorf("the JSON island changed: %v", island)
	}
}
