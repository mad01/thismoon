package mcpserver

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/render"
)

// TestChartKindsReachTheSchema pins the create tool's content description to
// render.ChartKinds, written there as kind: a|b|c, so a kind added or
// removed in Go reaches the agents reading the schema.
func TestChartKindsReachTheSchema(t *testing.T) {
	f, ok := reflect.TypeOf(createInput{}).FieldByName("Content")
	if !ok {
		t.Fatal("createInput has no Content field")
	}
	want := "kind: " + strings.Join(render.ChartKinds(), "|")
	if !strings.Contains(f.Tag.Get("jsonschema"), want) {
		t.Errorf("content schema lacks %q", want)
	}
}

// TestDiagramVocabularyReachesTheSchema pins the same description to
// render.DiagramKinds and render.DiagramDirections, written as node kind:
// a|b and direction: a|b so neither can match the chart's kind list.
func TestDiagramVocabularyReachesTheSchema(t *testing.T) {
	f, ok := reflect.TypeOf(createInput{}).FieldByName("Content")
	if !ok {
		t.Fatal("createInput has no Content field")
	}
	for _, want := range []string{
		"node kind: " + strings.Join(render.DiagramKinds(), "|"),
		"direction: " + strings.Join(render.DiagramDirections(), "|"),
	} {
		if !strings.Contains(f.Tag.Get("jsonschema"), want) {
			t.Errorf("content schema lacks %q", want)
		}
	}
}

// TestDeckFieldsReachTheSchema pins the create tool's deck description to
// the deck's chrome and per-slide field names, now that the shared deck
// text defers to it, and the update tool's deck description to the create
// tool's, so neither can lose a field without a test noticing.
func TestDeckFieldsReachTheSchema(t *testing.T) {
	create, ok := reflect.TypeOf(createInput{}).FieldByName("Deck")
	if !ok {
		t.Fatal("createInput has no Deck field")
	}
	tag := create.Tag.Get("jsonschema")
	for _, want := range []string{
		"logo", "logo_position", "progress", "presenter", "footer", "transition",
		"layout", "tone", "notes", "reveal",
	} {
		if !strings.Contains(tag, want) {
			t.Errorf("create deck schema lacks %q", want)
		}
	}
	update, ok := reflect.TypeOf(updateInput{}).FieldByName("Deck")
	if !ok {
		t.Fatal("updateInput has no Deck field")
	}
	if !strings.Contains(update.Tag.Get("jsonschema"), "present_create") {
		t.Error("update deck schema must point at present_create's deck")
	}
}

// TestTerminalBlockReachesTheSchema pins the create tool's descriptions to
// the terminal block: its fields, what prefill does, the session length,
// and the rows bounds the renderer enforces in the content text, and its
// exemption from the no-cards rule in the deck text.
func TestTerminalBlockReachesTheSchema(t *testing.T) {
	content, ok := reflect.TypeOf(createInput{}).FieldByName("Content")
	if !ok {
		t.Fatal("createInput has no Content field")
	}
	for _, want := range []string{
		"terminal {session, cmd?, cwd?, prefill?, rows?}",
		"prefill is typed into the session on start, not run",
		fmt.Sprintf("session is 1 to %d of a-z, 0-9, and -", render.TerminalSessionMaxLen),
		fmt.Sprintf("rows %d to %d", render.TerminalMinRows, render.TerminalMaxRows),
	} {
		if !strings.Contains(content.Tag.Get("jsonschema"), want) {
			t.Errorf("content schema lacks %q", want)
		}
	}
	deck, ok := reflect.TypeOf(createInput{}).FieldByName("Deck")
	if !ok {
		t.Fatal("createInput has no Deck field")
	}
	want := "a terminal block is the one card a slide may keep"
	if !strings.Contains(deck.Tag.Get("jsonschema"), want) {
		t.Errorf("deck schema lacks %q", want)
	}
}
