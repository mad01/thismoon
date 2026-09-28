package mermaid

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/render"
)

// convert runs Convert and fails the test on error.
func convert(t *testing.T, src string) render.GraphInput {
	t.Helper()
	g, err := Convert(src)
	if err != nil {
		t.Fatalf("Convert(%q): %v", src, err)
	}
	return g
}

// ids returns the node ids in output order.
func ids(g render.GraphInput) []string {
	out := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		out = append(out, n.ID)
	}
	return out
}

// labels returns the node labels keyed by id.
func labels(g render.GraphInput) map[string]string {
	out := make(map[string]string, len(g.Nodes))
	for _, n := range g.Nodes {
		out[n.ID] = n.Label
	}
	return out
}

func edge(from, to, kind, label string) render.GraphEdge {
	return render.GraphEdge{From: from, To: to, Type: kind, Label: label}
}

func TestConvertHeader(t *testing.T) {
	tests := []struct {
		name string
		src  string
		dir  string
	}{
		{"graph TD", "graph TD\nA-->B", "TB"},
		{"graph TB", "graph TB\nA-->B", "TB"},
		{"flowchart LR", "flowchart LR\nA-->B", "LR"},
		{"BT maps to TB", "graph BT\nA-->B", "TB"},
		{"RL maps to LR", "graph RL\nA-->B", "LR"},
		{"no direction", "graph\nA-->B", ""},
		{"flowchart no direction", "flowchart\nA-->B", ""},
		{"uppercase keyword", "GRAPH TD\nA-->B", "TB"},
		{"mixed case keyword", "Flowchart LR\nA-->B", "LR"},
		{"lowercase direction", "graph td\nA-->B", "TB"},
		{"trailing semicolon", "graph TD;\nA-->B", "TB"},
		{"statements on header line", "graph TD; A-->B; B-->C", "TB"},
		{"comment before header", "%% a comment\ngraph LR\nA-->B", "LR"},
		{"init directive before header", "%%{init: {'theme':'dark'}}%%\ngraph LR\nA-->B", "LR"},
		{"surrounding whitespace", "\n\n  graph LR\n  A-->B\n\n", "LR"},
		{"crlf lines", "graph LR\r\nA-->B\r\n", "LR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := convert(t, tt.src)
			if g.Direction != tt.dir {
				t.Errorf("Direction = %q, want %q", g.Direction, tt.dir)
			}
			if g.Layout != "" {
				t.Errorf("Layout = %q, want empty", g.Layout)
			}
			if len(g.Nodes) < 2 || len(g.Edges) < 1 {
				t.Errorf(
					"got %d nodes, %d edges; want at least 2 and 1",
					len(g.Nodes),
					len(g.Edges),
				)
			}
		})
	}
}

func TestConvertHeaderLineStatements(t *testing.T) {
	g := convert(t, "graph TD; A-->B; B-->C")
	if got, want := ids(g), []string{"A", "B", "C"}; !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
	want := []render.GraphEdge{edge("A", "B", "", ""), edge("B", "C", "", "")}
	if !slices.Equal(g.Edges, want) {
		t.Errorf("edges = %+v, want %+v", g.Edges, want)
	}
}

func TestConvertShapes(t *testing.T) {
	tests := []struct {
		stmt  string
		label string
	}{
		{"A", "A"},
		{"A[text]", "text"},
		{"A(text)", "text"},
		{"A([text])", "text"},
		{"A[[text]]", "text"},
		{"A[(db)]", "db"},
		{"A((text))", "text"},
		{"A>text]", "text"},
		{"A{text}", "text"},
		{"A{{text}}", "text"},
		{"A[/text/]", "text"},
		{`A[\text\]`, "text"},
		{`A[/text\]`, "text"},
		{`A[\text/]`, "text"},
		{"A(((text)))", "text"},
		{"A[ padded ]", "padded"},
		{"A[two words here]", "two words here"},
		{`A["with [brackets] inside"]`, "with [brackets] inside"},
		{`A("with (parens) inside")`, "with (parens) inside"},
		{`A{"with {braces} inside"}`, "with {braces} inside"},
		{`A(["quoted stadium"])`, "quoted stadium"},
		{`A[/"quoted trapezoid"/]`, "quoted trapezoid"},
		{`A{"semi; colon"}`, "semi; colon"},
		{"A[line one<br/>line two]", "line one\nline two"},
		{"A[line one<br>line two]", "line one\nline two"},
		{"A[line one<br />line two]", "line one\nline two"},
		{"A[line one <BR/> line two]", "line one\nline two"},
		{"A[\"`**bold** markdown`\"]", "**bold** markdown"},
		{"A[x]:::cls", "x"},
		{"A:::cls", "A"},
		{"A:::my-class", "A"},
		{`A@{ shape: circle }`, "A"},
		{`A@{ shape: circle, label: "Start" }`, "Start"},
		{`A@{ label: "a, b", shape: rect }`, "a, b"},
		{`A@{ shape: rect, label: Bare words }`, "Bare words"},
		{"A@{ shape: rect, label: \"`**bold**`\" }", "**bold**"},
		{`A@{ shape: rect, label: "one<br>two" }`, "one\ntwo"},
		{`A@{ shape: rect, label: "" }`, "A"},
		{`A@{ not key value }`, "A"},
		{`A[x]@{ shape: circle }`, "x"},
		{`A[x]@{ label: "block wins" }`, "block wins"},
		{`A@{ shape: circle }:::cls`, "A"},
		{`A:::cls@{ shape: "cyl{}" }`, "A"},
		{`A[""]`, "A"},
		{"A[a #quot;b#quot;]", `a "b"`},
		{`A["A dec char:#9829;"]`, "A dec char:♥"},
		{"A[#35; and #amp;]", "# and &"},
		{"A[#1 priority]", "#1 priority"},
		{"A[C# and F#]", "C# and F#"},
		{"A[#zzz; unknown]", "#zzz; unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.stmt, func(t *testing.T) {
			g := convert(t, "graph TD\n"+tt.stmt)
			if got := ids(g); !slices.Equal(got, []string{"A"}) {
				t.Fatalf("ids = %v, want [A]", got)
			}
			if got := g.Nodes[0].Label; got != tt.label {
				t.Errorf("label = %q, want %q", got, tt.label)
			}
			if g.Nodes[0].Type != "" || g.Nodes[0].Color != 0 {
				t.Errorf("node = %+v, want plain leaf", g.Nodes[0])
			}
		})
	}
}

