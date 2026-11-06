package vault

import (
	"maps"
	"slices"
	"testing"
)

const (
	idA = "01JAAAAAAAAAAAAAAAAAAAAAAA"
	idT = "01JTTTTTTTTTTTTTTTTTTTTTTT"
)

// graphFixture is a private personal vault and a shared team vault whose
// notes link within and across the two.
func graphFixture(t *testing.T) *Set {
	t.Helper()
	return newTestSet(
		t,
		map[string]string{
			"notes/a.md": "---\ntitle: A\nid: " + idA + "\ntags: [x]\n---\n" +
				"[[b]] [[team-guide]] [[ghost]] [[a]] [[b|again]]\n",
			"notes/b.md": "---\ntitle: B\nstatus: validated\n---\n[[A]] [[Ghost]]\n",
		},
		map[string]string{
			"notes/team-guide.md": "---\ntitle: Team Guide\nid: " + idT + "\nreview_by: 2026-12-01\n---\n" +
				"[[a]] is private, so this link dangles.\n",
		},
	)
}

type edgeKey struct{ source, target, kind string }

func graphEdges(g Graph) map[edgeKey]bool {
	out := map[edgeKey]bool{}
	for _, e := range g.Edges {
		out[edgeKey{e.Source, e.Target, e.Kind}] = e.Resolved
	}
	return out
}

func graphDegrees(g Graph) map[string]int {
	out := map[string]int{}
	for _, n := range g.Nodes {
		out[n.ID] = n.Degree
	}
	return out
}

func TestGraph(t *testing.T) {
	const (
		a     = "id:" + idA
		b     = "personal:notes/b.md"
		guide = "id:" + idT
		ghost = "dangling:ghost"
		priv  = "dangling:a"
	)
	set := graphFixture(t)
	personal, _ := set.Get("personal")
	center := Ref{Vault: personal, Path: "notes/b.md"}
	tests := []struct {
		name      string
		opts      GraphOptions
		wantEdges map[edgeKey]bool
		wantDeg   map[string]int
	}{
		{
			name: "every vault",
			wantEdges: map[edgeKey]bool{
				{a, b, EdgeLink}:        true,
				{a, guide, EdgeLink}:    true,
				{a, ghost, EdgeLink}:    false,
				{b, a, EdgeLink}:        true,
				{b, ghost, EdgeLink}:    false,
				{guide, priv, EdgeLink}: false,
			},
			wantDeg: map[string]int{a: 4, b: 3, guide: 2, ghost: 2, priv: 1},
		},
		{
			name:      "the shared vault alone",
			opts:      GraphOptions{Sources: []string{"team"}},
			wantEdges: map[edgeKey]bool{{guide, priv, EdgeLink}: false},
			wantDeg:   map[string]int{guide: 1, priv: 1},
		},
		{
			name: "the private vault keeps its shared targets",
			opts: GraphOptions{Sources: []string{"personal"}},
			wantEdges: map[edgeKey]bool{
				{a, b, EdgeLink}:     true,
				{a, guide, EdgeLink}: true,
				{a, ghost, EdgeLink}: false,
				{b, a, EdgeLink}:     true,
				{b, ghost, EdgeLink}: false,
			},
			wantDeg: map[string]int{a: 4, b: 3, guide: 1, ghost: 2},
		},
		{
			name: "around one note",
			opts: GraphOptions{Center: &center},
			wantEdges: map[edgeKey]bool{
				{a, b, EdgeLink}:     true,
				{a, ghost, EdgeLink}: false,
				{b, a, EdgeLink}:     true,
				{b, ghost, EdgeLink}: false,
			},
			wantDeg: map[string]int{a: 3, b: 3, ghost: 2},
		},
		{
			name: "two hops reach the shared note",
			opts: GraphOptions{Center: &center, Depth: 2, Sources: []string{"personal"}},
			wantEdges: map[edgeKey]bool{
				{a, b, EdgeLink}:     true,
				{a, guide, EdgeLink}: true,
				{a, ghost, EdgeLink}: false,
				{b, a, EdgeLink}:     true,
				{b, ghost, EdgeLink}: false,
			},
			wantDeg: map[string]int{a: 4, b: 3, guide: 1, ghost: 2},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, err := set.Graph(tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := graphEdges(g); !maps.Equal(got, tc.wantEdges) {
				t.Errorf("edges = %v\nwant %v", got, tc.wantEdges)
			}
			if got := graphDegrees(g); !maps.Equal(got, tc.wantDeg) {
				t.Errorf("degrees = %v, want %v", got, tc.wantDeg)
			}
		})
	}
}

func TestGraphNodes(t *testing.T) {
	g, err := graphFixture(t).Graph(GraphOptions{TagsAsNodes: true})
	if err != nil {
		t.Fatal(err)
	}
	nodes := map[string]GraphNode{}
	for _, n := range g.Nodes {
		nodes[n.ID] = n
	}
	guide := nodes["id:"+idT]
	if guide.Vault != "team" || guide.Path != "notes/team-guide.md" || guide.Stem != "team-guide" ||
		guide.Tier != "notes" || guide.ReviewBy != "2026-12-01" || guide.Dangling {
		t.Errorf("guide node = %+v", guide)
	}
	if ghost := nodes["dangling:ghost"]; !ghost.Dangling || ghost.Stem != "ghost" {
		t.Errorf("ghost node = %+v, want one dangling node for [[ghost]] and [[Ghost]]", ghost)
	}
	if _, ok := graphEdges(g)[edgeKey{"id:" + idA, "tag:x", EdgeTag}]; !ok {
		t.Errorf("edges %v lack the tag edge", g.Edges)
	}
	for i := 1; i < len(g.Nodes); i++ {
		if g.Nodes[i-1].ID >= g.Nodes[i].ID {
			t.Fatalf("nodes not sorted by id: %s before %s", g.Nodes[i-1].ID, g.Nodes[i].ID)
		}
	}
}

