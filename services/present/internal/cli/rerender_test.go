package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// seedLegacyPage writes a page directly with legacy HTML content and no
// doc.json, simulating a page created before Doc persistence existed.
func seedLegacyPage(t *testing.T, st store.Store, legacyHTML string) string {
	t.Helper()
	p, err := st.Create(t.Context(), store.Draft{Title: "Legacy", Content: legacyHTML})
	if err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	return p.ID
}

func TestRerenderUpgradesLegacyAndReRendersDoc(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	// Legacy page: stored HTML with old block classes, no doc.json.
	legacyID := seedLegacyPage(t, st,
		`<div class="section" id="s"><h2 class="section-heading">H</h2>`+
			`<div class="callout callout-info">note</div>`+
			`<span class="chip chip-a">x</span></div>`)

	// Doc page: persisted doc.json so rerender re-renders from source.
	docID := seedLegacyPage(t, st, "<p>placeholder content from before a render change</p>")
	docJSON := []byte(`{"sections":[{"h":"FromDoc","blocks":[{"t":"p","text":"re-rendered"}]}]}`)
	if err := st.SaveDoc(t.Context(), docID, docJSON); err != nil {
		t.Fatalf("SaveDoc: %v", err)
	}

	legacyBefore, _ := st.Get(t.Context(), legacyID)
	docBefore, _ := st.Get(t.Context(), docID)

	var out bytes.Buffer
	if err := rerenderPages(t.Context(), st, nil, &out); err != nil {
		t.Fatalf("rerenderPages: %v", err)
	}

	// Legacy page: classes gone, wk-* present, version bumped.
	legacyAfter, _ := st.Get(t.Context(), legacyID)
	if strings.Contains(legacyAfter.Content, `class="section"`) ||
		strings.Contains(legacyAfter.Content, `class="callout`) ||
		strings.Contains(legacyAfter.Content, `class="chip`) {
		t.Errorf("legacy classes still present after rerender:\n%s", legacyAfter.Content)
	}
	if !strings.Contains(legacyAfter.Content, "<wk-section") ||
		!strings.Contains(legacyAfter.Content, "<wk-callout") ||
		!strings.Contains(legacyAfter.Content, "<wk-badge") {
		t.Errorf("wk-* tags missing after legacy upgrade:\n%s", legacyAfter.Content)
	}
	if legacyAfter.Version <= legacyBefore.Version {
		t.Errorf("legacy version not bumped: %d -> %d", legacyBefore.Version, legacyAfter.Version)
	}

	// Doc page: re-rendered from doc.json (new heading appears), version bumped.
	docAfter, _ := st.Get(t.Context(), docID)
	if !strings.Contains(docAfter.Content, "FromDoc") ||
		!strings.Contains(docAfter.Content, "re-rendered") {
		t.Errorf("doc page not re-rendered from source:\n%s", docAfter.Content)
	}
	if strings.Contains(docAfter.Content, "placeholder content") {
		t.Errorf("old placeholder content survived a doc re-render:\n%s", docAfter.Content)
	}
	if docAfter.Version <= docBefore.Version {
		t.Errorf("doc version not bumped: %d -> %d", docBefore.Version, docAfter.Version)
	}

	// Summary mentions both outcomes.
	summary := out.String()
	if !strings.Contains(summary, "re-rendered-from-doc") {
		t.Errorf("summary missing re-rendered-from-doc:\n%s", summary)
	}
	if !strings.Contains(summary, "upgraded-legacy-html") {
		t.Errorf("summary missing upgraded-legacy-html:\n%s", summary)
	}
}

func TestRerenderUnchangedReportsUnchanged(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	// Already-migrated content with no legacy classes; upgrade is a no-op.
	// Seed in serializer-normalized form (the upgrader parses then re-serializes,
	// so bare boolean attributes like data-fixation must already be data-fixation="").
	clean := render.UpgradeLegacyHTML(`<wk-section id="x"><p data-fixation>clean</p></wk-section>`)
	id := seedLegacyPage(t, st, clean)
	before, _ := st.Get(t.Context(), id)

	var out bytes.Buffer
	if err := rerenderPages(t.Context(), st, nil, &out); err != nil {
		t.Fatalf("rerenderPages: %v", err)
	}
	after, _ := st.Get(t.Context(), id)
	if after.Version != before.Version {
		t.Errorf("unchanged page version bumped: %d -> %d", before.Version, after.Version)
	}
	if !strings.Contains(out.String(), "unchanged") {
		t.Errorf("summary missing unchanged:\n%s", out.String())
	}
}

