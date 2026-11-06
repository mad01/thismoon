package library

import (
	"context"
	"maps"
	"testing"
)

func similarTo(t *testing.T, lib *Library, ref string) map[string]string {
	t.Helper()
	r, err := lib.Locate(nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	report, err := lib.Similar(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range report.Similar {
		got[s.Vault+":"+s.Path] = s.Match
	}
	return got
}

func TestSimilarFindsTitlesAliasesAndContent(t *testing.T) {
	lib, f := duplicateFixture(t, true)
	v := personal(t, lib)
	f.write("notes/pool-rollouts.md", "---\ntitle: Pool Rollouts\n---\nShort.\n")
	f.write("notes/zonal.md", "---\ntitle: Zonal Rollout\n---\n"+duplicateBody+"\n")
	f.write(
		"notes/copy.md",
		"---\ntitle: Node Pool Upgrades\nid: 01JC0000000000000000000001\n---\nx\n",
	)
	f.write("notes/espresso.md", "---\ntitle: Espresso\n---\nGrind finer for a slower shot.\n")

	// Without an index only titles and aliases match. The note's own id
	// copy (a promotion in flight) is the note, not a look-alike.
	want := map[string]string{"personal:notes/pool-rollouts.md": MatchTitle}
	if got := similarTo(t, lib, "personal:notes/node-pools.md"); !maps.Equal(got, want) {
		t.Errorf("similar before indexing = %v, want %v", got, want)
	}

	if _, err := lib.RefreshSemantic(context.Background(), v, true, nil); err != nil {
		t.Fatal(err)
	}
	want["personal:notes/zonal.md"] = MatchSemantic
	if got := similarTo(t, lib, "personal:notes/node-pools.md"); !maps.Equal(got, want) {
		t.Errorf("similar after indexing = %v, want %v", got, want)
	}
	if got := similarTo(t, lib, "personal:notes/espresso.md"); len(got) != 0 {
		t.Errorf("an unrelated note is like %v", got)
	}
	got := similarTo(t, lib, "team:notes/guide.md")
	if len(got) != 0 {
		t.Errorf("similar of the team guide = %v", got)
	}
}
