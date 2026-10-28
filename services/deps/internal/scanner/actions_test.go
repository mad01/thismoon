package scanner

import (
	"context"
	"testing"

	"github.com/mad01/thismoon/services/deps/internal/osv"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// queryRecorder is a Checker that records the queries it gets and finds
// nothing.
type queryRecorder struct{ got []osv.Query }

func (r *queryRecorder) Check(_ context.Context, queries []osv.Query) ([][]store.Advisory, error) {
	r.got = append(r.got, queries...)
	return make([][]store.Advisory, len(queries)), nil
}

// TestCheckQueriesActionsByCommit: a GitHub Action is checked by the commit
// its version carries, with the label as the version; other ecosystems keep
// their version query.
func TestCheckQueriesActionsByCommit(t *testing.T) {
	const sha = "a284dc1814e3fd07f2e34267fc8f81227ed29fb8"
	deps := []store.Dependency{
		{Ecosystem: "GitHub Actions", Name: "tj-actions/changed-files", Version: "v45.0.7+" + sha},
		{Ecosystem: "GitHub Actions", Name: "actions/checkout", Version: sha},
		{Ecosystem: "npm", Name: "left-pad", Version: "1.2.0+build"},
	}
	rec := &queryRecorder{}
	if _, err := Check(context.Background(), deps, rec); err != nil {
		t.Fatalf("Check: %v", err)
	}
	want := []osv.Query{
		{
			Name:      "tj-actions/changed-files",
			Ecosystem: "GitHub Actions",
			Version:   "v45.0.7",
			Commit:    sha,
		},
		{Name: "actions/checkout", Ecosystem: "GitHub Actions", Commit: sha},
		{Name: "left-pad", Ecosystem: "npm", Version: "1.2.0+build"},
	}
	if len(rec.got) != len(want) {
		t.Fatalf("queries = %+v, want %+v", rec.got, want)
	}
	for i := range want {
		if rec.got[i] != want[i] {
			t.Errorf("query %d = %+v, want %+v", i, rec.got[i], want[i])
		}
	}
}
