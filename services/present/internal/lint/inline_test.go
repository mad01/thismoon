package lint

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/render"
)

// everyFieldPage reads testdata/every-field.json, a Doc that sets every text
// field the walker visits, as both renditions of one page.
func everyFieldPage(t *testing.T) Page {
	t.Helper()
	data, err := os.ReadFile("testdata/every-field.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Title string          `json:"title"`
		Doc   json.RawMessage `json:"doc"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	p := Page{Title: raw.Title}
	for _, d := range []*render.Doc{&p.Brief, &p.Deck} {
		if err := json.Unmarshal(raw.Doc, d); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// TestInlineFieldsMatchTheRenderer holds the walker's inline flags to the
// renderer itself: every visited field gets a marker written as a code
// span, the page is rendered as a brief and a deck, and a field the lint
// treats as inline markdown must come out as <code>, a plain one as the
// marker without it. A field the renderer draws nowhere fails too, so the
// walker cannot name a field the page does not show.
func TestInlineFieldsMatchTheRenderer(t *testing.T) {
	p := everyFieldPage(t)
	var fields []text
	p.texts(func(f text) {
		*f.s = fmt.Sprintf("`zq%dx`", len(fields))
		fields = append(fields, f)
	})
	if len(fields) < 40 {
		t.Fatalf("walked %d fields; every-field.json should reach every kind of field", len(fields))
	}
	brief, err := render.RenderDoc(p.Brief, p.Title)
	if err != nil {
		t.Fatalf("render brief: %v", err)
	}
	deck, err := render.RenderDeck(p.Deck, p.Title)
	if err != nil {
		t.Fatalf("render deck: %v", err)
	}
	out := map[Rendition]string{"": brief, Content: brief, Deck: deck}
	for i, f := range fields {
		marker := fmt.Sprintf("zq%dx", i)
		html := out[f.rend]
		code := strings.Contains(html, "<code>"+marker+"</code>")
		switch {
		case !strings.Contains(html, marker):
			t.Errorf("%s: the renderer draws it nowhere", f.where)
		case f.inline && !code:
			t.Errorf("%s: the lint reads it as inline markdown, the renderer does not", f.where)
		case !f.inline && code:
			t.Errorf("%s: the renderer reads inline markdown in it, the lint does not", f.where)
		}
	}
}
