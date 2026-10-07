package mcpserver

import (
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