func TestConvertLabelRedefinition(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want map[string]string
	}{
		{
			"later definition labels bare mention", "graph TD\nA --> B\nB[Store]",
			map[string]string{"A": "A", "B": "Store"},
		},
		{
			"bare mention keeps earlier label", "graph TD\nB[Store]\nA --> B",
			map[string]string{"A": "A", "B": "Store"},
		},
		{"later label wins", "graph TD\nB[One]\nB[Two]", map[string]string{"B": "Two"}},
		{
			"label in chain", "graph TD\nA[Start] --> B[End] --> A",
			map[string]string{"A": "Start", "B": "End"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := convert(t, tt.src)
			got := labels(g)
			for id, want := range tt.want {
				if got[id] != want {
					t.Errorf("label[%s] = %q, want %q", id, got[id], want)
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("node ids = %v, want %d nodes", ids(g), len(tt.want))
			}
		})
	}
}

func TestConvertNodeOrder(t *testing.T) {
	g := convert(t, "graph TD\nC --> A\nB[b] --> A\nD")
	if got, want := ids(g), []string{"C", "A", "B", "D"}; !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v (first-mention order)", got, want)
	}
}

func TestConvertEdges(t *testing.T) {
	tests := []struct {
		stmt  string
		nodes []string
		edges []render.GraphEdge
	}{
		{"A --> B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{"A-->B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{"A --- B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{"A -.-> B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "publishes", "")}},
		{"A -.- B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "publishes", "")}},
		{"A -..-> B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "publishes", "")}},
		{"A ==> B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{"A === B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{"A --x B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{"A --o B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{"A --x|no| B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "no")}},
		{"A <--> B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{"A <-.-> B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "publishes", "")}},
		{"A x--x B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{"A o--o B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{"A -->|yes| B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "yes")}},
		{"A-->|yes|B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "yes")}},
		{"A --> |yes| B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "yes")}},
		{`A ---|"quoted"| B`, []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "quoted")}},
		{
			"A -.->|dotted| B",
			[]string{"A", "B"},
			[]render.GraphEdge{edge("A", "B", "publishes", "dotted")},
		},
		{"A -- yes --> B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "yes")}},
		{"A -- yes --- B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "yes")}},
		{"A--yes-->B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "yes")}},
		{
			"A -- two words --> B",
			[]string{"A", "B"},
			[]render.GraphEdge{edge("A", "B", "", "two words")},
		},
		{
			"A -- foo-bar --> B",
			[]string{"A", "B"},
			[]render.GraphEdge{edge("A", "B", "", "foo-bar")},
		},
		{
			`A -- "quoted" --> B`,
			[]string{"A", "B"},
			[]render.GraphEdge{edge("A", "B", "", "quoted")},
		},
		{
			"A -. maybe .-> B",
			[]string{"A", "B"},
			[]render.GraphEdge{edge("A", "B", "publishes", "maybe")},
		},
		{
			"A -. maybe .- B",
			[]string{"A", "B"},
			[]render.GraphEdge{edge("A", "B", "publishes", "maybe")},
		},
		{"A == big ==> B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "big")}},
		{"A -- text --x B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "text")}},
		{"A --- oops", []string{"A", "oops"}, []render.GraphEdge{edge("A", "oops", "", "")}},
		{"A --- xray", []string{"A", "xray"}, []render.GraphEdge{edge("A", "xray", "", "")}},
		{"A ---oops", []string{"A", "oops"}, []render.GraphEdge{edge("A", "oops", "", "")}},
		{"my-node-->B", []string{"my-node", "B"}, []render.GraphEdge{edge("my-node", "B", "", "")}},
		{"a.b --> c_d", []string{"a.b", "c_d"}, []render.GraphEdge{edge("a.b", "c_d", "", "")}},
		{"A1-.->B2", []string{"A1", "B2"}, []render.GraphEdge{edge("A1", "B2", "publishes", "")}},
		{"A ~~~ B", []string{"A", "B"}, nil},
		{"A e1@--> B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{"A e1@==> B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{
			"A e1@-.->|x| B",
			[]string{"A", "B"},
			[]render.GraphEdge{edge("A", "B", "publishes", "x")},
		},
		{"A e1@<--> B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{
			"A -- \"`**bold**`\" --> B",
			[]string{"A", "B"},
			[]render.GraphEdge{edge("A", "B", "", "**bold**")},
		},
		{
			"A -->|#quot;yes#quot;| B",
			[]string{"A", "B"},
			[]render.GraphEdge{edge("A", "B", "", `"yes"`)},
		},
		{"A -- C# --> B", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "C#")}},
		{"a&a --> b", []string{"a", "b"}, []render.GraphEdge{edge("a", "b", "", "")}},
		{"a --> b&b", []string{"a", "b"}, []render.GraphEdge{edge("a", "b", "", "")}},
		{
			"a&a&c --> b",
			[]string{"a", "c", "b"},
			[]render.GraphEdge{edge("a", "b", "", ""), edge("c", "b", "", "")},
		},
		{
			"A --> B --> C",
			[]string{"A", "B", "C"},
			[]render.GraphEdge{edge("A", "B", "", ""), edge("B", "C", "", "")},
		},
		{
			"A -->|x| B -.->|y| C",
			[]string{"A", "B", "C"},
			[]render.GraphEdge{edge("A", "B", "", "x"), edge("B", "C", "publishes", "y")},
		},
		{"A & B --> C & D", []string{"A", "B", "C", "D"}, []render.GraphEdge{
			edge("A", "C", "", ""), edge("A", "D", "", ""),
			edge("B", "C", "", ""), edge("B", "D", "", ""),
		}},
		{"A --> B & C --> D", []string{"A", "B", "C", "D"}, []render.GraphEdge{
			edge("A", "B", "", ""), edge("A", "C", "", ""),
			edge("B", "D", "", ""), edge("C", "D", "", ""),
		}},
		{"A[Start] --> B{Choice}", []string{"A", "B"}, []render.GraphEdge{edge("A", "B", "", "")}},
		{"A --> A", []string{"A"}, []render.GraphEdge{edge("A", "A", "", "")}},
	}
	for _, tt := range tests {
		t.Run(tt.stmt, func(t *testing.T) {
			g := convert(t, "graph TD\n"+tt.stmt)
			if got := ids(g); !slices.Equal(got, tt.nodes) {
				t.Errorf("ids = %v, want %v", got, tt.nodes)
			}
			if !slices.Equal(g.Edges, tt.edges) {
				t.Errorf("edges = %+v, want %+v", g.Edges, tt.edges)
			}
		})
	}
}

func TestConvertStatements(t *testing.T) {
	src := `graph LR
%% a comment line
A --> B; B --> C
classDef foo fill:#f9f,stroke:#333
class A foo
style B fill:#bbf
linkStyle 0 stroke:red
click A "https://example.com" "tooltip"
direction TB
A:::foo --> D
;;
   %% indented comment
D[Done];`
	g := convert(t, src)
	if got, want := ids(g), []string{"A", "B", "C", "D"}; !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
	want := []render.GraphEdge{
		edge("A", "B", "", ""), edge("B", "C", "", ""), edge("A", "D", "", ""),
	}
	if !slices.Equal(g.Edges, want) {
		t.Errorf("edges = %+v, want %+v", g.Edges, want)
	}
	if got := labels(g)["D"]; got != "Done" {
		t.Errorf("label[D] = %q, want Done", got)
	}
}

func TestConvertEntitySemicolonDoesNotSplit(t *testing.T) {
	g := convert(t, "graph TD\nA[a #quot;b#quot;]; B[#9829;]; C")
	if got, want := ids(g), []string{"A", "B", "C"}; !slices.Equal(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	got := labels(g)
	if got["A"] != `a "b"` || got["B"] != "♥" {
		t.Errorf("labels = %v, want entity codes decoded", got)
	}
}

func TestConvertSkippedKeywordNeedsWordBoundary(t *testing.T) {
	g := convert(t, "graph TD\nstyleA --> classB\nclicker")
	if got, want := ids(g), []string{"styleA", "classB", "clicker"}; !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
}

func TestConvertSubgraphs(t *testing.T) {
	src := `graph TD
subgraph one
A --> B
end
subgraph two [Second Title]
C --> D
end
B --> C
E`
	g := convert(t, src)
	want := []render.GraphNode{
		{ID: "A", Label: "A", Type: "module", Color: 0},
		{ID: "B", Label: "B", Type: "module", Color: 0},
		{ID: "C", Label: "C", Type: "module", Color: 1},
		{ID: "D", Label: "D", Type: "module", Color: 1},
		{ID: "E", Label: "E"},
	}
	if !slices.Equal(g.Nodes, want) {
		t.Errorf("nodes = %+v, want %+v", g.Nodes, want)
	}
	wantEdges := []render.GraphEdge{
		edge("A", "B", "", ""), edge("C", "D", "", ""), edge("B", "C", "", ""),
	}
	if !slices.Equal(g.Edges, wantEdges) {
		t.Errorf("edges = %+v, want %+v", g.Edges, wantEdges)
	}
}

func TestConvertNestedSubgraphs(t *testing.T) {
	src := `graph TD
subgraph outer
A
subgraph inner
direction LR
B
end
C
end
subgraph third
D
end`
	g := convert(t, src)
	want := []render.GraphNode{
		{ID: "A", Label: "A", Type: "module", Color: 0},
		{ID: "B", Label: "B", Type: "module", Color: 1},
		{ID: "C", Label: "C", Type: "module", Color: 0},
		{ID: "D", Label: "D", Type: "module", Color: 2},
	}
	if !slices.Equal(g.Nodes, want) {
		t.Errorf("nodes = %+v, want %+v", g.Nodes, want)
	}
}

func TestConvertSubgraphColorWraps(t *testing.T) {
	var b strings.Builder
	b.WriteString("graph TD\n")
	for _, id := range []string{"A", "B", "C", "D", "E"} {
		b.WriteString("subgraph s" + id + "\n" + id + "\nend\n")
	}
	g := convert(t, b.String())
	got := make([]int, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		got = append(got, n.Color)
	}
	if want := []int{0, 1, 2, 3, 0}; !slices.Equal(got, want) {
		t.Errorf("colors = %v, want %v", got, want)
	}
}

func TestConvertSubgraphEdgeCases(t *testing.T) {
	t.Run("edge to subgraph id makes a plain node", func(t *testing.T) {
		g := convert(t, "graph TD\nsubgraph svc\nA\nend\nB --> svc")
		want := []render.GraphNode{
			{ID: "A", Label: "A", Type: "module"},
			{ID: "B", Label: "B"},
			{ID: "svc", Label: "svc"},
		}
		if !slices.Equal(g.Nodes, want) {
			t.Errorf("nodes = %+v, want %+v", g.Nodes, want)
		}
	})
	t.Run("first subgraph mentioning a node claims it", func(t *testing.T) {
		g := convert(t, "graph TD\nA --> B\nsubgraph s\nA\nend\nsubgraph t\nA\nend")
		want := []render.GraphNode{
			{ID: "A", Label: "A", Type: "module", Color: 0},
			{ID: "B", Label: "B"},
		}
		if !slices.Equal(g.Nodes, want) {
			t.Errorf("nodes = %+v, want %+v", g.Nodes, want)
		}
	})
	t.Run("mermaid docs membership example", func(t *testing.T) {
		src := `flowchart TB
    c1-->a2
    subgraph one
    a1-->a2
    end
    subgraph two
    b1-->b2
    end
    subgraph three
    c1-->c2
    end`
		g := convert(t, src)
		want := []render.GraphNode{
			{ID: "c1", Label: "c1", Type: "module", Color: 2},
			{ID: "a2", Label: "a2", Type: "module", Color: 0},
			{ID: "a1", Label: "a1", Type: "module", Color: 0},
			{ID: "b1", Label: "b1", Type: "module", Color: 1},
			{ID: "b2", Label: "b2", Type: "module", Color: 1},
			{ID: "c2", Label: "c2", Type: "module", Color: 2},
		}
		if !slices.Equal(g.Nodes, want) {
			t.Errorf("nodes = %+v, want %+v", g.Nodes, want)
		}
	})
	t.Run("stray end is ignored", func(t *testing.T) {
		g := convert(t, "graph TD\nA\nend\nB")
		if got, want := ids(g), []string{"A", "B"}; !slices.Equal(got, want) {
			t.Errorf("ids = %v, want %v", got, want)
		}
	})
	t.Run("unclosed subgraph still converts", func(t *testing.T) {
		g := convert(t, "graph TD\nsubgraph s\nA")
		if n := g.Nodes[0]; n.Type != "module" {
			t.Errorf("node A = %+v, want module", n)
		}
	})
}

func TestConvertFrontMatter(t *testing.T) {
	tests := []struct {
		name string
		src  string
		dir  string
	}{
		{"title", "---\ntitle: Flow\n---\ngraph LR\nA-->B", "LR"},
		{
			"nested config",
			"---\nconfig:\n  flowchart:\n    nodeSpacing: 1\n---\n\ngraph TD\nA-->B",
			"TB",
		},
		{"blank lines before", "\n\n---\ntitle: x\n---\nflowchart\nA-->B", ""},
		{"indented markers", "  ---\ntitle: x\n  ---\ngraph RL\nA-->B", "LR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := convert(t, tt.src)
			if g.Direction != tt.dir {
				t.Errorf("Direction = %q, want %q", g.Direction, tt.dir)
			}
			if got, want := ids(g), []string{"A", "B"}; !slices.Equal(got, want) {
				t.Errorf("ids = %v, want %v", got, want)
			}
		})
	}
	t.Run("unclosed front matter is not a flowchart", func(t *testing.T) {
		_, err := Convert("---\ntitle: x\ngraph LR\nA-->B")
		if !errors.Is(err, ErrNotFlowchart) {
			t.Errorf("err = %v, want ErrNotFlowchart", err)
		}
	})
	t.Run("dashes after the header are a parse error", func(t *testing.T) {
		_, err := Convert("graph LR\n---\nA-->B")
		if err == nil || !strings.Contains(err.Error(), "line 2") {
			t.Errorf("err = %v, want a line 2 parse error", err)
		}
	})
}

func TestConvertAccessibility(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"accTitle", "graph LR\naccTitle: The title\nA-->B"},
		{"accDescr colon", "graph LR\n  accDescr: A description; with a semicolon\nA-->B"},
		{"accDescr one-line block", "graph LR\naccDescr { one line }\nA-->B"},
		{"accDescr multi-line block", "graph LR\naccDescr {\n  several\n  lines\n}\nA-->B"},
		{"accDescr block no space", "graph LR\naccDescr{\nx\n}\nA-->B"},
		{"both", "graph LR\naccTitle: t\naccDescr: d\nA-->B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := convert(t, tt.src)
			if got, want := ids(g), []string{"A", "B"}; !slices.Equal(got, want) {
				t.Errorf("ids = %v, want %v", got, want)
			}
		})
	}
	t.Run("acc prefix on a node id is not accessibility", func(t *testing.T) {
		g := convert(t, "graph LR\naccTitleNode --> accDescrNode")
		if got, want := ids(g), []string{"accTitleNode", "accDescrNode"}; !slices.Equal(got, want) {
			t.Errorf("ids = %v, want %v", got, want)
		}
	})
}

func TestConvertQuotedStringsSpanLines(t *testing.T) {
	t.Run("label keeps inner newlines and trims the ends", func(t *testing.T) {
		g := convert(t, "graph LR\nA[\"\n\n  first line\n  second line\n\"]; B")
		if got := labels(g)["A"]; got != "first line\nsecond line" {
			t.Errorf("label = %q, want %q", got, "first line\nsecond line")
		}
		if got, want := ids(g), []string{"A", "B"}; !slices.Equal(got, want) {
			t.Errorf("ids = %v, want %v", got, want)
		}
	})
	t.Run("markdown string spanning lines", func(t *testing.T) {
		g := convert(t, "graph LR\na(\"`The **cat**\n  in the hat`\") --> b")
		if got := labels(g)["a"]; got != "The **cat**\nin the hat" {
			t.Errorf("label = %q, want %q", got, "The **cat**\nin the hat")
		}
	})
	t.Run("click tooltip spanning lines is skipped", func(t *testing.T) {
		g := convert(t, "graph LR\nA --> B\nclick A \"/u?id=1\" \"AAA\n6000\"\nB --> C")
		if got, want := ids(g), []string{"A", "B", "C"}; !slices.Equal(got, want) {
			t.Errorf("ids = %v, want %v", got, want)
		}
		if len(g.Edges) != 2 {
			t.Errorf("edges = %+v, want 2", g.Edges)
		}
	})
	t.Run("comment marker inside a quote is content", func(t *testing.T) {
		g := convert(t, "graph LR\nA[\"x\n%% not a comment\"]")
		if got := labels(g)["A"]; got != "x\n%% not a comment" {
			t.Errorf("label = %q", got)
		}
	})
	t.Run("unterminated quote at end of input", func(t *testing.T) {
		_, err := Convert("graph LR\nA --> B\nB[\"open\nstill open")
		if err == nil || !strings.Contains(err.Error(), "unterminated quote") {
			t.Errorf("err = %v, want unterminated quote", err)
		}
		if err != nil && !strings.Contains(err.Error(), "line 3") {
			t.Errorf("err = %v, want it to name line 3 where the statement starts", err)
		}
	})
}

func TestConvertEdgeIDs(t *testing.T) {
	t.Run("edge config statement is dropped", func(t *testing.T) {
		g := convert(
			t,
			"flowchart LR\nA e1@==> B\nA e2@--> C\ne1@{ curve: linear }\ne2@{ animate: true }",
		)
		if got, want := ids(g), []string{"A", "B", "C"}; !slices.Equal(got, want) {
			t.Errorf("ids = %v, want %v", got, want)
		}
		want := []render.GraphEdge{edge("A", "B", "", ""), edge("A", "C", "", "")}
		if !slices.Equal(g.Edges, want) {
			t.Errorf("edges = %+v, want %+v", g.Edges, want)
		}
	})
	t.Run("unknown id with a block is a node", func(t *testing.T) {
		g := convert(t, "flowchart LR\ne1@{ shape: circle, label: \"Node\" }\nA e2@--> B")
		if got, want := ids(g), []string{"e1", "A", "B"}; !slices.Equal(got, want) {
			t.Errorf("ids = %v, want %v", got, want)
		}
		if got := labels(g)["e1"]; got != "Node" {
			t.Errorf("label[e1] = %q, want Node", got)
		}
	})
	t.Run("class on an edge id is skipped", func(t *testing.T) {
		g := convert(t, "flowchart LR\nA e1@--> B\nclass e1 animate")
		if got, want := ids(g), []string{"A", "B"}; !slices.Equal(got, want) {
			t.Errorf("ids = %v, want %v", got, want)
		}
	})
	t.Run("id without a link is still an error", func(t *testing.T) {
		_, err := Convert("flowchart LR\nA e1@ B")
		if err == nil || !strings.Contains(err.Error(), "line 2") {
			t.Errorf("err = %v, want a line 2 parse error", err)
		}
	})
}

// idList returns n ids with the given prefix joined by sep.
func idList(prefix string, n int, sep string) string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = prefix + strconv.Itoa(i)
	}
	return strings.Join(ids, sep)
}

func TestConvertLimits(t *testing.T) {
	t.Run("node cap", func(t *testing.T) {
		if g := convert(t, "graph TD\n"+idList("n", maxNodes, "\n")); len(g.Nodes) != maxNodes {
			t.Errorf("got %d nodes, want %d", len(g.Nodes), maxNodes)
		}
		_, err := Convert("graph TD\n" + idList("n", maxNodes+1, "\n"))
		if !errors.Is(err, errTooManyNodes) {
			t.Errorf("err = %v, want errTooManyNodes", err)
		}
	})
	t.Run("edge cap by fan-out", func(t *testing.T) {
		src := "graph TD\n" + idList("a", 50, "&") + " --> " + idList("b", 100, "&")
		if g := convert(t, src); len(g.Edges) != maxEdges {
			t.Errorf("got %d edges, want %d", len(g.Edges), maxEdges)
		}
		src = "graph TD\n" + idList("a", 50, "&") + " --> " + idList("b", 101, "&")
		_, err := Convert(src)
		if !errors.Is(err, errTooManyEdges) {
			t.Errorf("err = %v, want errTooManyEdges", err)
		}
		if err != nil && !strings.HasPrefix(err.Error(), "mermaid: flowchart has more than") {
			t.Errorf("err = %q, want the bare limit message", err)
		}
	})
	t.Run("edge cap across statements", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("graph TD\n")
		for i := 0; i <= maxEdges; i++ {
			b.WriteString("a --> b\n")
		}
		if _, err := Convert(b.String()); !errors.Is(err, errTooManyEdges) {
			t.Errorf("err = %v, want errTooManyEdges", err)
		}
	})
	t.Run("duplicate ids in a fan-out do not multiply", func(t *testing.T) {
		src := "graph TD\n" + strings.Repeat(
			"a&",
			4000,
		) + "a --> " + strings.Repeat(
			"b&",
			4000,
		) + "b"
		g := convert(t, src)
		if len(g.Edges) != 1 || len(g.Nodes) != 2 {
			t.Errorf("got %d nodes, %d edges; want 2 and 1", len(g.Nodes), len(g.Edges))
		}
	})
}

func TestConvertErrors(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		contains string
		is       error
	}{
		{"empty", "", "empty input", ErrNotFlowchart},
		{"whitespace only", "  \n\n\t\n", "empty input", ErrNotFlowchart},
		{"only comments", "%% nothing\n%% here", "empty input", ErrNotFlowchart},
		{"sequence diagram", "sequenceDiagram\nA->>B: hi", `"sequenceDiagram"`, ErrNotFlowchart},
		{"class diagram", "classDiagram\nclass A", "not a flowchart", ErrNotFlowchart},
		{"prose", "this is not a diagram at all", "not a flowchart", ErrNotFlowchart},
		{"bad direction", "graph XX\nA-->B", `"graph XX"`, ErrNotFlowchart},
		{"extra header tokens", "graph LR A-->B", "not a flowchart", ErrNotFlowchart},
		{"node before header", "A-->B\ngraph LR", "not a flowchart", ErrNotFlowchart},
		{"no nodes", "graph TD", "flowchart has no nodes", nil},
		{"only skipped statements", "graph TD\nclassDef x fill:#fff", "has no nodes", nil},
		{"empty subgraph", "graph TD\nsubgraph s\nend", "has no nodes", nil},
		{"dangling link", "graph TD\nA --> ", `line 2: cannot parse "A -->"`, nil},
		{"dangling link before next line", "graph TD\nA -->\nB", "line 2", nil},
		{"link without source", "graph TD\n--> B", `line 2: cannot parse "--> B"`, nil},
		{"leftover after chain", "graph TD\nA --> B )", `unexpected ")"`, nil},
		{"unterminated label", "graph TD\nA[unterminated", "line 2", nil},
		{"unterminated quote", `graph TD` + "\n" + `A["unterminated]`, "unterminated quote", nil},
		{"quote not followed by closer", `graph TD` + "\n" + `A["x" y]`, `missing ']'`, nil},
		{"unterminated pipe label", "graph TD\nA -->|no close B", "unterminated |label|", nil},
		{"unterminated inline text", "graph TD\nA -- text B", "unterminated link text", nil},
		{"single dash", "graph TD\nA - B", "line 2", nil},
		{"dangling ampersand", "graph TD\nA & --> B", "line 2", nil},
		{"unterminated shape block", "graph TD\nA@{ shape: circle", "unterminated @{", nil},
		{"bare class marker", "graph TD\nA:::", "missing class name", nil},
		{"line number counts blank lines", "graph TD\n\n\nA -->", "line 4", nil},
		{"line number counts comments", "graph TD\n%% c\n%% c\nA -->", "line 4", nil},
		{"second statement on a line", "graph TD\nA --> B; C -->", "line 2", nil},
		{"non-ascii id", "graph TD\nÅ --> B", "line 2", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Convert(tt.src)
			if err == nil {
				t.Fatalf("Convert(%q) = nil error, want one containing %q", tt.src, tt.contains)
			}
			if !strings.HasPrefix(err.Error(), "mermaid: ") {
				t.Errorf("error %q lacks the mermaid: prefix", err)
			}
			if !strings.Contains(err.Error(), tt.contains) {
				t.Errorf("error %q does not contain %q", err, tt.contains)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("errors.Is(%q, %v) = false", err, tt.is)
			}
			if tt.is == nil && errors.Is(err, ErrNotFlowchart) {
				t.Errorf("error %q wraps ErrNotFlowchart, want a parse error", err)
			}
		})
	}
}

