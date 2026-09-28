package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"text/template"
)

// GraphInput is the structured input for a Cytoscape graph. The model passes
// nodes and edges; the server generates the full getGraphColors() + initGraph()
// JS that the template calls on load and theme toggle.
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

var graphTemplate = template.Must(template.New("graph").Parse(graphTemplateSrc))

// The flow dash pattern [10, 6] has period 16; app.js's startGraphFlow marches
// line-dash-offset over that same period, so keep the two in step.
const graphTemplateSrc = `function getGraphColors() {
  var dark = document.documentElement.getAttribute('data-theme') === 'dark';
  return dark
    ? { bg:'#1A1916', centerBg:'#3A2A20', centerBorder:'#E8956A', centerText:'#F5F3EF',
        leafBg:'#252320', leafBorder:'#4A453D', leafText:'#F5F3EF',
        modBorder:['#E8956A','#7AAAE8','#6BC48A','#8B6BB0'],
        edge:'#4A453D', edgeArrow:'#6B6459', hot:'#E8956A',
        registryBg:'#35322C', registryBorder:'#4A453D', registryText:'#B8B2A7' }
    : { bg:'#FFFFFF', centerBg:'#FFF5F0', centerBorder:'#C4704B', centerText:'#252320',
        leafBg:'#FFFFFF', leafBorder:'#D8D4CD', leafText:'#252320',
        modBorder:['#C4704B','#5B8EC4','#4A9E6B','#8B6BB0'],
        edge:'#D8D4CD', edgeArrow:'#B8B2A7', hot:'#C4704B',
        registryBg:'#EEECE8', registryBorder:'#D8D4CD', registryText:'#6B6459' };
}

function initGraph() {
  var c = getGraphColors();
  cytoscape({
    container: document.getElementById('cy-graph'),
    elements: {{.ElementsJSON}},
    style: [
      { selector: 'node', style: {
          'label': 'data(label)', 'text-wrap': 'wrap', 'text-max-width': '120px',
          'font-size': '12px', 'text-valign': 'center', 'text-halign': 'center',
          'width': '140px', 'height': '50px', 'shape': 'roundrectangle',
          'background-color': c.leafBg, 'border-width': 2,
          'border-color': c.leafBorder, 'color': c.leafText
      }},
      { selector: 'node[type="center"]', style: {
          'background-color': c.centerBg, 'border-color': c.centerBorder,
          'border-width': 3, 'color': c.centerText, 'font-weight': 'bold'
      }},
      { selector: 'node[type="registry"]', style: {
          'background-color': c.registryBg, 'border-color': c.registryBorder,
          'border-style': 'dashed', 'color': c.registryText
      }},
{{- range .ModuleStyles}}
      { selector: '{{.Selector}}', style: { 'border-color': c.modBorder[{{.ColorIndex}}] }},
{{- end}}
      { selector: 'edge', style: {
          'width': 2, 'line-color': c.edge, 'target-arrow-color': c.edgeArrow,
          'target-arrow-shape': 'triangle', 'curve-style': 'bezier',
          'arrow-scale': 0.8
      }},
      { selector: 'edge[type="publishes"]', style: { 'line-style': 'dashed' }},
      { selector: 'edge[flow]', style: { 'line-style': 'dashed', 'line-dash-pattern': [10, 6] }},
{{- if .HasWeight}}
      { selector: 'edge[weight]', style: { 'width': 'mapData(weight, 0, {{.MaxWeight}}, 1.5, 7)' }},
      { selector: 'edge[hot]', style: { 'line-color': c.hot, 'target-arrow-color': c.hot }},
{{- end}}
      { selector: 'edge[label]', style: {
          'label': 'data(label)', 'font-size': '10px', 'color': c.leafText,
          'text-background-color': c.bg, 'text-background-opacity': 0.8,
          'text-background-padding': '2px'
      }}
    ],
    layout: {{.LayoutOpts}}
  });
}`

type graphData struct {
	ElementsJSON string
	ModuleStyles []moduleStyle
	LayoutOpts   string
	HasWeight    bool
	MaxWeight    string // JS number literal for the mapData upper bound
}

type moduleStyle struct {
	Selector   string
	ColorIndex int
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

	seen := map[int]bool{}
	var styles []moduleStyle
	for _, n := range g.Nodes {
		if n.Type == "module" && !seen[n.Color] {
			seen[n.Color] = true
			styles = append(styles, moduleStyle{
				Selector:   fmt.Sprintf(`node[type="module"][color="%d"]`, n.Color),
				ColorIndex: n.Color,
			})
		}
	}

	layout, err := layoutOptions(g)
	if err != nil {
		return "", err
	}
	maxW := maxWeight(g.Edges)
	var buf bytes.Buffer
	if err := graphTemplate.Execute(&buf, graphData{
		ElementsJSON: string(elemJSON),
		ModuleStyles: styles,
		LayoutOpts:   layout,
		HasWeight:    maxW > 0,
		MaxWeight:    strconv.FormatFloat(maxW, 'f', -1, 64),
	}); err != nil {
		return "", fmt.Errorf("render graph: %w", err)
	}
	return buf.String(), nil
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
