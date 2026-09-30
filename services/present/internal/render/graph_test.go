package render

import (
	"fmt"
	"strings"
	"testing"
)

func TestRenderGraphMinimal(t *testing.T) {
	g := GraphInput{
		Nodes: []GraphNode{
			{ID: "a", Label: "Node A", Type: "center"},
			{ID: "b", Label: "Node B"},
		},
		Edges: []GraphEdge{
			{From: "a", To: "b"},
		},
	}

	out, err := RenderGraph(g)
	if err != nil {
		t.Fatalf("RenderGraph: %v", err)
	}

	checks := []string{
		"function initGraph()",
		"cytoscape(",
		`layout: presentGraphLayout('dagre', 'LR')`,
		"Node A",
		"Node B",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in graph output", want)
		}
	}
}

func TestRenderGraphDirection(t *testing.T) {
	manyNodes := make([]GraphNode, lrNodeThreshold+1)
	for i := range manyNodes {
		manyNodes[i] = GraphNode{ID: fmt.Sprintf("n%d", i), Label: "N"}
	}
	fewNodes := manyNodes[:2]

	cases := []struct {
		name string
		g    GraphInput
		want string
	}{
		{
			"auto small graph is LR",
			GraphInput{Nodes: fewNodes},
			`presentGraphLayout('dagre', 'LR')`,
		},
		{
			"auto large graph is TB",
			GraphInput{Nodes: manyNodes},
			`presentGraphLayout('dagre', 'TB')`,
		},
		{
			"explicit TB wins on small graph",
			GraphInput{Nodes: fewNodes, Direction: "TB"},
			`presentGraphLayout('dagre', 'TB')`,
		},
		{
			"explicit LR wins on large graph",
			GraphInput{Nodes: manyNodes, Direction: "LR"},
			`presentGraphLayout('dagre', 'LR')`,
		},
		{
			"breadthfirst aliases to dagre",
			GraphInput{Nodes: manyNodes, Layout: "breadthfirst"},
			`presentGraphLayout('dagre', 'TB')`,
		},
		{
			"elk keeps the resolved direction",
			GraphInput{Nodes: manyNodes, Layout: "elk"},
			`presentGraphLayout('elk', 'TB')`,
		},
		{
			"elk algorithm passes through",
			GraphInput{Nodes: fewNodes, Layout: "elk-mrtree", Direction: "TB"},
			`presentGraphLayout('elk-mrtree', 'TB')`,
		},
	}
	for _, tc := range cases {
		out, err := RenderGraph(tc.g)
		if err != nil {
			t.Fatalf("%s: RenderGraph: %v", tc.name, err)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: missing %q", tc.name, tc.want)
		}
	}
}

func TestRenderGraphCoseLayout(t *testing.T) {
	g := GraphInput{
		Nodes:  []GraphNode{{ID: "a", Label: "A"}},
		Layout: "cose",
	}

	out, err := RenderGraph(g)
	if err != nil {
		t.Fatalf("RenderGraph: %v", err)
	}
	if !strings.Contains(out, `presentGraphLayout('cose', 'LR')`) {
		t.Error("layout not set to cose")
	}
}

func TestRenderGraphUnknownLayoutFails(t *testing.T) {
	for _, layout := range []string{"elk-bogus", "klay", "ELK"} {
		_, err := RenderGraph(GraphInput{
			Nodes:  []GraphNode{{ID: "a", Label: "A"}},
			Layout: layout,
		})
		if err == nil || !strings.Contains(err.Error(), layout) {
			t.Errorf("layout %q: err = %v, want unknown-layout error naming it", layout, err)
		}
	}
}