func TestConvertErrorSnippetIsCapped(t *testing.T) {
	long := strings.Repeat("x", 200)
	_, err := Convert("graph TD\n" + long + " --> ")
	if err == nil {
		t.Fatal("want error")
	}
	if len(err.Error()) > 200 {
		t.Errorf("error is %d bytes; statement snippet not capped: %q", len(err.Error()), err)
	}
	if !strings.Contains(err.Error(), "...") {
		t.Errorf("error %q lacks the truncation marker", err)
	}
}

// validSources are flowcharts Convert accepts; they seed the fuzzer and are
// checked as a whole here.
var validSources = []string{
	"graph TD\nA-->B",
	"flowchart LR\nA[Start] --> B{Choice}\nB -->|yes| C([Done])\nB -->|no| D[(Store)]",
	"graph TD; A-->B; B-->C",
	"graph LR\n%% comment\nA -- text --> B -. dotted .-> C == thick ==> D",
	"graph TD\nsubgraph one\nA --> B\nend\nsubgraph two\nC\nend\nB --> C",
	"graph TD\nA & B --> C & D",
	`graph TD` + "\n" + `A["quoted [label]"]:::cls --> B@{ shape: circle, label: "x" }`,
	"graph TD\nA[line one<br/>line two] <--> B\nA --x C\nA --o D\nA ~~~ E",
	"graph TD\nmy-node --> a.b --> c_d",
	"flowchart\nA --- oops",
	"---\ntitle: t\n---\ngraph LR\naccTitle: t\naccDescr {\n d\n}\nA[\"multi\nline\"] e1@--> B\ne1@{ animate: true }",
	"flowchart RL\nA@{ shape: rect, label: \"Process\" } --> B\nclick A \"u\" \"tip\n2\"",
	"graph BT\nA[\"`**bold**`\"] --> B\nclassDef x fill:#fff\nstyle A fill:#f9f\nclick A \"u\"",
}

