package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func renderBlocks(t *testing.T, blocks ...Block) string {
	t.Helper()
	out, err := RenderDoc(Doc{Sections: []Section{{Heading: "S", Blocks: blocks}}}, "T")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	return out
}

func TestRenderDocColumnsBlock(t *testing.T) {
	out := renderBlocks(t, Block{T: "columns", Columns: [][]Block{
		{{T: "p", Text: "left"}, {T: "stat", Value: "41 min", Label: "outage"}},
		{{T: "chart", Kind: "bar", Series: []ChartSeries{{Points: []ChartPoint{{X: "a", Y: 1}}}}}},
		{{T: "list", Items: []string{"one"}}},
	}})
	for _, want := range []string{
		`<wk-columns cols="3">`,
		`<p data-fixation>left</p>`,
		`<wk-stat-value>41 min</wk-stat-value>`,
		`class="present-chart"`,
		`<li>one</li>`,
		`</wk-columns>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
	if n := strings.Count(out, "<wk-col>"); n != 3 {
		t.Errorf("wk-col count = %d, want 3", n)
	}
}

func TestRenderDocStatBlock(t *testing.T) {
	out := renderBlocks(
		t,
		Block{T: "stat", Value: "41 min", Label: "checkout **outage**", Subtitle: "14:02 to 14:43"},
	)
	for _, want := range []string{
		`<wk-stat>`,
		`<wk-stat-value>41 min</wk-stat-value>`,
		`<wk-stat-label data-fixation>checkout <strong>outage</strong></wk-stat-label>`,
		`<wk-stat-sub data-fixation>14:02 to 14:43</wk-stat-sub>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
	// The value is a figure: escaped, never fixated, no inline markdown.
	out = renderBlocks(t, Block{T: "stat", Value: "<3 **x**", Label: "l"})
	if !strings.Contains(out, `<wk-stat-value>&lt;3 **x**</wk-stat-value>`) {
		t.Errorf("stat value not escaped verbatim: %s", out)
	}
	if strings.Contains(out, "wk-stat-sub") {
		t.Errorf("stat without sub rendered a sub line: %s", out)
	}
}

func TestRenderDocQuoteBlock(t *testing.T) {
	out := renderBlocks(
		t,
		Block{T: "quote", Text: "We never saw the resolver.", Cite: "On-call, retro"},
	)
	for _, want := range []string{
		"<blockquote>",
		`<p data-fixation>We never saw the resolver.</p>`,
		`<cite>On-call, retro</cite>`,
		"</blockquote>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
	if out := renderBlocks(t, Block{T: "quote", Text: "q"}); strings.Contains(out, "<cite>") {
		t.Errorf("quote without cite rendered one: %s", out)
	}
}

func TestRenderDocDetailsBlock(t *testing.T) {
	out := renderBlocks(t, Block{T: "details", Summary: "Full timeline", Blocks: []Block{
		{T: "list", Items: []string{"14:02 first 502s"}},
		{T: "callout", Severity: "ok", Text: "resolved"},
	}})
	for _, want := range []string{
		"<details>",
		`<summary data-fixation>Full timeline</summary>`,
		`<li>14:02 first 502s</li>`,
		`<wk-callout variant="ok" data-fixation>resolved</wk-callout>`,
		"</details>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
	if strings.Contains(out, "<details open") {
		t.Error("details must render closed")
	}
}

func TestRenderDocCalloutSeverities(t *testing.T) {
	for _, sev := range []string{"info", "warn", "ok", "error"} {
		out := renderBlocks(t, Block{T: "callout", Severity: sev, Text: "x"})
		if !strings.Contains(out, `<wk-callout variant="`+sev+`" data-fixation>`) {
			t.Errorf("sev %q: %s", sev, out)
		}
	}
}

// Name normalization reaches the blocks a container holds and the new
// text fields, and still leaves a nested code block alone.
func TestRenderDocNormalizesNestedBlocks(t *testing.T) {
	out := renderBlocks(
		t,
		Block{T: "columns", Columns: [][]Block{
			{{T: "p", Text: "a & b"}, {T: "code", Text: "a & b"}},
			{{T: "stat", Value: "A → B", Label: "c & d", Subtitle: "e & f"}},
		}},
		Block{
			T:       "details",
			Summary: "g & h",
			Blocks:  []Block{{T: "quote", Text: "i & j", Cite: "k & l"}},
		},
	)
	for _, want := range []string{
		"a and b", "<code class=\"language-text\">a &amp; b</code>",
		"<wk-stat-value>A → B</wk-stat-value>", "c and d", "e and f", "g and h", "i and j", "k and l",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
}

func TestRenderDocBlockValidation(t *testing.T) {
	one := []Block{{T: "p", Text: "x"}}
	cases := []struct {
		name  string
		block Block
		want  string
	}{
		{"one column", Block{T: "columns", Columns: [][]Block{one}}, "want 2 or 3 columns"},
		{
			"four columns",
			Block{T: "columns", Columns: [][]Block{one, one, one, one}},
			"want 2 or 3 columns",
		},
		{"no columns", Block{T: "columns"}, "want 2 or 3 columns"},
		{
			"nested columns",
			Block{
				T:       "columns",
				Columns: [][]Block{one, {{T: "columns", Columns: [][]Block{one, one}}}},
			},
			"columns block: not allowed inside",
		},
		{
			"details in columns",
			Block{
				T:       "columns",
				Columns: [][]Block{one, {{T: "details", Summary: "s", Blocks: one}}},
			},
			"details block: not allowed inside",
		},
		{"empty details", Block{T: "details", Summary: "s"}, "blocks is empty"},
		{"details without summary", Block{T: "details", Blocks: one}, "summary is required"},
		{
			"nested details",
			Block{
				T:       "details",
				Summary: "s",
				Blocks:  []Block{{T: "details", Summary: "t", Blocks: one}},
			},
			"details block: not allowed inside",
		},
		{
			"graph in details",
			Block{T: "details", Summary: "s", Blocks: []Block{{T: "graph"}}},
			"graph block: not allowed inside a details block",
		},
		{
			"panel accent in a column",
			Block{
				T:       "columns",
				Columns: [][]Block{one, {{T: "panel", Title: "P", Accent: "wg600"}}},
			},
			`unknown accent "wg600"`,
		},
		{"unknown chart kind", Block{T: "chart", Title: "Errors", Kind: "pie"}, `chart "Errors": unknown kind "pie"`},
		{
			"unknown chart kind in a column",
			Block{T: "columns", Columns: [][]Block{one, {{T: "chart", Kind: "pie"}}}},
			`unknown kind "pie" (want one of bar, line`,
		},
		{
			"step without caption",
			Block{T: "chart", Title: "C", Steps: []Step{{Caption: "one"}, {Caption: " "}}},
			`chart "C": step 2 has no caption`,
		},
		{
			"series step without steps",
			Block{T: "chart", Title: "C", Series: []ChartSeries{{Name: "a", Step: 1}}},
			`series "a" names step 1 but the chart has no steps`,
		},
		{
			"series step out of range",
			Block{T: "chart", Title: "C", Steps: []Step{{Caption: "one"}}, Series: []ChartSeries{{Name: "a", Step: 2}}},
			`series "a" step 2: want 1 to 1`,
		},
		{
			"steps on a sparkline",
			Block{T: "chart", Title: "C", Kind: "sparkline", Steps: []Step{{Caption: "one"}}},
			`a sparkline cannot carry steps`,
		},
		{
			"series step on a doughnut",
			Block{T: "chart", Title: "C", Kind: "doughnut", Steps: []Step{{Caption: "one"}}, Series: []ChartSeries{{Name: "a", Step: 1}}},
			`a doughnut draws its first series only`,
		},
		{
			"series step on a sankey",
			Block{T: "chart", Title: "C", Kind: "sankey", Steps: []Step{{Caption: "one"}}, Series: []ChartSeries{{Name: "a", Step: 1}}},
			`a sankey draws flows, not series`,
		},
		{
			"series step on a ribbon",
			Block{T: "chart", Title: "C", Kind: "ribbon", Steps: []Step{{Caption: "one"}}, Series: []ChartSeries{{Name: "a", Step: 1, Points: []ChartPoint{{X: "q1", Y: 1}}}}},
			`a ribbon walks its periods`,
		},
		{"order on a bar chart", Block{T: "chart", Title: "C", Kind: "bar", Order: "given"}, `chart "C": order is a ribbon field`},
		{"unknown ribbon order", Block{T: "chart", Title: "C", Kind: "ribbon", Order: "value"}, `chart "C": unknown order "value" (want rank or given)`},
		{
			"ribbon series without a name",
			Block{T: "chart", Title: "C", Kind: "ribbon", Series: []ChartSeries{{Name: "a", Points: []ChartPoint{{X: "q1", Y: 1}}}, {Name: " ", Points: []ChartPoint{{X: "q1", Y: 1}}}}},
			`chart "C": series 2 has no name`,
		},
		{
			"ribbon with a fifth series",
			Block{T: "chart", Title: "C", Kind: "ribbon", Series: []ChartSeries{
				{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"},
			}},
			`chart "C": a ribbon takes at most 4 series (the palette's series colours), got 5`,
		},
		{
			"ribbon series named twice",
			Block{T: "chart", Title: "C", Kind: "ribbon", Series: []ChartSeries{{Name: "a", Points: []ChartPoint{{X: "q1", Y: 1}}}, {Name: "a", Points: []ChartPoint{{X: "q1", Y: 1}}}}},
			`chart "C": two series are named "a"`,
		},
		{
			"ribbon point without a period",
			Block{T: "chart", Title: "C", Kind: "ribbon", Series: []ChartSeries{{Name: "a", Points: []ChartPoint{{X: "q1", Y: 1}, {X: " ", Y: 2}}}}},
			`chart "C": series "a" point 2 has no x (the period)`,
		},
		{
			"ribbon period repeated in a series",
			Block{T: "chart", Title: "C", Kind: "ribbon", Series: []ChartSeries{{Name: "a", Points: []ChartPoint{{X: "q1", Y: 1}, {X: "q2", Y: 2}, {X: "q1", Y: 9}}}}},
			`chart "C": series "a" repeats period "q1"`,
		},
		{
			"ribbon negative value",
			Block{T: "chart", Title: "C", Kind: "ribbon", Series: []ChartSeries{{Name: "a", Points: []ChartPoint{{X: "q1", Y: -3}}}}},
			`chart "C": series "a" at q1 is -3, want 0 or more`,
		},
		{
			"ribbon column total overflows",
			Block{T: "chart", Title: "C", Kind: "ribbon", Series: []ChartSeries{{Name: "a", Points: []ChartPoint{{X: "q1", Y: 1e308}}}, {Name: "b", Points: []ChartPoint{{X: "q1", Y: 1e308}}}}},
			`chart "C": the q1 column total is too large to draw`,
		},
		{
			"ribbon series disagree on the period order",
			Block{T: "chart", Title: "C", Kind: "ribbon", Series: []ChartSeries{
				{Name: "a", Points: []ChartPoint{{X: "q2", Y: 1}, {X: "q3", Y: 1}}},
				{Name: "b", Points: []ChartPoint{{X: "q1", Y: 1}, {X: "q2", Y: 1}, {X: "q3", Y: 1}}},
			}},
			`chart "C": series "b" lists "q1" before "q2", an earlier series the other way round`,
		},
		{
			"ribbon steps differ from periods",
			Block{T: "chart", Title: "C", Kind: "ribbon", Steps: []Step{{Caption: "one"}, {Caption: "two"}}, Series: []ChartSeries{{Name: "a", Points: []ChartPoint{{X: "q1", Y: 1}, {X: "q2", Y: 1}, {X: "q3", Y: 1}}}}},
			`chart "C": a ribbon takes one caption per period (captions: 2, periods: 3)`,
		},
		{
			"ribbon steps without series",
			Block{T: "chart", Title: "C", Kind: "ribbon", Steps: []Step{{Caption: "one"}}},
			`chart "C": a ribbon takes one caption per period (captions: 1, periods: 0)`,
		},
		{"diagram unknown direction", Block{T: "diagram", Caption: "D", Direction: "RL", Nodes: []DiagramNode{{ID: "a", Label: "A"}}}, `diagram "D": unknown direction "RL" (want LR or TB)`},
		{"diagram without nodes", Block{T: "diagram", Caption: "D"}, `diagram "D": needs at least one node`},
		{"diagram id used twice", Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "a", Label: "G"}}, Nodes: []DiagramNode{{ID: "a", Label: "A"}}}, `diagram "D": id "a" is used twice`},
		{"diagram node without id", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: " ", Label: "A"}}}, `diagram "D": node 1 has no id`},
		{"diagram node without label", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a"}}}, `diagram "D": node "a" has no label`},
		{"diagram node unknown kind", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A", Kind: "cloud"}}}, `diagram "D": node "a": unknown kind "cloud" (want one of service, store, queue, person, external)`},
		{"diagram node unknown tone", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A", Tone: "pink"}}}, `diagram "D": node "a": unknown tone "pink" (want one of neutral, green, red, blue, amber, purple)`},
		{"diagram node in a missing group", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A", Group: "g"}}}, `diagram "D": node "a": group "g" is not a group`},
		{"diagram group without label", Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "g"}}, Nodes: []DiagramNode{{ID: "a", Label: "A", Group: "g"}}}, `diagram "D": group "g" has no label`},
		{"diagram group unknown tone", Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "g", Label: "G", Tone: "wg600"}}, Nodes: []DiagramNode{{ID: "a", Label: "A", Group: "g"}}}, `diagram "D": group "g": unknown tone "wg600"`},
		{"diagram empty group", Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "g", Label: "G"}}, Nodes: []DiagramNode{{ID: "a", Label: "A"}}}, `diagram "D": group "g" holds no node and no group`},
		{"diagram group parent is a node", Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "g", Label: "G", Group: "a"}}, Nodes: []DiagramNode{{ID: "a", Label: "A", Group: "g"}}}, `diagram "D": group "g": parent "a" is not a group`},
		{
			"diagram groups nested three deep",
			Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "g1", Label: "G"}, {ID: "g2", Label: "G", Group: "g1"}, {ID: "g3", Label: "G", Group: "g2"}}, Nodes: []DiagramNode{{ID: "a", Label: "A", Group: "g3"}}},
			`diagram "D": group "g3": groups nest at most 2 deep`,
		},
		{
			"diagram groups in a loop",
			Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "g1", Label: "G", Group: "g2"}, {ID: "g2", Label: "G", Group: "g1"}}, Nodes: []DiagramNode{{ID: "a", Label: "A", Group: "g1"}}},
			`groups nest at most 2 deep (or its parents loop)`,
		},
		{"diagram edge to a missing node", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, Edges: []DiagramEdge{{From: "a", To: "c"}}}, `diagram "D": edge 1: "c" is not a node`},
		{"diagram edge to a group", Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "g", Label: "G"}}, Nodes: []DiagramNode{{ID: "a", Label: "A", Group: "g"}}, Edges: []DiagramEdge{{From: "a", To: "g"}}}, `diagram "D": edge 1: "g" is not a node`},
		{"diagram self edge", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, Edges: []DiagramEdge{{From: "a", To: "a"}}}, `diagram "D": edge 1: from and to are both "a"`},
		{"diagram negative weight", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, Edges: []DiagramEdge{{From: "a", To: "b", Weight: -1}}}, `diagram "D": edge 1 (a to b): weight -1, want 0 or more`},
		{"diagram step without caption", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, Steps: []Step{{Caption: " "}}}, `diagram "D": step 1 has no caption`},
		{"diagram focus names nothing", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, Steps: []Step{{Caption: "one", Focus: []string{"zz"}}}}, `diagram "D": step 1: focus "zz" names no node or group`},
		{"diagram node step without steps", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A", Step: 1}}}, `diagram "D": node "a" names step 1 but the diagram has no steps`},
		{"diagram node step out of range", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A", Step: 3}}, Steps: []Step{{Caption: "one"}}}, `diagram "D": node "a" step 3: want 1 to 1`},
		{
			"diagram node before its group",
			Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "g", Label: "G", Step: 2}}, Nodes: []DiagramNode{{ID: "a", Label: "A", Group: "g", Step: 1}}, Steps: []Step{{Caption: "one"}, {Caption: "two"}}},
			`diagram "D": node "a" appears at step 1, before "g" at step 2`,
		},
		{
			"diagram edge before its end",
			Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B", Step: 2}}, Edges: []DiagramEdge{{From: "a", To: "b"}}, Steps: []Step{{Caption: "one"}, {Caption: "two"}}},
			`diagram "D": edge 1 (a to b) appears from the start, before "b" at step 2`,
		},
		{"chart step with focus", Block{T: "chart", Title: "C", Steps: []Step{{Caption: "one", Focus: []string{"a"}}}}, `chart "C": step 1 names focus, which only a diagram takes`},
		{"diagram with 25 nodes", Block{T: "diagram", Caption: "D", Nodes: func() []DiagramNode {
			var ns []DiagramNode
			for i := 0; i < 25; i++ {
				ns = append(ns, DiagramNode{ID: fmt.Sprintf("n%d", i), Label: "N"})
			}
			return ns
		}()}, `diagram "D": 25 nodes, at most 24`},
		{"diagram with 13 groups", Block{T: "diagram", Caption: "D", Groups: func() []DiagramGroup {
			var gs []DiagramGroup
			for i := 0; i < 13; i++ {
				gs = append(gs, DiagramGroup{ID: fmt.Sprintf("g%d", i), Label: "G"})
			}
			return gs
		}(), Nodes: []DiagramNode{{ID: "a", Label: "A"}}}, `diagram "D": 13 groups, at most 12`},
		{"diagram with a frame", Block{T: "diagram", Caption: "D", Frame: new(bool), Nodes: []DiagramNode{{ID: "a", Label: "A"}}}, `diagram "D": frame is a chart or image field`},
		{"diagram without caption is named by its first node", Block{T: "diagram", Nodes: []DiagramNode{{ID: "web", Label: " "}}}, `diagram (first node "web"): node "web" has no label`},
		{"diagram id is not a token", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a b", Label: "A"}}}, `diagram "D": node 1: id "a b" is not a plain token`},
		{"diagram group without id", Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: " ", Label: "G"}}, Nodes: []DiagramNode{{ID: "a", Label: "A"}}}, `diagram "D": group 1 has no id`},
		{"diagram group label blank", Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "g", Label: " "}}, Nodes: []DiagramNode{{ID: "a", Label: "A", Group: "g"}}}, `diagram "D": group "g" has no label`},
		{"diagram node label too long", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: strings.Repeat("x", 33)}}}, `diagram "D": node "a": label is over 32 characters`},
		{"diagram node text too long", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A", Text: strings.Repeat("x", 61)}}}, `diagram "D": node "a": text is over 60 characters`},
		{"diagram group label too long", Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "g", Label: strings.Repeat("x", 33)}}, Nodes: []DiagramNode{{ID: "a", Label: "A", Group: "g"}}}, `diagram "D": group "g": label is over 32 characters`},
		{"diagram edge label too long", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, Edges: []DiagramEdge{{From: "a", To: "b", Label: strings.Repeat("x", 25)}}}, `diagram "D": edge 1 (a to b): label is over 24 characters`},
		{"diagram edge repeated", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, Edges: []DiagramEdge{{From: "a", To: "b"}, {From: "a", To: "b", Label: "again"}}}, `diagram "D": edge 2 repeats edge 1 (a to b)`},
		{"diagram weight without flow", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, Edges: []DiagramEdge{{From: "a", To: "b", Weight: 3}}}, `diagram "D": edge 1 (a to b): weight needs flow`},
		{
			"diagram group before its parent",
			Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "outer", Label: "O", Step: 2}, {ID: "inner", Label: "I", Group: "outer", Step: 1}}, Nodes: []DiagramNode{{ID: "a", Label: "A", Group: "inner", Step: 2}}, Steps: []Step{{Caption: "one"}, {Caption: "two"}}},
			`diagram "D": group "inner" appears at step 1, before "outer" at step 2`,
		},
		{"diagram group step out of range", Block{T: "diagram", Caption: "D", Groups: []DiagramGroup{{ID: "g", Label: "G", Step: 2}}, Nodes: []DiagramNode{{ID: "a", Label: "A", Group: "g", Step: 2}}, Steps: []Step{{Caption: "one"}}}, `diagram "D": group "g" step 2: want 1 to 1`},
		{"diagram edge step out of range", Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, Edges: []DiagramEdge{{From: "a", To: "b", Step: 2}}, Steps: []Step{{Caption: "one"}}}, `diagram "D": edge 1 (a to b) step 2: want 1 to 1`},
		{
			"diagram focus on a hidden element",
			Block{T: "diagram", Caption: "D", Nodes: []DiagramNode{{ID: "a", Label: "A"}, {ID: "b", Label: "B", Step: 2}}, Steps: []Step{{Caption: "one", Focus: []string{"b"}}, {Caption: "two"}}},
			`diagram "D": step 1: focus "b" is not shown until step 2`,
		},
		{"direction on a paragraph", Block{T: "p", Text: "x", Direction: "LR"}, `p block: nodes, groups, edges, and direction are diagram fields`},
		{"frame on a paragraph", Block{T: "p", Text: "x", Frame: new(bool)}, `p block: frame is a chart or image field`},
		{
			"stepped diagram in details",
			Block{T: "details", Summary: "s", Blocks: []Block{{T: "diagram", Nodes: []DiagramNode{{ID: "a", Label: "A"}}, Steps: []Step{{Caption: "one"}}}}},
			`diagram steps: not allowed inside a details block`,
		},
		{
			"stepped chart in details",
			Block{T: "details", Summary: "s", Blocks: []Block{{T: "chart", Steps: []Step{{Caption: "one"}}}}},
			`chart steps: not allowed inside a details block`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := RenderDoc(
				Doc{Sections: []Section{{Heading: "S", Blocks: []Block{c.block}}}},
				"T",
			)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want %q", err, c.want)
			}
		})
	}
}

