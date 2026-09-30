package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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
// border, and text triple the template defines for both themes, so a tinted
// box keeps its contrast when the reader flips to dark. The order is the
// order the palette lists them in.
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

// The flow dash pattern [10, 6] has period 16; app.js's startGraphFlow marches
// line-dash-offset over that same period, so keep the two in step.
const graphTemplateSrc = `function getGraphColors() {
  var dark = document.documentElement.getAttribute('data-theme') === 'dark';
  return dark
    ? { bg:'#1A1916', centerBg:'#3A2A20', centerBorder:'#E8956A', centerText:'#F5F3EF',
        leafBg:'#252320', leafBorder:'#4A453D', leafText:'#F5F3EF',
        modBorder:['#E8956A','#7AAAE8','#6BC48A','#8B6BB0'],
        edge:'#4A453D', edgeArrow:'#6B6459', hot:'#E8956A',
        registryBg:'#35322C', registryBorder:'#4A453D', registryText:'#B8B2A7',
        tones: {
          neutral: { bg:'#35322C', border:'#5A544B', text:'#E6E1D8' },
          green:   { bg:'#1F3A2B', border:'#6BC48A', text:'#C8EBD5' },
          red:     { bg:'#3F2A22', border:'#E8956A', text:'#F3CDBB' },
          blue:    { bg:'#1F2E42', border:'#7AAAE8', text:'#C8DAF3' },
          amber:   { bg:'#3E3418', border:'#D9AE55', text:'#F0DEB0' },
          purple:  { bg:'#32283F', border:'#A98BCB', text:'#DDD0EA' } } }
    : { bg:'#FFFFFF', centerBg:'#FFF5F0', centerBorder:'#C4704B', centerText:'#252320',
        leafBg:'#FFFFFF', leafBorder:'#D8D4CD', leafText:'#252320',
        modBorder:['#C4704B','#5B8EC4','#4A9E6B','#8B6BB0'],
        edge:'#D8D4CD', edgeArrow:'#B8B2A7', hot:'#C4704B',
        registryBg:'#EEECE8', registryBorder:'#D8D4CD', registryText:'#6B6459',
        tones: {
          neutral: { bg:'#EEECE8', border:'#C9C4BB', text:'#3D3A34' },
          green:   { bg:'#E6F4EC', border:'#4A9E6B', text:'#1F5C38' },
          red:     { bg:'#FBEAE3', border:'#C4704B', text:'#7A3A1F' },
          blue:    { bg:'#E7EFF9', border:'#5B8EC4', text:'#2A4E75' },
          amber:   { bg:'#FBF1DC', border:'#C9973A', text:'#6B4E12' },
          purple:  { bg:'#EFE8F5', border:'#8B6BB0', text:'#4E3A6B' } } };
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
          // The height follows the wrapped label so a long name grows the
          // box instead of spilling past it; the width stays fixed so the
          // layouts keep their even columns. Body plus padding is 144px wide.
          'width': '120px', 'height': 'label', 'padding': '12px', 'shape': 'roundrectangle',
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
{{- range .Tones}}
      { selector: 'node[tone="{{.}}"]', style: {
          'background-color': c.tones.{{.}}.bg, 'border-color': c.tones.{{.}}.border, 'color': c.tones.{{.}}.text
      }},
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
          'text-wrap': 'wrap', 'text-max-width': '100px',
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
	Tones        []string // the tones the graph uses, in palette order
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

	tones, err := usedTones(g.Nodes)
	if err != nil {
		return "", err
	}
	layout, err := layoutOptions(g)
	if err != nil {
		return "", err
	}
	maxW := maxWeight(g.Edges)
	var buf bytes.Buffer
	if err := graphTemplate.Execute(&buf, graphData{
		ElementsJSON: string(elemJSON),
		ModuleStyles: moduleStyles(g.Nodes),
		Tones:        tones,
		LayoutOpts:   layout,
		HasWeight:    maxW > 0,
		MaxWeight:    strconv.FormatFloat(maxW, 'f', -1, 64),
	}); err != nil {
		return "", fmt.Errorf("render graph: %w", err)
	}
	return buf.String(), nil
}

// moduleStyles returns one border-color selector per module color the graph
// uses, in first-use order.
func moduleStyles(nodes []GraphNode) []moduleStyle {
	seen := map[int]bool{}
	var styles []moduleStyle
	for _, n := range nodes {
		if n.Type == "module" && !seen[n.Color] {
			seen[n.Color] = true
			styles = append(styles, moduleStyle{
				Selector:   fmt.Sprintf(`node[type="module"][color="%d"]`, n.Color),
				ColorIndex: n.Color,
			})
		}
	}
	return styles
}

// usedTones returns the tones the nodes name, in palette order, or an error
// naming the first tone the palette does not define.
func usedTones(nodes []GraphNode) ([]string, error) {
	used := map[string]bool{}
	for _, n := range nodes {
		if n.Tone == "" {
			continue
		}
		if !validTone(n.Tone) {
			return nil, fmt.Errorf(
				"node %q: unknown tone %q (want one of %s)",
				n.ID,
				n.Tone,
				strings.Join(graphTones, ", "),
			)
		}
		used[n.Tone] = true
	}
	var tones []string
	for _, t := range graphTones {
		if used[t] {
			tones = append(tones, t)
		}
	}
	return tones, nil
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
