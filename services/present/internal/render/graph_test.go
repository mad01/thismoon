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
		"function getGraphColors()",
		"function initGraph()",
		"cytoscape(",
		`name: 'dagre'`,
		"nodeDimensionsIncludeLabels: true",
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
		{"auto small graph is LR", GraphInput{Nodes: fewNodes}, `rankDir: 'LR'`},
		{"auto large graph is TB", GraphInput{Nodes: manyNodes}, `rankDir: 'TB'`},
		{
			"explicit TB wins on small graph",
			GraphInput{Nodes: fewNodes, Direction: "TB"},
			`rankDir: 'TB'`,
		},
		{
			"explicit LR wins on large graph",
			GraphInput{Nodes: manyNodes, Direction: "LR"},
			`rankDir: 'LR'`,
		},
		{
			"breadthfirst aliases to dagre",
			GraphInput{Nodes: manyNodes, Layout: "breadthfirst"},
			`name: 'dagre'`,
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
	if !strings.Contains(out, `name: 'cose'`) {
		t.Error("layout not set to cose")
	}
	if !strings.Contains(out, "nodeRepulsion") {
		t.Error("cose layout should have nodeRepulsion parameter")
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
	if !strings.Contains(out, `c.modBorder[0]`) {
		t.Error("module color 0 style not generated")
	}
	if !strings.Contains(out, `c.modBorder[2]`) {
		t.Error("module color 2 style not generated")
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

func TestRenderGraphThemeColors(t *testing.T) {
	g := GraphInput{
		Nodes: []GraphNode{{ID: "a", Label: "A", Type: "center"}},
	}

	out, err := RenderGraph(g)
	if err != nil {
		t.Fatalf("RenderGraph: %v", err)
	}

	darkColors := []string{"#1A1916", "#3A2A20", "#E8956A"}
	lightColors := []string{"#FFFFFF", "#FFF5F0", "#C4704B"}

	for _, c := range darkColors {
		if !strings.Contains(out, c) {
			t.Errorf("dark theme color %s missing", c)
		}
	}
	for _, c := range lightColors {
		if !strings.Contains(out, c) {
			t.Errorf("light theme color %s missing", c)
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
	if !strings.Contains(out, `type="registry"`) {
		t.Error("registry node style selector missing")
	}
}