func TestConvertValidSources(t *testing.T) {
	for _, src := range validSources {
		g := convert(t, src)
		checkInvariants(t, src, g)
	}
}

// TestConvertCorpus runs every real flowchart under testdata (see
// testdata/SOURCES.md). Files in the unsupported table must fail with the
// reason given; when one starts parsing the entry has to go.
func TestConvertCorpus(t *testing.T) {
	// corpusFiles pins the corpus size so a deleted file fails the test
	// instead of shrinking coverage silently.
	const corpusFiles = 177
	unsupported := map[string]string{}
	files, err := filepath.Glob(filepath.Join("testdata", "*.mmd"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != corpusFiles {
		t.Fatalf(
			"found %d corpus files, want %d (see testdata/SOURCES.md)",
			len(files),
			corpusFiles,
		)
	}
	for _, file := range files {
		name := filepath.Base(file)
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			g, err := Convert(string(src))
			if reason, listed := unsupported[name]; listed {
				if err == nil {
					t.Fatalf("listed as unsupported (%s) but converted; drop the entry", reason)
				}
				return
			}
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			checkInvariants(t, name, g)
		})
	}
}

// corpusFile converts one testdata file and fails the test on error.
func corpusFile(t *testing.T, name string) render.GraphInput {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return convert(t, string(src))
}