// The page's one graph may sit in a column, beside a paragraph say, and
// renders there in the one container the page view mounts it in (MAD-374).
func TestRenderDocGraphInColumns(t *testing.T) {
	out := renderBlocks(t, Block{T: "columns", Columns: [][]Block{
		{{T: "graph"}},
		{{T: "p", Text: "beside it"}},
	}})
	graph := strings.Index(out, `<div id="cy-graph" class="cy-container"></div>`)
	col := strings.Index(out, "<wk-col>")
	if graph < 0 || col < 0 || graph < col {
		t.Errorf("graph not rendered inside a column: %s", out)
	}
	if n := strings.Count(out, `id="cy-graph"`); n != 1 {
		t.Errorf("cy-graph count = %d, want 1", n)
	}
}

// A page has one graph, so a Doc that places it twice, at the top level or
// in a column, is refused before anything renders two containers.
func TestRenderDocGraphOncePerPage(t *testing.T) {
	graph := Block{T: "graph"}
	inColumn := Block{T: "columns", Columns: [][]Block{{graph}, {{T: "p", Text: "x"}}}}
	cases := []struct {
		name string
		doc  Doc
	}{
		{
			"twice in one section",
			Doc{Sections: []Section{{Heading: "S", Blocks: []Block{graph, graph}}}},
		},
		{
			"across sections",
			Doc{Sections: []Section{
				{Heading: "S", Blocks: []Block{graph}},
				{Heading: "U", Blocks: []Block{graph}},
			}},
		},
		{
			"top level and in a column",
			Doc{Sections: []Section{{Heading: "S", Blocks: []Block{graph, inColumn}}}},
		},
		{
			"in two columns",
			Doc{Sections: []Section{{Heading: "S", Blocks: []Block{inColumn, inColumn}}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := RenderDoc(c.doc, "T")
			if err == nil || !strings.Contains(err.Error(), "found 2") {
				t.Errorf("err = %v, want the second graph refused", err)
			}
		})
	}
	for _, ok := range []Doc{
		{Sections: []Section{{Heading: "S", Blocks: []Block{graph}}}},
		{Sections: []Section{{Heading: "S", Blocks: []Block{inColumn}}}},
	} {
		if _, err := RenderDoc(ok, "T"); err != nil {
			t.Errorf("one graph refused: %v", err)
		}
	}
}

// A container built in Go that skipped validation stops at the depth guard
// instead of recursing.
func TestRenderBlockAtDepthGuard(t *testing.T) {
	deep := Block{T: "columns", Columns: [][]Block{{{T: "p", Text: "x"}}, {{T: "p", Text: "y"}}}}
	if out := string(renderBlockAt(deep, 1)); !strings.Contains(
		out,
		"render error: columns block nested too deep",
	) {
		t.Errorf("container at depth 1: %s", out)
	}
	if out := string(renderBlockAt(Block{T: "p", Text: "x"}, 2)); !strings.Contains(
		out,
		"nested too deep",
	) {
		t.Errorf("block past max depth: %s", out)
	}
}

// cols is one wire key for two shapes: a table's header strings and a
// columns block's arrays of blocks. Both round-trip through the canonical
// JSON, and a table keeps the key where it was.
func TestBlockColsJSONRoundTrip(t *testing.T) {
	src := `{"sections":[{"h":"S","blocks":[` +
		`{"t":"table","cols":["A","B"],"rows":[["1","2"]]},` +
		`{"t":"columns","cols":[[{"t":"p","text":"l"}],[{"t":"stat","value":"1","label":"x"}]]}` +
		`]}]}`
	c, err := Compile([]byte(src), "T")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	blocks := c.Doc.Sections[0].Blocks
	if got := blocks[0].Cols; len(got) != 2 || got[0] != "A" {
		t.Errorf("table cols = %v", got)
	}
	if blocks[0].Columns != nil {
		t.Errorf("table decoded Columns: %v", blocks[0].Columns)
	}
	if got := blocks[1].Columns; len(got) != 2 || got[1][0].T != "stat" || got[1][0].Value != "1" {
		t.Errorf("columns = %+v", got)
	}
	if blocks[1].Cols != nil {
		t.Errorf("columns block decoded Cols: %v", blocks[1].Cols)
	}
	if !strings.Contains(string(c.JSON), `{"t":"table","cols":["A","B"],"rows":[["1","2"]]}`) {
		t.Errorf("table JSON moved: %s", c.JSON)
	}
	if !strings.Contains(
		string(c.JSON),
		`{"t":"columns","cols":[[{"t":"p","text":"l"}],[{"t":"stat","value":"1","label":"x"}]]}`,
	) {
		t.Errorf("columns JSON: %s", c.JSON)
	}
	var again Doc
	if err := json.Unmarshal(c.JSON, &again); err != nil {
		t.Fatalf("re-parse canonical JSON: %v", err)
	}
	if again.Sections[0].Blocks[1].Columns[0][0].Text != "l" {
		t.Errorf("canonical JSON lost the columns: %s", c.JSON)
	}
	if _, err := Compile([]byte(`{"sections":[{"h":"S","blocks":[{"t":"table","cols":[["x"]]}]}]}`), "T"); err == nil {
		t.Error("table with nested cols accepted")
	}
	if _, err := Compile([]byte(`{"sections":[{"h":"S","blocks":[{"t":"columns","cols":["x","y"]}]}]}`), "T"); err == nil {
		t.Error("columns with string cols accepted")
	}
}

func TestRenderDocDeckChrome(t *testing.T) {
	plain, err := RenderDeck(
		Doc{Meta: "m", Sections: []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x"}}}}},
		"T",
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "deck-chrome") || strings.Contains(plain, "brief-presenter") {
		t.Errorf("chrome emitted for a Doc without chrome fields: %s", plain)
	}
	if !strings.HasPrefix(plain, "<h1 ") {
		t.Errorf("fragment without chrome must start with the title: %q", plain[:40])
	}

	withChrome := Doc{
		Meta:         "2026-10-01 · review",
		Presenter:    "Alex, platform",
		Footer:       "Incident review",
		Logo:         "https://example.com/mark.png",
		LogoPosition: "top-right",
		Progress:     "bar",
		Sections:     []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x"}}}},
	}
	// The brief ignores the chrome fields: no island, no byline, the same
	// bytes as a Doc without them.
	brief, err := RenderDoc(withChrome, "T")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(brief, "deck-chrome") || strings.Contains(brief, "brief-presenter") {
		t.Errorf("the brief rendered deck chrome: %s", brief)
	}
	bare := withChrome
	bare.Presenter, bare.Footer, bare.Logo, bare.LogoPosition, bare.Progress = "", "", "", "", ""
	if want, _ := RenderDoc(bare, "T"); brief != want {
		t.Errorf(
			"brief with chrome fields differs from the brief without them:\n%s\n%s",
			brief,
			want,
		)
	}

	out, err := RenderDeck(withChrome, "T")
	if err != nil {
		t.Fatal(err)
	}
	want := `<script type="application/json" class="deck-chrome">` +
		`{"logo":"https://example.com/mark.png","logo_position":"top-right","progress":"bar","presenter":"Alex, platform","footer":"Incident review"}` +
		`</script>` + "\n<h1 "
	if !strings.HasPrefix(out, want) {
		t.Errorf("chrome island:\n got %q\nwant prefix %q", out, want)
	}
	if !strings.Contains(
		out,
		"<div class=\"brief-meta\">2026-10-01 · review</div>\n<div class=\"brief-presenter\">Alex, platform</div>",
	) {
		t.Errorf("presenter line not under the meta line: %s", out)
	}
	if strings.Count(out, "data-fixation") != 3 {
		t.Errorf("presenter line must stay out of the fixation walk: %s", out)
	}

	// One field alone is enough for the island, and only that key is written.
	out, err = RenderDeck(
		Doc{
			Logo:     "none",
			Sections: []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x"}}}},
		},
		"T",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(
		out,
		`<script type="application/json" class="deck-chrome">{"logo":"none"}</script>`,
	) {
		t.Errorf("logo none island: %s", out)
	}
}

// A chrome string must never break out of its script island.
func TestRenderDocDeckChromeScriptSafe(t *testing.T) {
	out, err := RenderDeck(Doc{
		Footer:   "</script><script>alert(1)</script>",
		Sections: []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x"}}}},
	}, "T")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "<script") != 1 || strings.Contains(out, "<script>alert") {
		t.Errorf("footer broke out of the island: %s", out)
	}
}

func TestRenderDocDeckChromeValidation(t *testing.T) {
	cases := []struct {
		name string
		doc  Doc
		want string
	}{
		{"position", Doc{LogoPosition: "middle"}, `logo_position "middle"`},
		{"progress", Doc{Progress: "ring"}, `progress "ring"`},
		{"logo scheme", Doc{Logo: "javascript:alert(1)"}, `logo "javascript:alert(1)"`},
		{"logo relative", Doc{Logo: "/assets/x.png"}, `logo "/assets/x.png"`},
		{"logo data", Doc{Logo: "data:image/png;base64,AAAA"}, "logo "},
		{"logo no host", Doc{Logo: "https:///x.png"}, "logo "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.doc.Sections = []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x"}}}}
			_, err := RenderDoc(c.doc, "T")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want %q", err, c.want)
			}
		})
	}
	for _, ok := range []Doc{
		{Logo: "none"},
		{Logo: "http://example.com/a.png"},
		{Logo: "HTTPS://example.com/a.png"},
		{LogoPosition: "bottom-left"},
		{LogoPosition: "top-left"},
		{Progress: "dots"},
		{Progress: "none"},
	} {
		ok.Sections = []Section{{Heading: "S", Blocks: []Block{{T: "p", Text: "x"}}}}
		if _, err := RenderDoc(ok, "T"); err != nil {
			t.Errorf("%+v: %v", ok, err)
		}
	}
}

