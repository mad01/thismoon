// Package discover enumerates external dependencies across repos, one Ecosystem
// implementation per package manager. The set is pluggable: adding a language
// means adding an Ecosystem, not touching the walker. Discovery is read-only and
// produces resolved name@version tuples — the advisory check happens elsewhere.
package discover

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/mad01/thismoon/services/deps/internal/store"
)

// Package is one resolved dependency a discoverer found. It maps directly onto
// store.Dependency once advisories are attached.
type Package struct {
	Ecosystem    string // OSV ecosystem string: Go | npm | PyPI
	Name         string
	Version      string
	ManifestPath string // the go.mod / package-lock.json that declared it
	Direct       bool
	// Imported is true when the package contributes compiled code to the main
	// module's build (Go reachability via `go list -deps`). Ecosystems that
	// don't compute reachability set it true so nothing is hidden.
	Imported bool
}

// Options tunes a discovery walk. ExcludePath, if set, skips a directory by its
// repo-relative path (the config's exclude_paths). It is nil when no config
// applies. Git worktrees and nested checkouts are skipped regardless of Options.
type Options struct {
	ExcludePath func(relPath string) bool
}

// excluded reports whether the directory at path (under repoRoot) is excluded.
func (o Options) excluded(repoRoot, path string) bool {
	if o.ExcludePath == nil {
		return false
	}
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return false
	}
	return o.ExcludePath(rel)
}

// Ecosystem discovers the dependencies of one package manager within a repo.
type Ecosystem interface {
	// Name is the OSV ecosystem string (Go, npm, PyPI) for reporting.
	Name() string
	// Discover walks repoRoot for this ecosystem's manifests and returns every
	// resolved dependency. A repo with no matching manifest yields nil, nil.
	Discover(repoRoot string, opts Options) ([]Package, error)
}

// Default is the ecosystem set built today: Go (resolved module graph +
// reachability), npm (package-lock), and PyPI (requirements.txt exact pins).
func Default() []Ecosystem {
	return []Ecosystem{Go{}, NPM{}, Python{}}
}

// Repos discovers dependencies across every repo using every ecosystem, tagging
// each with the repo it came from. A discoverer error on one repo/ecosystem is
// collected and reported but does not abort the whole scan.
func Repos(repos []string, ecosystems []Ecosystem, opts Options) ([]store.Dependency, error) {
	var (
		deps []store.Dependency
		errs []error
	)
	for _, repo := range repos {
		for _, eco := range ecosystems {
			pkgs, err := eco.Discover(repo, opts)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s in %s: %w", eco.Name(), repo, err))
				continue
			}
			for _, p := range pkgs {
				deps = append(deps, store.Dependency{
					Ecosystem:    p.Ecosystem,
					Name:         p.Name,
					Version:      p.Version,
					Repo:         repo,
					ManifestPath: p.ManifestPath,
					Direct:       p.Direct,
					Imported:     p.Imported,
				})
			}
		}
	}
	if len(errs) > 0 {
		// Surface discovery problems without losing the deps we did find.
		return deps, fmt.Errorf("discovery: %w", errors.Join(errs...))
	}
	return deps, nil
}
