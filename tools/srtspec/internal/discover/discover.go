// Package discover finds the sandbox artifacts in a repo tree: the access
// manifests and the srt settings file each one renders to, the settings
// files nothing renders, the expansion layers kept beside a recipe, and the
// components whose recipe launches a process without declaring a manifest.
// The mapping is the fleet's layout convention, so generate and verify need
// no per-manifest configuration:
//
//   - a component's service-info.yaml (in services/<name>, tools/<name>, or a
//     top-level <name> directory) renders to recipes/<name>/<label>.srt.json;
//   - a recipe's own access.yaml renders beside it, as
//     recipes/<name>/<label>.srt.json; a recipe one level down
//     (recipes/<group>/<name>/access.yaml) renders beside it the same way.
//
// A service-info.yaml without spec.access is not a manifest and is skipped,
// and so is anything under a testdata directory.
package discover

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mad01/thismoon/kit/access"
)

// componentGlobs are the service-info.yaml locations, relative to the root.
var componentGlobs = []string{
	"*/" + access.ServiceInfoFile,
	"services/*/" + access.ServiceInfoFile,
	"tools/*/" + access.ServiceInfoFile,
}

// The recipe-side locations, relative to the root: standalone manifests,
// rendered settings files and expansion layers, each at both recipe depths.
var (
	recipeGlobs = []string{
		"recipes/*/" + access.StandaloneFile,
		"recipes/*/*/" + access.StandaloneFile,
	}
	settingsGlobs = []string{"recipes/*/*.srt.json", "recipes/*/*/*.srt.json"}
	layerGlobs    = []string{"recipes/*/*.local.json", "recipes/*/*/*.local.json"}
)

// Target is one manifest and where it renders. Paths are relative to the
// root Find was given.
type Target struct {
	Path     string // the manifest file
	Output   string // the settings file; "" when the sandbox is not srt
	Manifest access.Manifest
}

// Find returns every manifest under root, sorted by path. It fails on an
// invalid manifest, on a component manifest with no recipe directory to
// render into, and on two manifests that render to the same file.
func Find(root string) ([]Target, error) {
	var targets []Target
	paths, err := glob(root, componentGlobs)
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		t, ok, err := component(root, p)
		if err != nil {
			return nil, err
		}
		if ok {
			targets = append(targets, t)
		}
	}
	paths, err = glob(root, recipeGlobs)
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		t, err := standalone(root, p)
		if err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	slices.SortFunc(targets, func(a, b Target) int { return strings.Compare(a.Path, b.Path) })
	return targets, checkUnique(targets)
}

func component(root, path string) (Target, bool, error) {
	m, err := access.Load(path)
	if errors.Is(err, access.ErrNoManifest) {
		return Target{}, false, nil
	}
	if err != nil {
		return Target{}, false, err
	}
	rel := relTo(root, path)
	t := Target{Path: rel, Manifest: m}
	if m.Sandbox != access.SandboxSRT {
		return t, true, nil
	}
	recipe := recipeDir(path)
	info, err := os.Stat(filepath.Join(root, recipe))
	if err != nil || !info.IsDir() {
		return Target{}, false, fmt.Errorf(
			"%s: no recipe directory %s to render %s.srt.json into",
			rel,
			recipe,
			m.Label,
		)
	}
	t.Output = filepath.Join(recipe, m.Label+".srt.json")
	return t, true, nil
}

// recipeDir maps the service-info.yaml at path to its component's recipe
// directory, relative to the root: recipes/<name of the directory holding
// the file>.
func recipeDir(path string) string {
	return filepath.Join("recipes", filepath.Base(filepath.Dir(path)))
}

func standalone(root, path string) (Target, error) {
	m, err := access.Load(path)
	if err != nil {
		return Target{}, err
	}
	t := Target{Path: relTo(root, path), Manifest: m}
	if m.Sandbox == access.SandboxSRT {
		t.Output = filepath.Join(filepath.Dir(t.Path), m.Label+".srt.json")
	}
	return t, nil
}

func checkUnique(targets []Target) error {
	from := map[string]string{}
	for _, t := range targets {
		if t.Output == "" {
			continue
		}
		if prev, ok := from[t.Output]; ok {
			return fmt.Errorf(
				"%s and %s both render %s; give one a distinct label",
				prev,
				t.Path,
				t.Output,
			)
		}
		from[t.Output] = t.Path
	}
	return nil
}

// Orphans returns the settings files under recipes/ that no target renders,
// relative to root. With the manifest as the source of truth, such a file is
// either hand-written or left behind by a removed manifest.
func Orphans(root string, targets []Target) ([]string, error) {
	files, err := glob(root, settingsGlobs)
	if err != nil {
		return nil, err
	}
	rendered := map[string]bool{}
	for _, t := range targets {
		rendered[t.Output] = true
	}
	var out []string
	for _, f := range files {
		if rel := relTo(root, f); !rendered[rel] {
			out = append(out, rel)
		}
	}
	return out, nil
}

// Layers returns the expansion layers (<label>.local.json) kept beside a
// recipe, relative to root and sorted. A repo ships one when a machine needs
// a widening the public manifest cannot name.
func Layers(root string) ([]string, error) {
	files, err := glob(root, layerGlobs)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, relTo(root, f))
	}
	return out, nil
}

// glob matches every pattern under root, drops anything under a testdata
// directory, and returns the matches sorted.
func glob(root string, patterns []string) ([]string, error) {
	var out []string
	for _, pattern := range patterns {
		paths, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return nil, fmt.Errorf("discover: %w", err)
		}
		for _, p := range paths {
			if !underTestdata(relTo(root, p)) {
				out = append(out, p)
			}
		}
	}
	slices.Sort(out)
	return out, nil
}

func underTestdata(rel string) bool {
	return slices.Contains(strings.Split(filepath.ToSlash(rel), "/"), "testdata")
}

func relTo(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
