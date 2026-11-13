package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/limits"
	"github.com/mad01/thismoon/services/present/internal/lint"
)

// claimSection is a section the lint passes: a claim heading over one
// short paragraph.
func claimSection(heading string) map[string]any {
	return map[string]any{"h": heading, "blocks": []any{map[string]any{"t": "p", "text": "Short."}}}
}

// labelSection trips heading.label once.
func labelSection(heading string) map[string]any {
	return map[string]any{
		"h":      heading,
		"blocks": []any{map[string]any{"t": "stat", "value": "3", "label": "x"}},
	}
}

func docJSON(t *testing.T, sections ...map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"summary": "A short summary.", "sections": sections})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func rules(fs []lint.Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, string(f.Rule)+" "+f.Where)
	}
	return out
}

// TestCreateWithOneFaultShowsOneWarning is the done check of the lint: a
// create with one fault returns that one warning, in the result's JSON as
// {rule, where, message}, and the page is stored.
func TestCreateWithOneFaultShowsOneWarning(t *testing.T) {
	h, _ := newTestHandlers(t)
	content := docJSON(
		t,
		claimSection("The cache cut p99 latency by 40 percent"),
		labelSection("Results"),
	)
	_, out, err := h.handleCreate(
		context.Background(),
		nil,
		createInput{Title: "T", Content: content},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.Version != 1 {
		t.Errorf("version = %d: a warning must not stop the write", out.Version)
	}
	if got := rules(out.Warnings); len(got) != 1 ||
		got[0] != "heading.label content.sections[1].h" {
		t.Fatalf("warnings = %v, want one heading.label at content.sections[1].h", got)
	}
	var wire map[string]any
	if err := json.Unmarshal([]byte(toJSON(out)), &wire); err != nil {
		t.Fatal(err)
	}
	if _, ok := wire["more"]; ok {
		t.Error("more is set with nothing more to show")
	}
	w := wire["warnings"].([]any)[0].(map[string]any)
	if len(w) != 3 || w["rule"] != "heading.label" ||
		!strings.HasPrefix(w["message"].(string), `section "Results"`) {
		t.Errorf("warning on the wire = %v, want {rule, where, message}", w)
	}
}

func TestCleanAndLegacyPagesHaveNoWarnings(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	clean := docJSON(t, claimSection("The cache cut p99 latency by 40 percent"))
	for name, content := range map[string]string{"clean Doc": clean, "legacy HTML": "<p>``` **x</p>"} {
		_, out, err := h.handleCreate(ctx, nil, createInput{Title: "T", Content: content})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out.Warnings != nil || strings.Contains(toJSON(out), "warnings") {
			t.Errorf("%s: warnings = %v, want the field omitted", name, out.Warnings)
		}
	}
}

// TestCreateCapsTheWarningsItLists: past the cap the result lists the
// first findings in lint order and counts the rest in more.
func TestCreateCapsTheWarningsItLists(t *testing.T) {
	h, _ := newTestHandlers(t)
	var sections []map[string]any
	for i := range warningsShown + 3 {
		sections = append(sections, labelSection(fmt.Sprintf("Label %d", i)))
	}
	_, out, err := h.handleCreate(context.Background(), nil,
		createInput{Title: "T", Content: docJSON(t, sections...)})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Warnings) != warningsShown || out.More != 3 {
		t.Fatalf("listed %d, more %d; want %d and 3", len(out.Warnings), out.More, warningsShown)
	}
	if out.Warnings[0].Where != "content.sections[0].h" {
		t.Errorf("first warning at %s, want the first section", out.Warnings[0].Where)
	}
}

// TestUpdateLintsTheWholeStoredPage: an update that sends only the content
// still reports the deck it left alone, read from the stored source.
func TestUpdateLintsTheWholeStoredPage(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	deck := docJSON(t, labelSection("Overview"))
	_, created, err := h.handleCreate(ctx, nil, createInput{
		Title: "T", Content: docJSON(t, labelSection("Background")), Deck: deck,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := rules(created.Warnings); len(got) != 2 {
		t.Fatalf("create warnings = %v, want the brief's and the deck's", got)
	}
	fixed := docJSON(t, claimSection("The cache cut p99 latency by 40 percent"))
	_, updated, err := h.handleUpdate(ctx, nil, updateInput{ID: created.ID, Content: &fixed})
	if err != nil {
		t.Fatal(err)
	}
	if got := rules(updated.Warnings); len(got) != 1 ||
		got[0] != "heading.label deck.sections[0].h" {
		t.Errorf("update warnings = %v, want only the untouched deck's", got)
	}
}

// TestPatchSectionListsItsOwnWarnings: the patched section's findings come
// back, the rest of the page is a count, and a removal leaves only the
// count.
func TestPatchSectionListsItsOwnWarnings(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	content := docJSON(t,
		labelSection("Overview"),
		claimSection("The cache cut p99 latency by 40 percent"),
		labelSection("Background"),
	)
	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "T", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	section, _ := json.Marshal(map[string]any{"h": "Details", "blocks": []any{
		map[string]any{"t": "p", "text": "an odd ` tick"},
	}})
	_, out, err := h.handlePatchSection(ctx, nil, patchSectionInput{
		ID: created.ID, Op: "replace", Anchor: "index:1", Section: string(section),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"heading.label content.sections[1].h",
		"inline.stray_mark content.sections[1].blocks[0].text",
	}
	if got := rules(out.Warnings); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("patched section warnings = %v, want %v", got, want)
	}
	if out.Elsewhere != 2 || out.More != 0 {
		t.Errorf("elsewhere %d, more %d; want 2 and 0", out.Elsewhere, out.More)
	}

	_, removed, err := h.handlePatchSection(ctx, nil, patchSectionInput{
		ID: created.ID, Op: "remove", Anchor: "index:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if removed.Warnings != nil || removed.Elsewhere != 2 {
		t.Errorf("after remove: warnings %v, elsewhere %d; want none and 2",
			rules(removed.Warnings), removed.Elsewhere)
	}
}

func TestSectionWarningsSplit(t *testing.T) {
	at := func(r lint.Rendition, first, last int) lint.Finding {
		return lint.Finding{Rule: limits.TableRows, Rendition: r, First: first, Last: last}
	}
	findings := []lint.Finding{
		at(lint.Content, -1, -1), // the summary
		at(lint.Content, 0, 3),   // a run covering section 2
		at(lint.Content, 2, 2),
		at(lint.Deck, 2, 2),
	}
	shown, more, elsewhere := sectionWarnings(findings, lint.Content, 2, false)
	if len(shown) != 2 || more != 0 || elsewhere != 2 {
		t.Errorf("shown %d, more %d, elsewhere %d; want 2, 0, 2", len(shown), more, elsewhere)
	}
	if shown[0].First != 0 {
		t.Error("the patched section's findings must keep lint order")
	}
}