// findEdge returns the first edge from -> to, failing the test if none.
func findEdge(t *testing.T, g render.GraphInput, from, to string) render.GraphEdge {
	t.Helper()
	for _, e := range g.Edges {
		if e.From == from && e.To == to {
			return e
		}
	}
	t.Fatalf("no edge %s -> %s in %+v", from, to, g.Edges)
	return render.GraphEdge{}
}

func TestConvertCorpusSpotChecks(t *testing.T) {
	t.Run("demo-01 front matter, accessibility, multi-line labels", func(t *testing.T) {
		g := corpusFile(t, "demo-01.mmd")
		if len(g.Nodes) != 15 || len(g.Edges) != 19 || g.Direction != "LR" {
			t.Errorf("got %d nodes, %d edges, dir %q; want 15, 19, LR",
				len(g.Nodes), len(g.Edges), g.Direction)
		}
		if got := g.Nodes[0].Label; got != "提交申请\n熊大" {
			t.Errorf("first label = %q, want %q", got, "提交申请\n熊大")
		}
		e := findEdge(t, g, "sid-1897B30A-9C5C-4D5B-B80B-76A038785070",
			"sid-4FC27B48-A6F9-460A-A675-021F5854FE22")
		if e.Label != "" || e.Type != "" {
			t.Errorf("edge = %+v, want plain unlabelled", e)
		}
	})
	t.Run("demo-15 click tooltips spanning lines", func(t *testing.T) {
		g := corpusFile(t, "demo-15.mmd")
		if len(g.Nodes) != 3 || len(g.Edges) != 2 || g.Direction != "LR" {
			t.Errorf("got %d nodes, %d edges, dir %q; want 3, 2, LR",
				len(g.Nodes), len(g.Edges), g.Direction)
		}
		if got := g.Nodes[0].Label; got != "1051 AAA fa:fa-check" {
			t.Errorf("first label = %q", got)
		}
		e := findEdge(t, g, "456ac9b0d15a8b7f1e71073221059886", "f7f580e11d00a75814d2ded41fe8e8fe")
		if e.Label != "Node" {
			t.Errorf("edge label = %q, want Node", e.Label)
		}
	})
	t.Run("docs-107 edge ids and edge config", func(t *testing.T) {
		g := corpusFile(t, "docs-107.mmd")
		if len(g.Nodes) != 3 || len(g.Edges) != 2 || g.Direction != "LR" {
			t.Errorf("got %d nodes, %d edges, dir %q; want 3, 2, LR",
				len(g.Nodes), len(g.Edges), g.Direction)
		}
		if got := labels(g)["C"]; got != "C" {
			t.Errorf("label[C] = %q, want C", got)
		}
		if e := findEdge(t, g, "A", "C"); e.Label != "" {
			t.Errorf("edge A->C label = %q, want empty", e.Label)
		}
	})
	t.Run("docs-008 front matter, subgraphs, dotted inline label", func(t *testing.T) {
		g := corpusFile(t, "docs-008.mmd")
		if len(g.Nodes) != 6 || len(g.Edges) != 6 || g.Direction != "LR" {
			t.Errorf("got %d nodes, %d edges, dir %q; want 6, 6, LR",
				len(g.Nodes), len(g.Edges), g.Direction)
		}
		if got := labels(g)["Cache"]; got != "Local cache" {
			t.Errorf("label[Cache] = %q, want Local cache", got)
		}
		e := findEdge(t, g, "Auth", "UI")
		if e.Label != "token" || e.Type != "publishes" {
			t.Errorf("edge Auth->UI = %+v, want label token, type publishes", e)
		}
		colors := map[string]int{}
		for _, n := range g.Nodes {
			if n.Type != "module" {
				t.Errorf("node %s type %q, want module", n.ID, n.Type)
			}
			colors[n.ID] = n.Color
		}
		if colors["UI"] != 0 || colors["API"] != 1 || colors["DB"] != 2 {
			t.Errorf("colors = %v, want UI 0, API 1, DB 2", colors)
		}
	})
	t.Run("docs-103 markdown strings spanning lines", func(t *testing.T) {
		g := corpusFile(t, "docs-103.mmd")
		if len(g.Nodes) != 4 || len(g.Edges) != 2 || g.Direction != "LR" {
			t.Errorf("got %d nodes, %d edges, dir %q; want 4, 2, LR",
				len(g.Nodes), len(g.Edges), g.Direction)
		}
		if got := labels(g)["a"]; got != "The **cat**\nin the hat" {
			t.Errorf("label[a] = %q", got)
		}
		if e := findEdge(t, g, "c", "d"); e.Label != "Bold **edge label**" {
			t.Errorf("edge c->d label = %q", e.Label)
		}
	})
}

