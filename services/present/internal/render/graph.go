package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// GraphInput is the structured input for a Cytoscape graph. The model passes
// nodes and edges; the server generates a data-only initGraph() that app.js
// completes at load with the current style (presentGraphStyle) and layout
// (presentGraphLayout), so a palette or box change reaches every stored graph
// on its next load without a rerender.
type GraphInput struct {
	Nodes     []GraphNode `json:"nodes"`
	Edges     []GraphEdge `json:"edges,omitempty"`
	Layout    string      `json:"layout,omitempty"`    // a layoutEngines key: dagre (default), cose, elk, elk-<algorithm>
	Direction string      `json:"direction,omitempty"` // TB or LR (dagre and ELK layered); empty = auto: LR for small graphs, TB otherwise
}

// GraphNode is a node in the graph.
type GraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type,omitempty"`  // center, module, leaf (default), registry
	Color int    `json:"color,omitempty"` // index into modBorder[] for type=module (0-3)
	Tone  string `json:"tone,omitempty"`  // a graphTones key: box background, border, and text as one family, over the type's colors
}

// GraphEdge is an edge in the graph.
type GraphEdge struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Type   string  `json:"type,omitempty"`   // consumes (solid, default), publishes (dashed)
	Label  string  `json:"label,omitempty"`  // optional edge label
	Weight float64 `json:"weight,omitempty"` // traffic volume: drives line width; the top third of the range is tinted
	Flow   bool    `json:"flow,omitempty"`   // animate dashes from source to target (app.js drives the dash offset)
}

// hotWeightShare is the fraction of the heaviest edge's weight at or above
// which an edge counts as hot and gets the accent tint.
const hotWeightShare = 2.0 / 3.0

// graphTones are the node tones a graph may name. Each is a background,
// border, and text triple app.js's getGraphColors defines for both themes, so
// a tinted box keeps its contrast when the reader flips to dark. Keep the list
// in step with GRAPH_TONES in app.js.
var graphTones = []string{"neutral", "green", "red", "blue", "amber", "purple"}

func validTone(tone string) bool {
	for _, t := range graphTones {
		if t == tone {
			return true
		}
	}
	return false
}

var graphTemplate = template.Must(template.New("graph").Parse(graphTemplateSrc))

// The script carries the elements and names the engine and direction. The
// cytoscape() shim in app.js fills in the style, so a page rendered by an
// older template (with its own inline style) is restyled too when its layout
// came from presentGraphLayout.
const graphTemplateSrc = `function initGraph() {
  cytoscape({
    container: document.getElementById('cy-graph'),
    elements: {{.ElementsJSON}},
    layout: {{.LayoutOpts}}
  });
}`

type graphData struct {
	ElementsJSON string
	LayoutOpts   string
}

// RenderGraph converts a GraphInput to a complete JS script string.
func RenderGraph(g GraphInput) (string, error) {
	if len(g.Nodes) == 0 {
		return "", fmt.Errorf("graph has no nodes")
	}

	elements := buildElements(g)
	elemJSON, err := json.Marshal(elements)
	if err != nil {
		return "", fmt.Errorf("marshal elements: %w", err)
	}

	if err := validateTones(g.Nodes); err != nil {
		return "", err
	}
	layout, err := layoutOptions(g)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := graphTemplate.Execute(&buf, graphData{
		ElementsJSON: string(elemJSON),
		LayoutOpts:   layout,
	}); err != nil {
		return "", fmt.Errorf("render graph: %w", err)
	}
	return buf.String(), nil
}

// validateTones returns an error naming the first tone the palette does not
// define; the selectors themselves live in app.js.
func validateTones(nodes []GraphNode) error {
	for _, n := range nodes {
		if n.Tone != "" && !validTone(n.Tone) {
			return fmt.Errorf(
				"node %q: unknown tone %q (want one of %s)",
				n.ID, n.Tone, strings.Join(graphTones, ", "),
			)
		}
	}
	return nil
}

// lrNodeThreshold is the node count at or below which an unset direction
// auto-flips to left-to-right, filling the wide graph container.
const lrNodeThreshold = 8

// layoutEngines maps the layout a graph names to the engine app.js builds
// options for. dagre is the default, and "breadthfirst" stays as an alias
// for pages whose stored graph.json predates the switch to it. "elk" is ELK
// layered with wrapping, which folds a long chain of layers into rows until
// the drawing approaches the container's aspect ratio; the elk-<algorithm>
// forms pick another ELK algorithm without wrapping.
var layoutEngines = map[string]string{
	"":             "dagre",
	"dagre":        "dagre",
	"breadthfirst": "dagre",
	"cose":         "cose",
	"elk":          "elk",
	"elk-layered":  "elk-layered",
	"elk-mrtree":   "elk-mrtree",
	"elk-stress":   "elk-stress",
	"elk-radial":   "elk-radial",
	"elk-force":    "elk-force",
}

// layoutOptions returns the JS expression the template puts in the layout
// slot: a call to app.js's presentGraphLayout with the engine and the
// resolved direction. The option objects live in app.js rather than here
// because the ELK ones take the container's aspect ratio at run time, and
// because the page's engine control rebuilds them for any engine on any
// page.
func layoutOptions(g GraphInput) (string, error) {
	engine, ok := layoutEngines[g.Layout]
	if !ok {
		return "", fmt.Errorf(
			"unknown layout %q (want dagre, cose, elk, or elk-<layered|mrtree|stress|radial|force>)",
			g.Layout,
		)
	}
	return fmt.Sprintf("presentGraphLayout('%s', '%s')", engine, layoutDirection(g)), nil
}

// layoutDirection resolves the direction dagre and ELK layered draw in: the
// explicit one, else LR for small graphs, which fills the wide container,
// and TB otherwise.
func layoutDirection(g GraphInput) string {
	if g.Direction == "TB" || g.Direction == "LR" {
		return g.Direction
	}
	if len(g.Nodes) <= lrNodeThreshold {
		return "LR"
	}
	return "TB"
}

type cyElement struct {
	Data map[string]any `json:"data"`
}

// maxWeight returns the largest positive edge weight, or 0 when no edge
// carries one.
func maxWeight(edges []GraphEdge) float64 {
	var maxW float64
	for _, e := range edges {
		if e.Weight > maxW {
			maxW = e.Weight
		}
	}
	return maxW
}

func buildElements(g GraphInput) []cyElement {
	elems := make([]cyElement, 0, len(g.Nodes)+len(g.Edges))
	for _, n := range g.Nodes {
		d := map[string]any{"id": n.ID, "label": n.Label}
		if n.Type != "" {
			d["type"] = n.Type
		}
		if n.Type == "module" {
			d["color"] = fmt.Sprintf("%d", n.Color)
		}
		if n.Tone != "" {
			d["tone"] = n.Tone
		}
		elems = append(elems, cyElement{Data: d})
	}
	maxW := maxWeight(g.Edges)
	for _, e := range g.Edges {
		d := map[string]any{"source": e.From, "target": e.To}
		if e.Type != "" {
			d["type"] = e.Type
		}
		if e.Label != "" {
			d["label"] = e.Label
		}
		if e.Weight > 0 {
			d["weight"] = e.Weight
			if e.Weight >= maxW*hotWeightShare {
				d["hot"] = 1
			}
		}
		if e.Flow {
			d["flow"] = 1
		}
		elems = append(elems, cyElement{Data: d})
	}
	return elems
}