func TestRerenderSpecificIDs(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.NewFS(dir)
	a := seedLegacyPage(t, st, `<div class="callout callout-warn">a</div>`)
	b := seedLegacyPage(t, st, `<div class="callout callout-warn">b</div>`)

	bBefore, _ := st.Get(t.Context(), b)
	var out bytes.Buffer
	if err := rerenderPages(t.Context(), st, []string{a}, &out); err != nil {
		t.Fatalf("rerenderPages: %v", err)
	}
	// Only a was targeted; b is untouched.
	bAfter, _ := st.Get(t.Context(), b)
	if bAfter.Version != bBefore.Version {
		t.Errorf("non-targeted page b was modified: %d -> %d", bBefore.Version, bAfter.Version)
	}
	aAfter, _ := st.Get(t.Context(), a)
	if !strings.Contains(aAfter.Content, "<wk-callout") {
		t.Errorf("targeted page a not upgraded:\n%s", aAfter.Content)
	}
}

// TestRerenderContentFileOnDisk confirms the rewrite lands in content.html.
func TestRerenderContentFileOnDisk(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.NewFS(dir)
	id := seedLegacyPage(t, st, `<div class="panel"><div class="panel-title">P</div></div>`)

	var out bytes.Buffer
	if err := rerenderPages(t.Context(), st, nil, &out); err != nil {
		t.Fatalf("rerenderPages: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "pages", id, "content.html"))
	if err != nil {
		t.Fatalf("read content.html: %v", err)
	}
	if !strings.Contains(string(raw), "<wk-panel>") {
		t.Errorf("content.html not upgraded on disk:\n%s", raw)
	}
}

func TestRerenderReRendersGraphFromSource(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.NewFS(dir)

	// Page whose graph.js is stale (placeholder) but has a graph source.
	p, err := st.Create(
		t.Context(),
		store.Draft{Title: "G", Content: "<p>clean</p>", Graph: "/* stale generated js */"},
	)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := st.SaveGraphSource(t.Context(), p.ID, []byte(`{"nodes":[{"id":"a","label":"NodeA"}]}`)); err != nil {
		t.Fatalf("SaveGraphSource: %v", err)
	}

	var out bytes.Buffer
	if err := rerenderPages(t.Context(), st, nil, &out); err != nil {
		t.Fatalf("rerenderPages: %v", err)
	}

	after, _ := st.Get(t.Context(), p.ID)
	if !strings.Contains(after.Graph, "NodeA") || !strings.Contains(after.Graph, "initGraph") {
		t.Errorf("graph not re-rendered from source:\n%s", after.Graph)
	}
	if after.Version <= p.Version {
		t.Errorf("version not bumped: %d -> %d", p.Version, after.Version)
	}
	if !strings.Contains(out.String(), "re-rendered-graph") {
		t.Errorf("summary missing re-rendered-graph:\n%s", out.String())
	}
}

func TestRerenderLeavesLegacyJSGraphAlone(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.NewFS(dir)

	legacyJS := "function getGraphColors(){} function initGraph(){}"
	p, _ := st.Create(
		t.Context(),
		store.Draft{Title: "G", Content: "<p>clean</p>", Graph: legacyJS},
	)

	var out bytes.Buffer
	if err := rerenderPages(t.Context(), st, nil, &out); err != nil {
		t.Fatalf("rerenderPages: %v", err)
	}
	after, _ := st.Get(t.Context(), p.ID)
	if after.Graph != legacyJS {
		t.Errorf("legacy JS graph was modified:\n%s", after.Graph)
	}
	if after.Version != p.Version {
		t.Errorf("version bumped for unchanged page: %d -> %d", p.Version, after.Version)
	}
}
