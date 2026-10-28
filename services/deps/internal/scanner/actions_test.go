package scanner

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/deps/internal/discover"
	"github.com/mad01/thismoon/services/deps/internal/osv"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// namesDownChecker finds GHSA-1 on every query and reports that OSV's GitHub
// Actions name list could not be read, as *osv.Client does beside complete
// results.
type namesDownChecker struct{}

func (namesDownChecker) Check(_ context.Context, queries []osv.Query) ([][]store.Advisory, error) {
	out := make([][]store.Advisory, len(queries))
	for i := range out {
		out[i] = []store.Advisory{{ID: "GHSA-1", Kind: store.KindVuln}}
	}
	return out, &osv.ActionNamesError{Err: errors.New("name list answered 503")}
}

// TestNameListFailureIsADiscoveryError: a check that could not read OSV's
// name list keeps its findings, and each repo with an Action gets a GitHub
// Actions discovery error, on a full check and on a one-repo rescan, so the
// scan never reads as clean. A repo with no Action gets none.
func TestNameListFailureIsADiscoveryError(t *testing.T) {
	const sha = "a284dc1814e3fd07f2e34267fc8f81227ed29fb8"
	st := newStore(t)
	e := &Engine{
		Prepare: func() (Plan, error) {
			return Plan{Repos: []string{"/r1", "/r2"}}, nil
		},
		Ecosystems: []discover.Ecosystem{
			stubEco{name: "GitHub Actions", pkgs: []discover.Package{
				{Ecosystem: "GitHub Actions", Name: "azure/setup-kubectl", Version: "v1+" + sha},
				{Ecosystem: "GitHub Actions", Name: "actions/checkout", Version: sha},
			}},
		},
		Checker: namesDownChecker{},
		Store:   st,
	}

	deps, err := e.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(deps) != 4 {
		t.Fatalf("deps = %+v, want two actions in each repo", deps)
	}
	for _, d := range deps {
		if !d.Flagged() {
			t.Errorf("%s in %s lost its finding", d.Name, d.Repo)
		}
	}
	wantErrs := func(when string) {
		t.Helper()
		got := st.DiscoveryErrors()
		byRepo := func(a, b store.DiscoveryError) int { return strings.Compare(a.Repo, b.Repo) }
		slices.SortFunc(got, byRepo)
		if len(got) != 2 {
			t.Fatalf("%s: discovery errors = %+v, want one per repo", when, got)
		}
		for i, repo := range []string{"/r1", "/r2"} {
			if got[i].Repo != repo || got[i].Ecosystem != "GitHub Actions" ||
				!strings.Contains(got[i].Error, "name list answered 503") {
				t.Errorf("%s: errs[%d] = %+v, want the error in %s", when, i, got[i], repo)
			}
		}
	}
	wantErrs("full check")

	if _, err := e.CheckRepo(context.Background(), "/r1"); err != nil {
		t.Fatalf("CheckRepo: %v", err)
	}
	wantErrs("rescan")

	_, discErrs, err := Check(context.Background(), []store.Dependency{
		{Repo: "/r3", Ecosystem: "npm", Name: "left-pad", Version: "1.0.0"},
	}, namesDownChecker{})
	if err != nil || len(discErrs) != 0 {
		t.Errorf("npm-only check = %v, %+v; want no discovery error", err, discErrs)
	}
}

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
	if _, _, err := Check(context.Background(), deps, rec); err != nil {
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
