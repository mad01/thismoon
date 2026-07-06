package web

import (
	"strings"
	"testing"
)

func TestRenderSectionsSplitsAtH2(t *testing.T) {
	src := []byte("intro text\n\n## First\n\nbody one\n\n## Second\n\nbody two\n")
	got, err := RenderSections(src)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(got, `<section class="doc-section">`); n != 3 {
		t.Errorf("section count = %d, want 3 (preamble + two h2 sections)\n%s", n, got)
	}
	if !strings.Contains(got, "<h2>First</h2>") || !strings.Contains(got, "body two") {
		t.Errorf("rendered content missing headings/bodies:\n%s", got)
	}
}

func TestRenderSectionsKeepsH3WithinSection(t *testing.T) {
	src := []byte("## Top\n\n### Sub\n\ntext\n")
	got, err := RenderSections(src)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(got, `<section class="doc-section">`); n != 1 {
		t.Errorf("section count = %d, want 1 (h3 must not split)\n%s", n, got)
	}
}

func TestRenderSectionsWholeDocWithoutHeadings(t *testing.T) {
	got, err := RenderSections([]byte("just a paragraph\n"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(got, `<section class="doc-section">`); n != 1 {
		t.Errorf("section count = %d, want 1\n%s", n, got)
	}
}

func TestRenderSectionsGFMTable(t *testing.T) {
	src := []byte("## T\n\n| a | b |\n|---|---|\n| 1 | 2 |\n")
	got, err := RenderSections(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "<table>") {
		t.Errorf("GFM table not rendered:\n%s", got)
	}
}
