package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Hop limits of /api/graph's depth.
const (
	minGraphDepth     = 1
	maxGraphDepth     = 5
	defaultGraphDepth = 1
)

// graphNode is vault.GraphNode plus what only serve knows: whether the note
// is in the stale queue, and its cached layout position.
type graphNode struct {
	vault.GraphNode
	Stale bool `json:"stale"`
	// X and Y are nil when the layout has no position for the node, so a
	// position of 0 survives and an unknown one is omitted.
	X *float64 `json:"x,omitempty"`
	Y *float64 `json:"y,omitempty"`
}

type graphView struct {
	Key    string            `json:"key"`
	Layout layoutState       `json:"layout"`
	Nodes  []graphNode       `json:"nodes"`
	Edges  []vault.GraphEdge `json:"edges"`
}

// layoutState says whether the positions are the layout of this key.
// Cached is false when the positions come from an older layout, or there
// are none.
type layoutState struct {
	Cached bool `json:"cached"`
}

// graphParams is /api/graph's query, parsed.
type graphParams struct {
	vault  string
	center string
	depth  int
	topics bool
}

func parseGraph(r *http.Request) (graphParams, error) {
	q := r.URL.Query()
	p := graphParams{vault: q.Get("vault"), center: strings.TrimSpace(q.Get("center"))}
	p.depth = defaultGraphDepth
	if d := q.Get("depth"); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil || n < minGraphDepth || n > maxGraphDepth {
			return p, badRequest("depth must be %d to %d, got %q", minGraphDepth, maxGraphDepth, d)
		}
		p.depth = n
	}
	var err error
	p.topics, err = flag01(q.Get("topics"), "topics")
	return p, err
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	p, err := parseGraph(r)
	if err != nil {
		writeError(w, err)
		return
	}
	m, err := s.current()
	if err != nil {
		writeError(w, err)
		return
	}
	view, err := s.graphView(m, p)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// graphView builds the graph p asks for and dresses its nodes with the
// stale flag and the cached layout.
func (s *Server) graphView(m *model, p graphParams) (graphView, error) {
	opts := vault.GraphOptions{Depth: p.depth, TagsAsNodes: p.topics}
	scope, err := m.scope(p.vault)
	if err != nil {
		return graphView{}, err
	}
	if p.vault != "" {
		opts.Sources = []string{p.vault}
	}
	if p.center != "" {
		ref, err := scope.Locate(p.center)
		if err != nil {
			return graphView{}, err
		}
		opts.Center = &ref
	}
	g, err := m.sn.Graph(opts)
	if err != nil {
		return graphView{}, err
	}
	staleNotes, err := s.staleSet(m)
	if err != nil {
		return graphView{}, err
	}
	key := layoutKey(m.prints, p.topics)
	positions, cached := s.readLayout(key)
	view := graphView{
		Key:    key,
		Layout: layoutState{Cached: cached},
		Nodes:  make([]graphNode, 0, len(g.Nodes)),
		Edges:  g.Edges,
	}
	for _, n := range g.Nodes {
		node := graphNode{GraphNode: n, Stale: n.Path != "" && staleNotes[n.Vault+":"+n.Path]}
		if xy, ok := positions[n.ID]; ok {
			node.X, node.Y = &xy[0], &xy[1]
		}
		view.Nodes = append(view.Nodes, node)
	}
	return view, nil
}

// staleSet names every note in the stale queue as vault:path.
func (s *Server) staleSet(m *model) (map[string]bool, error) {
	q, err := s.staleQueue(m, len(m.sn.Notes())+1)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, g := range q.Groups {
		for _, e := range g.Notes {
			out[e.Vault+":"+e.Path] = true
		}
	}
	return out, nil
}
