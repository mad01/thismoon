package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// NPM discovers npm dependencies from package-lock.json files (lockfile v2/v3),
// which carry the fully-resolved version of every package. It does not run npm
// or touch the network — it reads the committed lockfile.
type NPM struct{}

func (NPM) Name() string { return "npm" }

func (n NPM) Discover(repoRoot string, opts Options) ([]Package, error) {
	locks, err := findManifests(repoRoot, "package-lock.json", opts)
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, lock := range locks {
		parsed, err := parseNPMLock(lock)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", lock, err)
		}
		pkgs = append(pkgs, parsed...)
	}
	return pkgs, nil
}

// npmLock mirrors the package-lock.json v2/v3 shape we use: the flat "packages"
// map keyed by install path ("" is the root project, "node_modules/foo" is a
// dependency), plus the root's declared dependency sets to mark direct deps.
type npmLock struct {
	Packages map[string]struct {
		Version         string            `json:"version"`
		Dev             bool              `json:"dev"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	} `json:"packages"`
}

func parseNPMLock(path string) ([]Package, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock npmLock
	if err := json.Unmarshal(raw, &lock); err != nil {
		return nil, err
	}

	// The root entry ("") declares which packages are direct.
	direct := map[string]bool{}
	if root, ok := lock.Packages[""]; ok {
		for name := range root.Dependencies {
			direct[name] = true
		}
		for name := range root.DevDependencies {
			direct[name] = true
		}
	}

	var pkgs []Package
	for installPath, entry := range lock.Packages {
		if installPath == "" || entry.Version == "" {
			continue // root project / workspace links have no checkable version
		}
		name := npmPackageName(installPath)
		pkgs = append(pkgs, Package{
			Ecosystem:    "npm",
			Name:         name,
			Version:      entry.Version,
			ManifestPath: path,
			Direct:       direct[name],
			Imported:     true, // npm reachability isn't computed; never hide.
		})
	}
	return pkgs, nil
}

// npmPackageName turns a lockfile install path into a package name: the segment
// after the final "node_modules/" ("node_modules/a/node_modules/@scope/b" ->
// "@scope/b").
func npmPackageName(installPath string) string {
	const sep = "node_modules/"
	if i := strings.LastIndex(installPath, sep); i >= 0 {
		return installPath[i+len(sep):]
	}
	return installPath
}