func TestGraphDuplicateIDsKeepNotesApart(t *testing.T) {
	const dup = "01JDUPDUPDUPDUPDUPDUPDUPDU"
	set := newTestSet(t, map[string]string{
		"notes/one.md": "---\nid: " + dup + "\n---\n[[two]]\n",
		"notes/two.md": "---\nid: " + dup + "\n---\n",
	}, nil)
	g, err := set.Graph(GraphOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[edgeKey]bool{{"personal:notes/one.md", "personal:notes/two.md", EdgeLink}: true}
	if got := graphEdges(g); !maps.Equal(got, want) {
		t.Errorf("edges = %v, want %v", got, want)
	}
}

func TestGraphEdgeKinds(t *testing.T) {
	const (
		idOld  = "01JOLDOLDOLDOLDOLDOLDOLDOL"
		idNew  = "01JNEWNEWNEWNEWNEWNEWNEWNE"
		oldN   = "id:" + idOld
		newN   = "id:" + idNew
		moc    = "personal:MOCs/topics.md"
		claim  = "personal:notes/claim.md"
		ghost  = "dangling:ghost"
		shared = "team:notes/base.md"
	)
	set := newTestSet(t,
		map[string]string{
			"notes/old.md": "---\ntitle: Old\nid: " + idOld + "\nstatus: superseded\n" +
				"superseded_by: id:" + idNew + "\n---\nbody\n",
			"notes/new.md": "---\ntitle: New\nid: " + idNew + "\n---\nbody\n",
			"notes/claim.md": "---\ntitle: Claim\nsources:\n" +
				"  - ref: notes/old.md\n" +
				"  - ref: team:notes/base.md\n" +
				"  - ref: github.com/o/r:main.go@abc123\n" +
				"  - ref: notes/missing.md\n" +
				"  - ref: notes/claim.md\n" +
				"---\n[[new]]\n",
			"MOCs/topics.md": "---\ntitle: Topics\n---\n[[claim]] [[ghost]]\n",
		},
		map[string]string{"notes/base.md": "---\ntitle: Base\n---\nbody\n"},
	)
	g, err := set.Graph(GraphOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[edgeKey]bool{
		{oldN, newN, EdgeSupersedes}: true,
		{claim, oldN, EdgeSource}:    true,
		{claim, shared, EdgeSource}:  true,
		{claim, newN, EdgeLink}:      true,
		{moc, claim, EdgeMOC}:        true,
		{moc, ghost, EdgeMOC}:        false,
	}
	if got := graphEdges(g); !maps.Equal(got, want) {
		t.Errorf("edges = %v\nwant %v", got, want)
	}
	wantDeg := map[string]int{oldN: 2, newN: 2, claim: 4, shared: 1, moc: 2, ghost: 1}
	if got := graphDegrees(g); !maps.Equal(got, wantDeg) {
		t.Errorf("degrees = %v, want %v", got, wantDeg)
	}
}

func TestSnapshotLinks(t *testing.T) {
	set := graphFixture(t)
	sn, err := set.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	personal, _ := set.Get("personal")
	team, _ := set.Get("team")
	body := []byte("[[b|Bee]] [[Team Guide]] [[ghost]] [[#local]] `[[in code]]`\n")
	want := []NoteLink{
		{Text: "[[b|Bee]]", Target: "personal:notes/b.md", Resolved: true},
		{Text: "[[Team Guide]]", Target: "id:" + idT, Resolved: true},
		{Text: "[[ghost]]", Target: "dangling:ghost"},
	}
	if got := sn.Links(personal, body); !slices.Equal(got, want) {
		t.Errorf("links = %+v\nwant %+v", got, want)
	}
	// A shared vault never resolves into a private one.
	got := sn.Links(team, []byte("[[a]]"))
	if len(got) != 1 || got[0].Resolved || got[0].Target != "dangling:a" {
		t.Errorf("shared links = %+v, want [[a]] dangling", got)
	}
	if got := sn.Links(&Vault{Name: "elsewhere"}, []byte("[[b]]")); len(got) != 1 ||
		got[0].Resolved {
		t.Errorf("links from a vault outside the snapshot = %+v, want dangling", got)
	}
}

func TestSnapshotGraphMatchesSetGraph(t *testing.T) {
	set := graphFixture(t)
	sn, err := set.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	opts := GraphOptions{TagsAsNodes: true}
	fromSet, err := set.Graph(opts)
	if err != nil {
		t.Fatal(err)
	}
	fromSnapshot, err := sn.Graph(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(fromSet.Nodes, fromSnapshot.Nodes, func(a, b GraphNode) bool {
		return a.ID == b.ID && a.Degree == b.Degree
	}) || !slices.Equal(fromSet.Edges, fromSnapshot.Edges) {
		t.Errorf("snapshot graph %+v differs from set graph %+v", fromSnapshot, fromSet)
	}
}
