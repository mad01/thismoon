package vault

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// Edge kinds.
const (
	EdgeLink = "link"
	EdgeTag  = "tag"
)

// Node id prefixes. A note's node id is id:<note id>, or vault:path when the
// note has no id or shares it with another note.
const (
	danglingNodePrefix = "dangling:"
	tagNodePrefix      = "tag:"
)

// defaultGraphDepth is the hop limit around a center note.
const defaultGraphDepth = 1

// GraphOptions selects the part of the link graph Graph returns.
type GraphOptions struct {
	// Sources names the vaults whose notes are walked; empty walks every
	// vault. A note in another vault that a walked note links to still
	// appears, without its own links.
	Sources []string
	// Center, when set, keeps only the nodes within Depth hops of it, links
	// followed in either direction.
	Center *Ref
	// Depth is the hop limit around Center, 1 when zero.
	Depth int
	// TagsAsNodes adds a node per tag and a tag edge from each note to it.
	TagsAsNodes bool
}

// GraphNode is a note, a dangling link target, or a tag.
type GraphNode struct {
	ID       string   `json:"id"                  jsonschema:"id:<note id>, vault:path, dangling:<target> or tag:<name>"`
	Vault    string   `json:"vault,omitempty"`
	Path     string   `json:"path,omitempty"`
	Stem     string   `json:"stem,omitempty"      jsonschema:"filename without .md; for a dangling node, the target as written"`
	Title    string   `json:"title,omitempty"`
	Tier     string   `json:"tier,omitempty"`
	Type     string   `json:"type,omitempty"`
	Status   string   `json:"status,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Evidence string   `json:"evidence,omitempty"`
	ReviewBy string   `json:"review_by,omitempty" jsonschema:"the review_by frontmatter value, when set"`
	Degree   int      `json:"degree"              jsonschema:"edges in this graph that touch the node, either direction"`
	Dangling bool     `json:"dangling"            jsonschema:"true for a link target no note matches"`
}

// GraphEdge is one link (or tag) from a note. Resolved is false for an edge
// to a dangling node.
type GraphEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Kind     string `json:"kind"     jsonschema:"link, or tag with tags as nodes"`
	Resolved bool   `json:"resolved"`
}

// Graph is the link graph of a set: every walked note, what it links to,
// and the dangling targets, sorted by id.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// Graph builds the link graph of the set with the one link resolver
// backlinks and promote use. Links of a note to itself are left out.
func (s *Set) Graph(opts GraphOptions) (Graph, error) {
	walked, err := s.Filter(opts.Sources)
	if err != nil {
		return Graph{}, err
	}
	idx, err := buildLinkIndex(s)
	if err != nil {
		return Graph{}, err
	}
	b := graphBuilder{idx: idx, nodes: map[string]*GraphNode{}, edges: map[GraphEdge]bool{}}
	walk := map[*Vault]bool{}
	for _, v := range walked.vaults {
		walk[v] = true
	}
	for _, n := range idx.notes {
		if walk[n.ref.Vault] {
			b.addNoteEdges(n, opts.TagsAsNodes)
		}
	}
	if opts.Center != nil {
		center, err := idx.note(*opts.Center)
		if err != nil {
			return Graph{}, err
		}
		depth := opts.Depth
		if depth <= 0 {
			depth = defaultGraphDepth
		}
		b.keepNear(b.addNote(center), depth)
	}
	return b.graph(), nil
}

type graphBuilder struct {
	idx   *linkIndex
	nodes map[string]*GraphNode
	edges map[GraphEdge]bool
}

func (b *graphBuilder) addNoteEdges(n *indexedNote, tags bool) {
	src := b.addNote(n)
	for _, l := range n.links {
		if l.to == n {
			continue
		}
		var dst string
		if l.to != nil {
			dst = b.addNote(l.to)
		} else {
			dst = b.addDangling(l.Target)
		}
		b.edges[GraphEdge{Source: src, Target: dst, Kind: EdgeLink, Resolved: l.to != nil}] = true
	}
	if !tags {
		return
	}
	for _, tag := range n.fm.Tags {
		id := tagNodePrefix + tag
		if b.nodes[id] == nil {
			b.nodes[id] = &GraphNode{ID: id, Title: tag}
		}
		b.edges[GraphEdge{Source: src, Target: id, Kind: EdgeTag, Resolved: true}] = true
	}
}

// addNote adds the node of n when it is new and returns its id.
func (b *graphBuilder) addNote(n *indexedNote) string {
	id := n.ref.String()
	if n.fm.ID != "" && !b.idx.dupIDs[n.fm.ID] {
		id = "id:" + n.fm.ID
	}
	if b.nodes[id] != nil {
		return id
	}
	b.nodes[id] = &GraphNode{
		ID:       id,
		Vault:    n.ref.Vault.Name,
		Path:     n.ref.Path,
		Stem:     n.stem,
		Title:    n.fm.Title,
		Tier:     tierDir(n.ref.Path),
		Type:     n.fm.Type,
		Status:   n.fm.Status,
		Tags:     n.fm.Tags,
		Evidence: markdown.EvidenceOf(n.fm),
		ReviewBy: reviewBy(n.fm),
	}
	return id
}

// addDangling adds the node of an unresolved target, one per target
// regardless of case, and returns its id.
func (b *graphBuilder) addDangling(target string) string {
	id := danglingNodePrefix + strings.ToLower(target)
	if b.nodes[id] == nil {
		b.nodes[id] = &GraphNode{ID: id, Stem: target, Dangling: true}
	}
	return id
}

// keepNear drops the nodes more than depth hops from center, and the edges
// that touch them.
func (b *graphBuilder) keepNear(center string, depth int) {
	adjacent := map[string][]string{}
	for e := range b.edges {
		adjacent[e.Source] = append(adjacent[e.Source], e.Target)
		adjacent[e.Target] = append(adjacent[e.Target], e.Source)
	}
	near := map[string]bool{center: true}
	frontier := []string{center}
	for hop := 0; hop < depth && len(frontier) > 0; hop++ {
		var next []string
		for _, id := range frontier {
			for _, nb := range adjacent[id] {
				if !near[nb] {
					near[nb] = true
					next = append(next, nb)
				}
			}
		}
		frontier = next
	}
	for id := range b.nodes {
		if !near[id] {
			delete(b.nodes, id)
		}
	}
	for e := range b.edges {
		if b.nodes[e.Source] == nil || b.nodes[e.Target] == nil {
			delete(b.edges, e)
		}
	}
}

// graph counts each node's degree on the final edges and sorts the result.
func (b *graphBuilder) graph() Graph {
	g := Graph{
		Nodes: make([]GraphNode, 0, len(b.nodes)),
		Edges: make([]GraphEdge, 0, len(b.edges)),
	}
	for e := range b.edges {
		b.nodes[e.Source].Degree++
		b.nodes[e.Target].Degree++
		g.Edges = append(g.Edges, e)
	}
	for _, n := range b.nodes {
		g.Nodes = append(g.Nodes, *n)
	}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	sort.Slice(g.Edges, func(i, j int) bool {
		a, c := g.Edges[i], g.Edges[j]
		if a.Source != c.Source {
			return a.Source < c.Source
		}
		if a.Target != c.Target {
			return a.Target < c.Target
		}
		return a.Kind < c.Kind
	})
	return g
}

// tierDir returns the tier directory a vault-relative path is in, or "".
func tierDir(rel string) string {
	first, _, _ := strings.Cut(rel, "/")
	for _, t := range AllTiers {
		if first == t.Dir() {
			return first
		}
	}
	return ""
}

// reviewBy returns the review_by frontmatter value as written, or "".
func reviewBy(fm markdown.Frontmatter) string {
	for _, f := range fm.Extra {
		if f.Key == "review_by" && f.Value != nil {
			return f.Value.Value
		}
	}
	return ""
}

// String renders a node for a text listing: vault:path for a note, the
// target in brackets for a dangling node, #name for a tag.
func (n GraphNode) String() string {
	switch {
	case n.Dangling:
		return fmt.Sprintf("[[%s]] (dangling)", n.Stem)
	case strings.HasPrefix(n.ID, tagNodePrefix):
		return "#" + n.Title
	}
	return n.Vault + ":" + n.Path
}
