package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newTestHandlers(t *testing.T) (*handlers, *[]string) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	opened := &[]string{}
	h := &handlers{
		store:   st,
		baseURL: "http://localhost:7423",
		open:    func(url string) error { *opened = append(*opened, url); return nil },
	}
	return h, opened
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func ptrStr(s string) *string { return &s }

func TestCreateReadUpdateFlow(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "T", Content: "<p>v1</p>"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.Version != 1 {
		t.Fatalf("unexpected create output: %+v", created)
	}
	if !strings.HasSuffix(created.URL, "/p/"+created.ID) {
		t.Fatalf("url = %q, want suffix /p/%s", created.URL, created.ID)
	}

	_, read, err := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if read.Content != "<p>v1</p>" || read.Title != "T" {
		t.Fatalf("read mismatch: %+v", read)
	}

	_, updated, err := h.handleUpdate(
		ctx,
		nil,
		updateInput{ID: created.ID, Content: ptrStr("<p>v2</p>")},
	)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("version = %d, want 2", updated.Version)
	}

	_, read2, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if read2.Content != "<p>v2</p>" || read2.Title != "T" {
		t.Fatalf("update did not patch correctly: %+v", read2)
	}
}

func TestListReturnsAll(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	_, _, _ = h.handleCreate(ctx, nil, createInput{Title: "A", Content: "<p>a</p>"})
	_, _, _ = h.handleCreate(ctx, nil, createInput{Title: "B", Content: "<p>b</p>"})

	_, out, err := h.handleList(ctx, nil, struct{}{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Pages) != 2 {
		t.Fatalf("list returned %d pages, want 2", len(out.Pages))
	}
	for _, p := range out.Pages {
		if !strings.HasPrefix(p.URL, "http://localhost:7423/p/") {
			t.Errorf("bad url %q", p.URL)
		}
	}
}

func TestOpenCallsOpenerWithURL(t *testing.T) {
	h, opened := newTestHandlers(t)
	ctx := context.Background()
	_, created, _ := h.handleCreate(ctx, nil, createInput{Title: "T", Content: "<p>x</p>"})

	_, out, err := h.handleOpen(ctx, nil, openInput{ID: created.ID})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !out.Opened {
		t.Fatal("Opened = false, want true")
	}
	if len(*opened) != 1 || (*opened)[0] != created.URL {
		t.Fatalf("opener called with %v, want [%s]", *opened, created.URL)
	}
}

func TestOpenUnknownIDFails(t *testing.T) {
	h, opened := newTestHandlers(t)
	_, _, err := h.handleOpen(context.Background(), nil, openInput{ID: "missing0000"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("open unknown id: err = %v, want ErrNotFound", err)
	}
	if len(*opened) != 0 {
		t.Fatal("opener should not be called for a missing page")
	}
}

func TestCreateAndReadWithReferences(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	refs := []refInput{
		{Title: "dotfiles", URL: "https://github.com/mad01/dotfiles"},
		{Title: "Go docs", URL: "https://pkg.go.dev"},
	}
	_, created, err := h.handleCreate(
		ctx,
		nil,
		createInput{Title: "T", Content: "<p>x</p>", References: refs},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, read, err := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(read.References) != 2 {
		t.Fatalf("refs = %d, want 2", len(read.References))
	}
	if read.References[0].Title != "dotfiles" ||
		read.References[0].URL != "https://github.com/mad01/dotfiles" {
		t.Fatalf("ref[0] mismatch: %+v", read.References[0])
	}
}

func TestUpdateReferences(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	_, created, _ := h.handleCreate(ctx, nil, createInput{Title: "T", Content: "<p>x</p>"})

	refs := []refInput{{Title: "PR", URL: "https://github.com/mad01/dotfiles/pull/1"}}
	_, updated, err := h.handleUpdate(ctx, nil, updateInput{ID: created.ID, References: &refs})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("version = %d, want 2", updated.Version)
	}
	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if len(read.References) != 1 || read.References[0].Title != "PR" {
		t.Fatalf("refs after update: %+v", read.References)
	}
}

func TestReadUnknownIDFails(t *testing.T) {
	h, _ := newTestHandlers(t)
	if _, _, err := h.handleRead(context.Background(), nil, readInput{ID: "missing0000"}); !errors.Is(
		err,
		store.ErrNotFound,
	) {
		t.Fatalf("read unknown id: err = %v, want ErrNotFound", err)
	}
}

// ── Structured Doc format tests ──

func TestCreatePersistsDocOnDocInput(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"sections": []map[string]any{
			{"h": "S", "blocks": []map[string]any{{"t": "p", "text": "hi"}}},
		},
	}
	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "Doc", Content: toJSON(doc)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !h.store.HasDoc(created.ID) {
		t.Fatal("doc.json not persisted on Doc-input create")
	}
	raw, err := h.store.LoadDoc(created.ID)
	if err != nil {
		t.Fatalf("LoadDoc: %v", err)
	}
	var got render.Doc
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("stored doc.json is not valid Doc JSON: %v", err)
	}
	if len(got.Sections) != 1 || got.Sections[0].Heading != "S" {
		t.Fatalf("doc round-trip lost data: %+v", got)
	}
}

