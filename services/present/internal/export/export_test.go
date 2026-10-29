package export

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

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
	if !strings.Contains(out, `<main id="root"><div class="brief">`+body) {
		t.Error("the rendered body is not mounted in #root")
	}
	if !strings.Contains(
		out,
		`<a href="https://example.com/runbook" target="_blank" rel="noopener">Runbook</a>`,
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

func TestNPMBytesCutsTheGeneratedBanner(t *testing.T) {
	npm := []byte("/*!\n * Chart.js v4.4.6\n */\n!function(){}();")
	generated := append(
		[]byte(
			"/**\n * Skipped minification because the original files appears to be already minified.\n"+
				" * Do NOT use SRI with dynamically generated files!\n */\n",
		),
		npm...)
	if got := npmBytes(generated); string(got) != string(npm) {
		t.Errorf("npmBytes(generated) = %q, want the npm file", got)
	}
	if got := npmBytes(npm); string(got) != string(npm) {
		t.Errorf("npmBytes(npm) = %q, want it unchanged", got)
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

// The library table and scripts/cache-assets.sh must name the same files at
// the same package versions, so an export hashes the file the CDN serves
// for the version the shell loads.
func TestLibrariesMatchTheAssetCache(t *testing.T) {
	script, err := os.ReadFile("../../scripts/cache-assets.sh")
	if err != nil {
		t.Fatal(err)
	}
	fetched := cachedURLs(t, string(script))
	for _, lib := range libraries {
		url, ok := fetched[lib.file]
		if !ok {
			t.Errorf("cache-assets.sh never fetches %s", lib.file)
			continue
		}
		pkg := cdnBase + strings.SplitN(lib.npm, "/", 2)[0] + "/"
		if !strings.HasPrefix(url, pkg) {
			t.Errorf(
				"%s: cache-assets.sh fetches %s, export loads %s",
				lib.file,
				url,
				cdnBase+lib.npm,
			)
		}
	}
}

var (
	reVersionVar = regexp.MustCompile(`(?m)^([A-Z0-9_]+_VERSION)="([^"]+)"$`)
	reFileVar    = regexp.MustCompile(`(?m)^([a-z0-9_]+_file)="\$ASSETS/js/([^"]+)"$`)
	reFetch      = regexp.MustCompile(`(?m)^fetch "([^"]+)" "\$([a-z0-9_]+_file)"`)
	reVarRef     = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)
)

// cachedURLs maps each cached js file name to the URL cache-assets.sh
// fetches it from.
func cachedURLs(t *testing.T, script string) map[string]string {
	t.Helper()
	vars := map[string]string{}
	for _, m := range reVersionVar.FindAllStringSubmatch(script, -1) {
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
	out := map[string]string{}
	for _, m := range reFetch.FindAllStringSubmatch(script, -1) {
		if name, ok := files[m[2]]; ok {
			out[name] = expand(m[1])
		}
	}
	if len(out) == 0 {
		t.Fatal("found no js fetch in cache-assets.sh; the patterns no longer match")
	}
	return out
}
