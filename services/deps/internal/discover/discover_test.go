package discover

import (
	"errors"
	"testing"
)

// stubEco is an Ecosystem that returns fixed packages or a fixed error.
type stubEco struct {
	name string
	pkgs []Package
	err  error
}

func (s stubEco) Name() string { return s.name }

func (s stubEco) Discover(string, Options) ([]Package, error) { return s.pkgs, s.err }

func TestReposRecordsEachFailureAndKeepsTheRest(t *testing.T) {
	ecos := []Ecosystem{
		stubEco{
			name: "Go",
			err:  errors.New(`go list in /r1: exec: "go": executable file not found in $PATH`),
		},
		stubEco{
			name: "npm",
			pkgs: []Package{{Ecosystem: "npm", Name: "lodash", Version: "4.17.21", Imported: true}},
		},
	}

	deps, errs := Repos([]string{"/r1", "/r2"}, ecos, Options{})

	if len(deps) != 2 {
		t.Errorf("deps = %d, want the npm package from both repos", len(deps))
	}
	if len(errs) != 2 {
		t.Fatalf("discovery errors = %+v, want one per repo for Go", errs)
	}
	for i, repo := range []string{"/r1", "/r2"} {
		if errs[i].Repo != repo || errs[i].Ecosystem != "Go" || errs[i].Error == "" {
			t.Errorf("errs[%d] = %+v, want Go in %s with its message", i, errs[i], repo)
		}
	}
}