func TestCreateDoesNotPersistDocOnHTMLInput(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	_, created, err := h.handleCreate(
		ctx,
		nil,
		createInput{Title: "Legacy", Content: "<p>raw html</p>"},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if h.store.HasDoc(created.ID) {
		t.Fatal("doc.json must not be written for legacy HTML input")
	}
}

func TestUpdatePersistsDocOnDocInput(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	_, created, _ := h.handleCreate(ctx, nil, createInput{Title: "T", Content: "<p>v1</p>"})
	if h.store.HasDoc(created.ID) {
		t.Fatal("precondition: legacy page should have no doc.json")
	}
	doc := map[string]any{
		"sections": []map[string]any{
			{"h": "Now Doc", "blocks": []map[string]any{{"t": "p", "text": "x"}}},
		},
	}
	if _, _, err := h.handleUpdate(ctx, nil, updateInput{ID: created.ID, Content: ptrStr(toJSON(doc))}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !h.store.HasDoc(created.ID) {
		t.Fatal("doc.json not persisted after updating with Doc content")
	}
}

func TestUpdateClearsStaleDocOnHTMLInput(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"sections": []map[string]any{
			{"h": "S", "blocks": []map[string]any{{"t": "p", "text": "hi"}}},
		},
	}
	_, created, _ := h.handleCreate(ctx, nil, createInput{Title: "T", Content: toJSON(doc)})
	if !h.store.HasDoc(created.ID) {
		t.Fatal("precondition: doc-input page should have doc.json")
	}
	if _, _, err := h.handleUpdate(ctx, nil, updateInput{ID: created.ID, Content: ptrStr("<p>now html</p>")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if h.store.HasDoc(created.ID) {
		t.Fatal("stale doc.json should be cleared when content replaced with HTML")
	}
}

func TestCreateWithDocJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"summary": "Test summary with **bold**.",
		"meta":    "2026-05-29",
		"chips":   []map[string]any{{"text": "3 items", "style": "stat"}},
		"sections": []map[string]any{
			{
				"h": "Overview",
				"blocks": []map[string]any{
					{"t": "p", "text": "Hello **world**."},
					{"t": "callout", "text": "Warning here.", "sev": "warn"},
				},
			},
		},
	}

	_, created, err := h.handleCreate(
		ctx,
		nil,
		createInput{Title: "Doc Test", Content: toJSON(doc)},
	)
	if err != nil {
		t.Fatalf("create with doc: %v", err)
	}

	_, read, err := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !strings.Contains(read.Content, `class="brief-title"`) {
		t.Error("missing brief-title in rendered content")
	}
	if !strings.Contains(read.Content, `<strong>bold</strong>`) {
		t.Error("inline bold not rendered in summary")
	}
	if !strings.Contains(read.Content, `<wk-badge variant="stat">`) {
		t.Error("chip not rendered")
	}
	if !strings.Contains(read.Content, `<strong>world</strong>`) {
		t.Error("inline bold not rendered in paragraph")
	}
	if !strings.Contains(read.Content, `<wk-callout variant="warn"`) {
		t.Error("callout with severity not rendered")
	}
	if !strings.Contains(read.Content, `<wk-section-heading>`) {
		t.Error("section heading not rendered")
	}
}

func TestCreateWithDocTable(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"sections": []map[string]any{
			{
				"h": "Data",
				"blocks": []map[string]any{
					{
						"t":    "table",
						"cols": []string{"Name", "Status"},
						"rows": [][]string{
							{"foo", "@chip(b:done)"},
							{"`bar`", "**active**"},
						},
					},
				},
			},
		},
	}

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "Table", Content: toJSON(doc)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})

	if !strings.Contains(read.Content, `<wk-table><table>`) {
		t.Error("table not rendered")
	}
	if !strings.Contains(read.Content, `<th>Name</th>`) {
		t.Error("table header missing")
	}
	if !strings.Contains(read.Content, `<wk-badge variant="b">`) {
		t.Error("chip in table cell not rendered")
	}
	if !strings.Contains(read.Content, `<code>bar</code>`) {
		t.Error("code in table cell not rendered")
	}
	if !strings.Contains(read.Content, `<strong>active</strong>`) {
		t.Error("bold in table cell not rendered")
	}
}