func TestRenderGraphModuleNodes(t *testing.T) {
	g := GraphInput{
		Nodes: []GraphNode{
			{ID: "m0", Label: "Mod0", Type: "module", Color: 0},
			{ID: "m1", Label: "Mod1", Type: "module", Color: 2},
		},
	}

	out, err := RenderGraph(g)
	if err != nil {
		t.Fatalf("RenderGraph: %v", err)
	}
	for _, want := range []string{
		`{"data":{"color":"0","id":"m0","label":"Mod0","type":"module"}}`,
		`{"data":{"color":"2","id":"m1","label":"Mod1","type":"module"}}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in graph output", want)
		}
	}
}

func TestRenderGraphEdgeTypes(t *testing.T) {
	g := GraphInput{
		Nodes: []GraphNode{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B"},
			{ID: "c", Label: "C"},
		},
		Edges: []GraphEdge{
			{From: "a", To: "b", Type: "publishes"},
			{From: "a", To: "c", Type: "consumes", Label: "data"},
		},
	}

	out, err := RenderGraph(g)
	if err != nil {
		t.Fatalf("RenderGraph: %v", err)
	}
	if !strings.Contains(out, "publishes") {
		t.Error("publishes edge type missing")
	}
	if !strings.Contains(out, "data") {
		t.Error("edge label missing")
	}
}

// TestRenderGraphIsDataOnly pins that the script carries no style: app.js's
// presentGraphStyle supplies it at load, so a palette change needs no
// rerender of stored pages.
func TestRenderGraphIsDataOnly(t *testing.T) {
	g := GraphInput{
		Nodes: []GraphNode{
			{ID: "a", Label: "A", Type: "center", Tone: "green"},
			{ID: "b", Label: "B", Type: "module", Color: 1},
		},
		Edges: []GraphEdge{{From: "a", To: "b", Weight: 3, Flow: true, Label: "x"}},
	}

	out, err := RenderGraph(g)
	if err != nil {
		t.Fatalf("RenderGraph: %v", err)
	}
	for _, banned := range []string{"style:", "getGraphColors", "#", "background-color", "mapData("} {
		if strings.Contains(out, banned) {
			t.Errorf("graph output carries %q, expected data only:\n%s", banned, out)
		}
	}
	for _, want := range []string{"function initGraph()", "elements: [", "layout: presentGraphLayout("} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in graph output", want)
		}
	}
}

func TestRenderGraphNoNodesFails(t *testing.T) {
	_, err := RenderGraph(GraphInput{})
	if err == nil {
		t.Fatal("expected error for empty graph")
	}
}

func TestRenderGraphRegistryNode(t *testing.T) {
	g := GraphInput{
		Nodes: []GraphNode{{ID: "r", Label: "Registry", Type: "registry"}},
	}

	out, err := RenderGraph(g)
	if err != nil {
		t.Fatalf("RenderGraph: %v", err)
	}
	if !strings.Contains(out, `{"data":{"id":"r","label":"Registry","type":"registry"}}`) {
		t.Error("registry node data missing")
	}
}

func TestRenderGraphFlowAndWeight(t *testing.T) {
	g := GraphInput{
		Nodes: []GraphNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}, {ID: "c", Label: "C"}},
		Edges: []GraphEdge{
			{From: "a", To: "b", Weight: 1200, Flow: true},
			{From: "b", To: "c", Weight: 150},
		},
	}

	out, err := RenderGraph(g)
	if err != nil {
		t.Fatalf("RenderGraph: %v", err)
	}
	checks := []string{
		// the heaviest edge is hot and flows; the light one is neither
		`{"data":{"flow":1,"hot":1,"source":"a","target":"b","weight":1200}}`,
		`{"data":{"source":"b","target":"c","weight":150}}`,
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in graph output", want)
		}
	}
}

func TestRenderGraphNoWeightSkipsWidthMapping(t *testing.T) {
	g := GraphInput{
		Nodes: []GraphNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		Edges: []GraphEdge{{From: "a", To: "b", Flow: true}},
	}

	out, err := RenderGraph(g)
	if err != nil {
		t.Fatalf("RenderGraph: %v", err)
	}
	if strings.Contains(out, `"hot"`) {
		t.Error("hot tint emitted for a graph without weights")
	}
	if !strings.Contains(out, `{"data":{"flow":1,"source":"a","target":"b"}}`) {
		t.Error("flow edge missing")
	}
}

func TestRenderGraphTones(t *testing.T) {
	g := GraphInput{
		Nodes: []GraphNode{
			{ID: "a", Label: "Doom", Tone: "neutral"},
			{ID: "b", Label: "Laya decides", Type: "center", Tone: "green"},
			{ID: "d", Label: "Plain"},
		},
	}

	out, err := RenderGraph(g)
	if err != nil {
		t.Fatalf("RenderGraph: %v", err)
	}
	for _, want := range []string{
		`{"data":{"id":"a","label":"Doom","tone":"neutral"}}`,
		`{"data":{"id":"b","label":"Laya decides","tone":"green","type":"center"}}`,
		`{"data":{"id":"d","label":"Plain"}}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in graph output", want)
		}
	}
}

func TestRenderGraphUnknownToneFails(t *testing.T) {
	g := GraphInput{Nodes: []GraphNode{{ID: "a", Label: "A", Tone: "teal"}}}
	_, err := RenderGraph(g)
	if err == nil || !strings.Contains(err.Error(), `unknown tone "teal"`) {
		t.Fatalf("want unknown tone error, got %v", err)
	}
}