// The chrome fields ride through Compile's re-marshal, so a stored deck
// keeps them, and a Doc without them marshals as before.
func TestCompileKeepsDeckChrome(t *testing.T) {
	c, err := Compile(
		[]byte(`{"logo":"none","progress":"bar","presenter":"A","footer":"F","sections":[]}`),
		"T",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(c.JSON); got != `{"sections":[],"logo":"none","progress":"bar","presenter":"A","footer":"F"}` {
		t.Errorf("canonical JSON = %s", got)
	}
	if c.Doc.Logo != "none" || c.Doc.Progress != "bar" {
		t.Errorf("doc = %+v", c.Doc)
	}
	if _, err := Compile([]byte(`{"progress":"ring","sections":[]}`), "T"); err == nil {
		t.Error("Compile accepted an unknown progress value")
	}
}

// CompileDeck renders the chrome the brief's Compile leaves out, over the
// same canonical JSON.
func TestCompileDeckRendersChrome(t *testing.T) {
	deck, err := CompileDeck([]byte(`{"presenter":"A","sections":[]}`), "T")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(deck.HTML, `<div class="brief-presenter">A</div>`) ||
		!strings.Contains(deck.HTML, "deck-chrome") {
		t.Errorf("CompileDeck did not render the chrome: %s", deck.HTML)
	}
	if string(deck.JSON) != `{"sections":[],"presenter":"A"}` {
		t.Errorf("CompileDeck canonical JSON = %s", deck.JSON)
	}
	brief, err := Compile([]byte(`{"presenter":"A","sections":[]}`), "T")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(brief.HTML, "brief-presenter") ||
		strings.Contains(brief.HTML, "deck-chrome") {
		t.Errorf("Compile rendered deck chrome in the brief: %s", brief.HTML)
	}
}

// A stat's value is shown verbatim: the symbols a figure carries are the
// point. The label and the sub line are prose and normalise like any other.
func TestRenderDocStatValueVerbatim(t *testing.T) {
	out := renderBlocks(
		t,
		Block{T: "stat", Value: "4×", Label: "faster & cheaper", Subtitle: "A → B"},
		Block{T: "stat", Value: "≈ 40%", Label: "of requests"},
	)
	for _, want := range []string{
		`<wk-stat-value>4×</wk-stat-value>`, `<wk-stat-value>≈ 40%</wk-stat-value>`,
		`faster and cheaper`, `<wk-stat-sub data-fixation>A to B</wk-stat-sub>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
	for _, bad := range []string{"4times", "approximately 40%"} {
		if strings.Contains(out, bad) {
			t.Errorf("stat value was normalised to %q: %s", bad, out)
		}
	}
}

func renderSections(t *testing.T, d Doc) string {
	t.Helper()
	out, err := RenderDoc(d, "T")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	return out
}

func oneSection(s Section) Doc {
	if s.Blocks == nil {
		s.Blocks = []Block{{T: "p", Text: "x"}}
	}
	return Doc{Sections: []Section{s}}
}

// A section's layout, tone, and reveal ride as attributes on wk-section,
// emitted only when set (and layout only when not the default), and the
// tone also sets the --slide-accent variable as a var() of the role, never
// a literal.
func TestRenderDocSectionLayoutFields(t *testing.T) {
	out := renderSections(
		t,
		oneSection(Section{Heading: "S", Layout: "statement", Tone: "blue", Reveal: true}),
	)
	want := `<wk-section id="s" data-layout="statement" data-tone="blue" style="--slide-accent: var(--blue)" data-reveal="true">`
	if !strings.Contains(out, want) {
		t.Errorf("missing %q in: %s", want, out)
	}
	for _, layout := range []string{"", "default"} {
		out := renderSections(t, oneSection(Section{Heading: "S", Layout: layout}))
		if !strings.Contains(out, `<wk-section id="s">`) {
			t.Errorf("layout %q: attributes emitted: %s", layout, out)
		}
	}
	for _, layout := range []string{"center", "section"} {
		out := renderSections(t, oneSection(Section{Heading: "S", Layout: layout}))
		if !strings.Contains(out, `data-layout="`+layout+`"`) {
			t.Errorf("layout %q not emitted: %s", layout, out)
		}
	}
	for _, tone := range []string{"primary", "green", "series-2", "tone-amber-bg", "paper"} {
		out := renderSections(t, oneSection(Section{Heading: "S", Tone: tone}))
		if !strings.Contains(out, `data-tone="`+tone+`" style="--slide-accent: var(--`+tone+`)"`) {
			t.Errorf("tone %q: %s", tone, out)
		}
	}
}

// Notes render as an inert template at the end of the section: inline
// markdown inside, no data-fixation, and outside the read-aloud walk by
// construction (a template's content is not among its child nodes).
func TestRenderDocSectionNotes(t *testing.T) {
	out := renderSections(t, oneSection(Section{
		Heading: "S", Notes: "Pause here & ask **who** owns the rota.",
		Blocks: []Block{{T: "p", Text: "x"}},
	}))
	want := "<p data-fixation>x</p>\n  <template class=\"deck-notes\">Pause here and ask <strong>who</strong> owns the rota.</template>\n</wk-section>"
	if !strings.Contains(out, want) {
		t.Errorf("notes:\n got %s\nwant to contain %s", out, want)
	}
	if n := strings.Count(out, "data-fixation"); n != 3 {
		t.Errorf(
			"data-fixation count = %d, want 3 (title, heading, paragraph); notes must not be fixated",
			n,
		)
	}
	if strings.Contains(renderSections(t, oneSection(Section{Heading: "S"})), "deck-notes") {
		t.Error("a section without notes emitted a notes template")
	}
}

func TestRenderDocSectionValidation(t *testing.T) {
	cases := []struct {
		name string
		sec  Section
		want string
	}{
		{"layout", Section{Heading: "S", Layout: "split"}, `section "S": unknown layout "split"`},
		{"tone", Section{Heading: "S", Tone: "wg600"}, `section "S": unknown tone "wg600"`},
		{"tone alias", Section{Heading: "S", Tone: "terracotta"}, `unknown tone "terracotta"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := RenderDoc(oneSection(c.sec), "T")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want %q", err, c.want)
			}
		})
	}
}