func TestCreateWithDocKV(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"sections": []map[string]any{
			{
				"h": "Info",
				"blocks": []map[string]any{
					{
						"t": "kv",
						"kv": []map[string]string{
							{"k": "Status", "v": "**active**"},
							{"k": "Path", "v": "`/usr/bin`"},
						},
					},
				},
			},
		},
	}

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "KV", Content: toJSON(doc)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if !strings.Contains(read.Content, `<wk-kv-label>`) {
		t.Error("kv-label not rendered")
	}
	if !strings.Contains(read.Content, `<wk-kv-value>`) {
		t.Error("kv-value not rendered")
	}
	if !strings.Contains(read.Content, `<strong>active</strong>`) {
		t.Error("bold in kv value not rendered")
	}
}

func TestCreateWithDocList(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"sections": []map[string]any{
			{
				"h": "Steps",
				"blocks": []map[string]any{
					{
						"t":       "list",
						"items":   []string{"First with `code`", "Second with **bold**"},
						"ordered": true,
					},
				},
			},
		},
	}

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "List", Content: toJSON(doc)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if !strings.Contains(read.Content, `<ol class="brief-list">`) {
		t.Error("ordered list not rendered")
	}
	if !strings.Contains(read.Content, `<code>code</code>`) {
		t.Error("code in list item not rendered")
	}
}

func TestCreateWithDocPanel(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"sections": []map[string]any{
			{
				"h": "Panels",
				"blocks": []map[string]any{
					{
						"t":      "panel",
						"title":  "My Panel",
						"sub":    "subtitle text",
						"accent": "terracotta",
					},
				},
			},
		},
	}

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "Panels", Content: toJSON(doc)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if !strings.Contains(read.Content, `<wk-panel`) {
		t.Error("panel not rendered")
	}
	if !strings.Contains(read.Content, `var(--terracotta)`) {
		t.Error("accent border not rendered")
	}
	if !strings.Contains(read.Content, `<wk-panel-title>`) {
		t.Error("panel title not rendered")
	}
}

func TestCreateWithDocProgress(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"sections": []map[string]any{
			{
				"h": "Progress",
				"blocks": []map[string]any{
					{"t": "progress", "pct": 72, "label": "72%"},
				},
			},
		},
	}

	_, created, err := h.handleCreate(
		ctx,
		nil,
		createInput{Title: "Progress", Content: toJSON(doc)},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if !strings.Contains(read.Content, `<wk-progress>`) {
		t.Error("progress not rendered")
	}
	if !strings.Contains(read.Content, `width:72%`) {
		t.Error("progress fill width not set")
	}
}

func TestCreateWithDocHTMLEscapeHatch(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"sections": []map[string]any{
			{
				"h": "Custom",
				"blocks": []map[string]any{
					{"t": "html", "text": `<div class="custom">raw html</div>`},
				},
			},
		},
	}

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "HTML", Content: toJSON(doc)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if !strings.Contains(read.Content, `<div class="custom">raw html</div>`) {
		t.Error("raw html not passed through")
	}
}

