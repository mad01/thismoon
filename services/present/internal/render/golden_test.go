package render

import (
	"os"
	"path/filepath"
	"testing"
)

// goldenDoc is a brief that uses every block and field the renderer knew
// before the layout work (MAD-365): none of the container blocks, stat,
// quote, or deck chrome fields. Its rendered fragment is pinned byte for
// byte in testdata/golden-brief.html, captured from the renderer as it was
// before those were added, so a stored page that never set them re-renders
// to the same bytes and `present rerender` reports it unchanged. MAD-377
// regenerated it when the section id badge went, so it carries no id.
var goldenDoc = Doc{
	Summary: "A stale DNS cache took checkout down for **41 minutes**; see [the brief](https://example.com/brief).",
	Meta:    "2026-09-29 · Incident review · JIRA-42",
	Chips: []Chip{
		{Text: "41 min", Style: "stat"},
		{Text: "payments", Style: "a"},
		{Text: "plain"},
	},
	Sections: []Section{
		{
			Heading: "What happened & when",
			Blocks: []Block{
				{T: "p", Text: "Checkout returned 502s from 14:02 → 14:43."},
				{T: "h3", Text: "Timeline"},
				{T: "list", Items: []string{"14:02 first 502s", "14:09 paged @chip(b:checkout)"}},
				{T: "list", Ordered: true, Items: []string{"restart", "verify"}},
				{T: "callout", Severity: "info", Text: "Other services were fine."},
				{T: "callout", Severity: "warn", Text: "The rota was not staffed."},
				{T: "callout", Text: "A plain callout."},
				{
					T:    "table",
					Cols: []string{"Time", "Event"},
					Rows: [][]string{{"14:02", "first `502`"}, {"14:43", "resolved"}},
				},
				{T: "kv", KV: []KVPair{{K: "TTL", V: "3600 s"}, {K: "Health check", V: "none"}}},
				{T: "panel", Title: "Resolver", Subtitle: "kept a dead upstream", Accent: "blue"},
				{T: "panel", Title: "Bare panel"},
				{T: "progress", Percent: 62.5, Label: "62.5%"},
			},
		},
		{
			Heading: "Where the request died",
			Blocks: []Block{
				{T: "graph"},
				{
					T: "chart", Kind: "area", Title: "5xx per minute", Unit: "req",
					Series: []ChartSeries{
						{
							Name:   "5xx",
							Color:  "terracotta",
							Points: []ChartPoint{{X: "14:00", Y: 0}, {X: "14:15", Y: 940}},
						},
					},
				},
				{
					T:     "chart",
					Kind:  "sankey",
					Flows: []ChartFlow{{From: "edge", To: "api", Value: 12}},
				},
				{T: "code", Lang: "go", Text: "if a < b && c > d {\n\treturn\n}"},
				{T: "html", Text: "<custom>raw</custom>"},
			},
		},
	},
}

// TestRenderDocGolden pins the fragment for a Doc without any of the
// MAD-365 additions. Set PRESENT_UPDATE_GOLDEN=1 to rewrite the fixture
// after a deliberate change to the markup of an existing block.
func TestRenderDocGolden(t *testing.T) {
	out, err := RenderDoc(goldenDoc, "JIRA-42: Checkout outage")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	path := filepath.Join("testdata", "golden-brief.html")
	if os.Getenv("PRESENT_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if out != string(want) {
		t.Errorf(
			"rendered fragment differs from %s; a Doc without the new fields must render byte-identically\n got:\n%s\nwant:\n%s",
			path,
			out,
			want,
		)
	}
}

// TestCompileDocGoldenJSON pins the canonical JSON the store keeps for the
// same Doc: a table's cols must keep its wire key and position after the
// columns block learned to share that key.
func TestCompileDocGoldenJSON(t *testing.T) {
	c, err := CompileDoc(goldenDoc, "T")
	if err != nil {
		t.Fatalf("CompileDoc: %v", err)
	}
	path := filepath.Join("testdata", "golden-doc.json")
	if os.Getenv("PRESENT_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, c.JSON, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if string(c.JSON) != string(want) {
		t.Errorf("canonical JSON differs from %s\n got: %s\nwant: %s", path, c.JSON, want)
	}
}

// goldenStage1Doc adds the stage 1 additions of MAD-365 (the container
// blocks, stat, quote, the ok callout, the deck chrome fields) and none of
// the stage 2 section fields or the transition. Its deck rendition is
// pinned in testdata/golden-stage1.html, captured before stage 2, so a deck
// written against stage 1 re-renders to the same bytes too (the brief
// rendition of the same Doc is the golden above plus the blocks).
var goldenStage1Doc = Doc{
	Summary:      "Stage 1 blocks and chrome.",
	Meta:         "2026-10-01 · golden",
	Presenter:    "Alex, platform",
	Footer:       "Stage 1 golden",
	Logo:         "none",
	LogoPosition: "top-left",
	Progress:     "bar",
	Sections: []Section{
		{
			Heading: "Blocks",
			Blocks: []Block{
				{T: "columns", Columns: [][]Block{
					{{T: "stat", Value: "41 min", Label: "outage", Subtitle: "Tuesday"}},
					{{T: "p", Text: "beside it"}},
				}},
				{T: "quote", Text: "We never saw it.", Cite: "On-call"},
				{
					T:       "details",
					Summary: "Timeline",
					Blocks:  []Block{{T: "list", Items: []string{"14:02", "14:43"}}},
				},
				{T: "callout", Severity: "ok", Text: "Live."},
				{T: "callout", Severity: "error", Text: "Not yet."},
			},
		},
		{Heading: "Second", Blocks: []Block{{T: "p", Text: "x"}}},
	},
}

func TestRenderDocGoldenStage1(t *testing.T) {
	out, err := RenderDeck(goldenStage1Doc, "Stage 1")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	path := filepath.Join("testdata", "golden-stage1.html")
	if os.Getenv("PRESENT_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if out != string(want) {
		t.Errorf(
			"rendered fragment differs from %s; a Doc without the stage 2 fields must render byte-identically\n got:\n%s\nwant:\n%s",
			path,
			out,
			want,
		)
	}
}