// The transition travels in the chrome island only when set and not fade,
// the default the deck view assumes; a deck that names fade or nothing
// keeps the bytes it had.
func TestRenderDocTransition(t *testing.T) {
	base := oneSection(Section{Heading: "S"})
	for _, tr := range []string{"", "fade"} {
		d := base
		d.Transition = tr
		if out, _ := RenderDeck(d, "T"); strings.Contains(out, "deck-chrome") {
			t.Errorf("transition %q emitted an island: %s", tr, out)
		}
	}
	for _, tr := range []string{"slide", "none"} {
		d := base
		d.Transition = tr
		out, err := RenderDeck(d, "T")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(
			out,
			`<script type="application/json" class="deck-chrome">{"transition":"`+tr+`"}</script>`,
		) {
			t.Errorf("transition %q: %s", tr, out)
		}
	}
	d := base
	d.Transition = "zoom"
	if _, err := RenderDoc(d, "T"); err == nil ||
		!strings.Contains(err.Error(), `transition "zoom"`) {
		t.Errorf("err = %v, want unknown transition", err)
	}
	c, err := Compile(
		[]byte(
			`{"transition":"slide","sections":[{"h":"S","layout":"center","tone":"green","notes":"n","reveal":true,"blocks":[]}]}`,
		),
		"T",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(c.JSON); got != `{"sections":[{"h":"S","blocks":[],"layout":"center","tone":"green","notes":"n","reveal":true}],"transition":"slide"}` {
		t.Errorf("canonical JSON = %s", got)
	}
}

// The sample deck under testdata exercises every layout, tone, notes,
// reveal, block type, and chrome field; the skill and the browser check use
// it, so it must always compile.
func TestSampleDeckRenders(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "sample-deck.json"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := CompileDeck(raw, "Sample deck")
	if err != nil {
		t.Fatalf("CompileDeck sample deck: %v", err)
	}
	for _, want := range []string{
		`"transition":"slide"`, `"presenter":"Alex, platform · 2026-10-01"`,
		`data-layout="statement"`, `data-layout="section"`, `data-layout="center"`,
		`data-tone="blue" style="--slide-accent: var(--blue)"`, `data-tone="amber"`, `data-tone="green"`, `data-tone="tone-neutral-bg"`,
		`data-reveal="true"`, `<template class="deck-notes">`,
		`<wk-columns cols="3">`, `<wk-stat-value>41 min</wk-stat-value>`, `<blockquote>`, `<details>`,
		`<wk-callout variant="ok"`, `<wk-callout variant="error"`, `class="present-chart"`, `id="cy-graph"`,
		`<wk-kv>`, `class="language-bash"`,
		`"logo":"https://raw.githubusercontent.com/mad01/thismoon/main/docs/assets/logo.png"`,
		`"logo_position":"top-right"`, `"progress":"dots"`, `"footer":"Incident review, checkout 502s"`,
		`<wk-section-subheading data-fixation>Per region</wk-section-subheading>`, `<wk-table><table>`,
		`<wk-progress>`, `<wk-panel style="border-left: 3px solid var(--red)">`,
		`<wk-callout variant="info"`, `<wk-callout variant="warn"`, `<p class="brief-meta">Figures as of`,
		`<a href="https://example.com/runbook">runbook</a>`,
		`<wk-figure>`, `alt="The on-call dashboard at 14:15, every checkout panel red" loading="lazy">`,
		`<wk-figcaption data-fixation>The on-call dashboard at 14:15.</wk-figcaption>`,
		`data-steps="3"`, `<ol class="present-steps">`, `<li data-fixation>The pricing service times out first.</li>`,
		`class="present-chart is-ribbon"`, `data-steps="4"`,
		`class="present-diagram" data-steps="4"`, `<p class="present-diagram-caption" data-fixation>`,
	} {
		if !strings.Contains(c.HTML, want) {
			t.Errorf("sample deck lacks %q", want)
		}
	}
	if n := len(c.Doc.Sections); n != 17 {
		t.Errorf("sample deck has %d sections, want 17", n)
	}
	// The four reveal sections: two lists, the mixed slide, and the stepped
	// chart that walks its steps after its paragraph (MAD-385).
	if n := strings.Count(c.HTML, `data-reveal="true"`); n != 4 {
		t.Errorf("reveal sections = %d, want 4", n)
	}
	// The four columns blocks: the row of figures, the chart beside its
	// caption, the graph beside a paragraph (MAD-374), and the image beside
	// its paragraph (MAD-375).
	if n := strings.Count(c.HTML, "<wk-columns "); n != 4 {
		t.Errorf("columns blocks = %d, want 4", n)
	}
}

// TestRenderDocDiagramsManyPerPage keeps diagrams outside the one-graph
// rule: two diagrams beside the page's one graph compile.
func TestRenderDocDiagramsManyPerPage(t *testing.T) {
	diagram := Block{T: "diagram", Nodes: []DiagramNode{{ID: "a", Label: "A"}}}
	doc := Doc{Sections: []Section{
		{Heading: "S", Blocks: []Block{diagram, {T: "graph"}}},
		{Heading: "U", Blocks: []Block{{T: "columns", Columns: [][]Block{{diagram}, {{T: "p", Text: "x"}}}}}},
	}}
	out, err := RenderDoc(doc, "T")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	if n := strings.Count(out, `class="present-diagram"`); n != 2 {
		t.Errorf("present-diagram appears %d times, want 2", n)
	}
}