// checkInvariants asserts what every successful conversion must satisfy.
func checkInvariants(t *testing.T, src string, g render.GraphInput) {
	t.Helper()
	if len(g.Nodes) == 0 {
		t.Fatalf("Convert(%q) returned no nodes without an error", src)
	}
	seen := map[string]bool{}
	for _, n := range g.Nodes {
		if n.ID == "" || n.Label == "" {
			t.Errorf("Convert(%q): node %+v has an empty id or label", src, n)
		}
		if seen[n.ID] {
			t.Errorf("Convert(%q): duplicate node id %q", src, n.ID)
		}
		seen[n.ID] = true
		if n.Type == "module" && (n.Color < 0 || n.Color >= moduleColors) {
			t.Errorf("Convert(%q): module color %d out of range", src, n.Color)
		}
		if n.Type != "" && n.Type != "module" {
			t.Errorf("Convert(%q): unexpected node type %q", src, n.Type)
		}
	}
	for _, e := range g.Edges {
		if !seen[e.From] || !seen[e.To] {
			t.Errorf("Convert(%q): edge %+v references an unknown node", src, e)
		}
		if e.Type != "" && e.Type != "publishes" {
			t.Errorf("Convert(%q): unexpected edge type %q", src, e.Type)
		}
		if e.Weight != 0 || e.Flow {
			t.Errorf("Convert(%q): edge %+v sets weight or flow", src, e)
		}
	}
	if d := g.Direction; d != "" && d != "TB" && d != "LR" {
		t.Errorf("Convert(%q): direction %q", src, d)
	}
	if g.Layout != "" {
		t.Errorf("Convert(%q): layout %q, want empty", src, g.Layout)
	}
}

func FuzzConvert(f *testing.F) {
	for _, src := range validSources {
		f.Add(src)
	}
	f.Add("graph TD\nA[\"unterminated")
	f.Add("graph TD\nA -- ")
	f.Add("graph TD\nA@{")
	f.Add("sequenceDiagram")
	f.Fuzz(func(t *testing.T, src string) {
		g, err := Convert(src)
		if err != nil {
			if !strings.HasPrefix(err.Error(), "mermaid: ") {
				t.Errorf("error %q lacks the mermaid: prefix", err)
			}
			return
		}
		checkInvariants(t, src, g)
	})
}
