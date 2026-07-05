package discover

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

// Go discovers Go module dependencies. It walks the repo for every go.mod, runs
// `go list -m -json all` in each module dir to get the *resolved* version graph
// (more accurate than parsing go.mod, which omits transitively-selected
// versions), and maps each module to a Package. It also runs `go list -deps` to
// learn which of those modules actually contribute compiled code, so a graph-
// only transitive (in the module graph but never imported) is marked
// Imported=false instead of surfacing as a real, unfixable finding.
type Go struct{}

func (Go) Name() string { return "Go" }

func (g Go) Discover(repoRoot string, opts Options) ([]Package, error) {
	mods, err := findManifests(repoRoot, "go.mod", opts)
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, manifest := range mods {
		dir := filepath.Dir(manifest)
		out, err := runGoList(dir)
		if err != nil {
			return nil, fmt.Errorf("go list in %s: %w", dir, err)
		}
		imported, importedOK := goListDeps(dir)
		parsed, err := parseGoList(strings.NewReader(out), manifest, imported, importedOK)
		if err != nil {
			return nil, fmt.Errorf("parse go list in %s: %w", dir, err)
		}
		pkgs = append(pkgs, parsed...)
	}
	return pkgs, nil
}

// runGoList executes `go list -m -json all` in dir. It streams one JSON object
// per module to stdout (not an array).
func runGoList(dir string) (string, error) {
	cmd := exec.Command("go", "list", "-m", "-json", "all")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// goListDeps runs `go list -deps -json ./...` in dir and returns the set of
// module paths that contribute at least one compiled package to the module's
// build — the import graph, as opposed to the full module graph from `go list -m
// all`. Both a module's own path and any replacement path are recorded so a
// replaced-and-imported module still matches. ok is false when the analysis
// can't run (a build error, no Go toolchain, etc.); callers then fall back to
// treating every dependency as imported rather than hiding a real finding. Note
// this resolves for the current GOOS/GOARCH only, so a dep imported solely on
// another platform reads as not-imported here.
func goListDeps(dir string) (set map[string]struct{}, ok bool) {
	cmd := exec.Command("go", "list", "-deps", "-json", "./...")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	return parseGoListDeps(strings.NewReader(string(out)))
}

// parseGoListDeps decodes a `go list -deps -json ./...` package stream into the
// set of module paths it touches. Stdlib packages have no Module and are
// skipped. ok is false on a malformed stream.
func parseGoListDeps(r io.Reader) (map[string]struct{}, bool) {
	set := map[string]struct{}{}
	dec := json.NewDecoder(r)
	for {
		var p struct {
			Module *struct {
				Path    string `json:"Path"`
				Replace *struct {
					Path string `json:"Path"`
				} `json:"Replace"`
			} `json:"Module"`
		}
		if err := dec.Decode(&p); err != nil {
			if err == io.EOF {
				break
			}
			return nil, false
		}
		if p.Module == nil {
			continue // stdlib package — no module
		}
		set[p.Module.Path] = struct{}{}
		if p.Module.Replace != nil && p.Module.Replace.Path != "" {
			set[p.Module.Replace.Path] = struct{}{}
		}
	}
	return set, true
}

// isImported reports whether module m is in the import graph. It fails open: if
// reachability couldn't be determined (importedOK false) or the analysis found
// no modules at all (an empty set, e.g. `./...` matched no buildable package),
// everything is treated as imported so a real advisory is never hidden. A
// replaced module matches on either its original or its replacement path.
func isImported(m goModule, imported map[string]struct{}, importedOK bool) bool {
	if !importedOK || len(imported) == 0 {
		return true
	}
	if _, ok := imported[m.Path]; ok {
		return true
	}
	if m.Replace != nil {
		if _, ok := imported[m.Replace.Path]; ok {
			return true
		}
	}
	return false
}

// goModule mirrors the fields of `go list -m -json` we use.
type goModule struct {
	Path     string `json:"Path"`
	Version  string `json:"Version"`
	Main     bool   `json:"Main"`
	Indirect bool   `json:"Indirect"`
	Replace  *struct {
		Path    string `json:"Path"`
		Version string `json:"Version"`
	} `json:"Replace"`
}

// parseGoList decodes the `go list -m -json all` stream into Packages. The main
// module and any replace-to-local-path (no version) entries are skipped — only
// resolved external versions are real dependencies to check. imported is the
// import-graph module set from goListDeps (importedOK false → unavailable); each
// module is tagged Imported accordingly, failing open to true.
func parseGoList(r io.Reader, manifest string, imported map[string]struct{}, importedOK bool) ([]Package, error) {
	dec := json.NewDecoder(r)
	var pkgs []Package
	for {
		var m goModule
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if m.Main {
			continue
		}
		// A replace directive points the module at a different path/version;
		// the effective dependency is the replacement.
		path, version := m.Path, m.Version
		if m.Replace != nil {
			path, version = m.Replace.Path, m.Replace.Version
		}
		if version == "" {
			// Replaced with a local filesystem path — nothing to check.
			continue
		}
		pkgs = append(pkgs, Package{
			Ecosystem:    "Go",
			Name:         path,
			Version:      goOSVVersion(version),
			ManifestPath: manifest,
			Direct:       !m.Indirect,
			Imported:     isImported(m, imported, importedOK),
		})
	}
	return pkgs, nil
}

// goOSVVersion normalizes a Go module version for OSV: the Go ecosystem in OSV
// stores versions without the leading "v" (v1.2.3 -> 1.2.3).
func goOSVVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}