func TestCreateWithDocTOCGenerated(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"sections": []map[string]any{
			{"h": "First Section", "blocks": []map[string]any{{"t": "p", "text": "a"}}},
			{"h": "Second Section", "blocks": []map[string]any{{"t": "p", "text": "b"}}},
		},
	}

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "TOC", Content: toJSON(doc)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if !strings.Contains(read.Content, `<wk-toc>`) {
		t.Error("TOC not generated for multi-section doc")
	}
	if !strings.Contains(read.Content, `href="#first-section"`) {
		t.Error("TOC link to first section missing")
	}
	if !strings.Contains(read.Content, `href="#second-section"`) {
		t.Error("TOC link to second section missing")
	}
}

func TestCreateWithDocSingleSectionNoTOC(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"sections": []map[string]any{
			{"h": "Only", "blocks": []map[string]any{{"t": "p", "text": "solo"}}},
		},
	}

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "NoTOC", Content: toJSON(doc)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if strings.Contains(read.Content, `<wk-toc>`) {
		t.Error("TOC should not be generated for single-section doc")
	}
}

// ── Structured Graph format tests ──

func TestCreateWithStructuredGraph(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "core", "label": "Core", "type": "center"},
			{"id": "auth", "label": "Auth", "type": "module", "color": 0},
			{"id": "cache", "label": "Cache", "type": "leaf"},
		},
		"edges": []map[string]any{
			{"from": "core", "to": "auth"},
			{"from": "core", "to": "cache", "type": "publishes", "label": "v1"},
		},
		"layout": "cose",
	}

	_, created, err := h.handleCreate(ctx, nil, createInput{
		Title:   "Graph",
		Content: "<p>body</p>",
		Graph:   toJSON(graph),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if !strings.Contains(read.Graph, "getGraphColors") {
		t.Error("graph missing getGraphColors function")
	}
	if !strings.Contains(read.Graph, "initGraph") {
		t.Error("graph missing initGraph function")
	}
	if !strings.Contains(read.Graph, `Core`) {
		t.Error("graph missing node label")
	}
	if !strings.Contains(read.Graph, `publishes`) {
		t.Error("graph missing edge type")
	}
	if !strings.Contains(read.Graph, `name: 'cose'`) {
		t.Error("graph layout not set to cose")
	}
}

func TestCreateWithLegacyGraphString(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	legacyJS := "function getGraphColors(){} function initGraph(){}"

	_, created, err := h.handleCreate(ctx, nil, createInput{
		Title:   "Legacy",
		Content: "<p>body</p>",
		Graph:   legacyJS,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if read.Graph != legacyJS {
		t.Errorf("legacy graph not stored as-is: got %q", read.Graph)
	}
}

func TestCreateWithDocJSONString(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	docStr := `{"summary":"Doc passed as string.","sections":[{"h":"Test","blocks":[{"t":"p","text":"hello"}]}]}`

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "Wrapped", Content: docStr})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if !strings.Contains(read.Content, `class="brief-summary"`) {
		t.Error("doc JSON string should be detected and rendered as Doc")
	}
	if !strings.Contains(read.Content, `<wk-section-heading>`) {
		t.Error("section heading not rendered from doc JSON string")
	}
}

func TestUpdateWithDocJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	_, created, _ := h.handleCreate(ctx, nil, createInput{Title: "T", Content: "<p>old</p>"})

	doc := map[string]any{
		"sections": []map[string]any{
			{"h": "New", "blocks": []map[string]any{{"t": "p", "text": "updated"}}},
		},
	}
	newContent := toJSON(doc)
	_, updated, err := h.handleUpdate(ctx, nil, updateInput{
		ID:      created.ID,
		Content: &newContent,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("version = %d, want 2", updated.Version)
	}

	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if !strings.Contains(read.Content, `<wk-section-heading>`) {
		t.Error("updated content should be rendered from doc JSON")
	}
	if strings.Contains(read.Content, "<p>old</p>") {
		t.Error("old content still present after update")
	}
}

// ── Source persistence + present_source tests ──

func TestCreatePersistsGraphSourceOnStructuredGraph(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	graph := map[string]any{
		"nodes": []map[string]any{{"id": "a", "label": "A"}},
		"edges": []map[string]any{},
	}
	_, created, err := h.handleCreate(
		ctx,
		nil,
		createInput{Title: "G", Content: "<p>x</p>", Graph: toJSON(graph)},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !h.store.HasGraphSource(created.ID) {
		t.Fatal("graph.json not persisted on structured-graph create")
	}
	raw, err := h.store.LoadGraphSource(created.ID)
	if err != nil {
		t.Fatalf("LoadGraphSource: %v", err)
	}
	var got render.GraphInput
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("stored graph.json is not valid GraphInput JSON: %v", err)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].ID != "a" {
		t.Fatalf("graph source round-trip lost data: %+v", got)
	}
}

func TestCreateDoesNotPersistGraphSourceOnLegacyJS(t *testing.T) {
	h, _ := newTestHandlers(t)
	_, created, err := h.handleCreate(context.Background(), nil, createInput{
		Title: "G", Content: "<p>x</p>", Graph: "function initGraph(){}",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if h.store.HasGraphSource(created.ID) {
		t.Fatal("graph.json must not be written for legacy JS input")
	}
}

func TestUpdateClearsGraphSourceOnLegacyJSAndOnClear(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	graph := map[string]any{"nodes": []map[string]any{{"id": "a", "label": "A"}}}
	_, created, _ := h.handleCreate(
		ctx,
		nil,
		createInput{Title: "G", Content: "<p>x</p>", Graph: toJSON(graph)},
	)
	if !h.store.HasGraphSource(created.ID) {
		t.Fatal("precondition: structured-graph page should have graph.json")
	}

	// Replace with legacy JS: source is now stale and must be cleared.
	if _, _, err := h.handleUpdate(ctx, nil, updateInput{ID: created.ID, Graph: ptrStr("function initGraph(){}")}); err != nil {
		t.Fatalf("update with JS: %v", err)
	}
	if h.store.HasGraphSource(created.ID) {
		t.Fatal("stale graph.json should be cleared when graph replaced with JS")
	}

	// Re-add structured, then clear the graph entirely.
	if _, _, err := h.handleUpdate(ctx, nil, updateInput{ID: created.ID, Graph: ptrStr(toJSON(graph))}); err != nil {
		t.Fatalf("update with JSON: %v", err)
	}
	if !h.store.HasGraphSource(created.ID) {
		t.Fatal("graph.json should be re-persisted on structured update")
	}
	if _, _, err := h.handleUpdate(ctx, nil, updateInput{ID: created.ID, Graph: ptrStr("")}); err != nil {
		t.Fatalf("update clearing graph: %v", err)
	}
	if h.store.HasGraphSource(created.ID) {
		t.Fatal("graph.json should be cleared when graph is removed")
	}
}

func TestSourceReturnsDocAndGraphJSONRoundTrip(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"summary": "S",
		"sections": []map[string]any{
			{"h": "One", "blocks": []map[string]any{{"t": "p", "text": "hello"}}},
		},
	}
	graph := map[string]any{"nodes": []map[string]any{{"id": "a", "label": "A"}}, "layout": "cose"}
	refs := []refInput{{Title: "repo", URL: "https://example.com"}}
	_, created, err := h.handleCreate(ctx, nil, createInput{
		Title: "T", Content: toJSON(doc), Graph: toJSON(graph), References: refs,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, src, err := h.handleSource(ctx, nil, sourceInput{ID: created.ID})
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if src.ContentFormat != "doc" {
		t.Fatalf("content_format = %q, want doc", src.ContentFormat)
	}
	if src.GraphFormat != "json" {
		t.Fatalf("graph_format = %q, want json", src.GraphFormat)
	}
	if len(src.References) != 1 || src.References[0].Title != "repo" {
		t.Fatalf("references not returned: %+v", src.References)
	}

	// The returned source must be directly accepted by present_update.
	var gotDoc render.Doc
	if err := json.Unmarshal([]byte(src.Content), &gotDoc); err != nil {
		t.Fatalf("source content is not Doc JSON: %v", err)
	}
	if gotDoc.Sections[0].Heading != "One" {
		t.Fatalf("doc source lost data: %+v", gotDoc)
	}
	if _, _, err := h.handleUpdate(ctx, nil, updateInput{
		ID: created.ID, Content: &src.Content, Graph: &src.Graph,
	}); err != nil {
		t.Fatalf("round-trip update with source output failed: %v", err)
	}
	_, src2, _ := h.handleSource(ctx, nil, sourceInput{ID: created.ID})
	if src2.ContentFormat != "doc" || src2.GraphFormat != "json" {
		t.Fatalf("formats degraded after round trip: %+v", src2)
	}
}

func TestSourceFallsBackToHTMLAndJS(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	legacyJS := "function getGraphColors(){} function initGraph(){}"
	_, created, _ := h.handleCreate(
		ctx,
		nil,
		createInput{Title: "L", Content: "<p>raw</p>", Graph: legacyJS},
	)

	_, src, err := h.handleSource(ctx, nil, sourceInput{ID: created.ID})
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if src.ContentFormat != "html" || src.Content != "<p>raw</p>" {
		t.Fatalf(
			"legacy content fallback wrong: format=%q content=%q",
			src.ContentFormat,
			src.Content,
		)
	}
	if src.GraphFormat != "js" || src.Graph != legacyJS {
		t.Fatalf("legacy graph fallback wrong: format=%q", src.GraphFormat)
	}
}

func TestSourceNoGraphOmitsGraphFormat(t *testing.T) {
	h, _ := newTestHandlers(t)
	_, created, _ := h.handleCreate(
		context.Background(),
		nil,
		createInput{Title: "T", Content: "<p>x</p>"},
	)
	_, src, err := h.handleSource(context.Background(), nil, sourceInput{ID: created.ID})
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if src.GraphFormat != "" || src.Graph != "" {
		t.Fatalf("graph fields should be empty for graphless page: %+v", src)
	}
}

func TestSourceUnknownIDFails(t *testing.T) {
	h, _ := newTestHandlers(t)
	if _, _, err := h.handleSource(context.Background(), nil, sourceInput{ID: "missing0000"}); !errors.Is(
		err,
		store.ErrNotFound,
	) {
		t.Fatalf("source unknown id: err = %v, want ErrNotFound", err)
	}
}

func TestListIncludesHasDoc(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	doc := map[string]any{
		"sections": []map[string]any{
			{"h": "S", "blocks": []map[string]any{{"t": "p", "text": "x"}}},
		},
	}
	_, docPage, _ := h.handleCreate(ctx, nil, createInput{Title: "Doc", Content: toJSON(doc)})
	_, htmlPage, _ := h.handleCreate(ctx, nil, createInput{Title: "HTML", Content: "<p>x</p>"})

	_, out, err := h.handleList(ctx, nil, struct{}{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byID := map[string]bool{}
	for _, p := range out.Pages {
		byID[p.ID] = p.HasDoc
	}
	if !byID[docPage.ID] {
		t.Error("doc-backed page should report has_doc=true")
	}
	if byID[htmlPage.ID] {
		t.Error("html page should report has_doc=false")
	}
}

func TestWithHintWrapsErrorsOnce(t *testing.T) {
	sentinel := errors.New("boom")
	fail := withHint(func(
		context.Context, *mcp.CallToolRequest, struct{},
	) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, sentinel
	})
	_, _, err := fail(context.Background(), nil, struct{}{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("wrapped error does not unwrap to the handler error: %v", err)
	}
	if want := "run 'present doctor'"; !strings.Contains(err.Error(), want) {
		t.Errorf("err = %q, want it to contain %q", err, want)
	}
	if n := strings.Count(err.Error(), "doctor"); n != 1 {
		t.Errorf("hint applied %d times, want once: %q", n, err)
	}

	ok := withHint(func(
		context.Context, *mcp.CallToolRequest, struct{},
	) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, nil
	})
	if _, _, err := ok(context.Background(), nil, struct{}{}); err != nil {
		t.Fatalf("clean handler returned error: %v", err)
	}
}

// TestToolAnnotationContract holds every registered tool to the repo-wide
// annotation rules: a spec-legal name, a description, an explicit open-world
// hint, and a destructive hint on anything that writes.
func TestToolAnnotationContract(t *testing.T) {
	s, err := New("test", Config{
		Workdir: t.TempDir(),
		Port:    7423,
		Checks:  func(context.Context) []doctor.Check { return nil },
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	mcptest.VerifyToolAnnotations(t, s)
}
