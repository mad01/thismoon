// Package discover finds the access manifests in a repo tree and the srt
// settings file each one renders to. The mapping is the fleet's layout
// convention, so generate needs no per-manifest configuration:
//
//   - a component's service-info.yaml (in services/<name>, tools/<name>, or a
//     top-level <name> directory) renders to recipes/<name>/<label>.srt.json;
//   - a recipe's own access.yaml renders beside it, as
//     recipes/<name>/<label>.srt.json.
//
// A service-info.yaml without spec.access is not a manifest and is skipped.
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

// recipeGlob is where standalone manifests live, relative to the root.
var recipeGlob = "recipes/*/" + access.StandaloneFile

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
	for _, glob := range componentGlobs {
		paths, err := filepath.Glob(filepath.Join(root, glob))
		if err != nil {
			return nil, fmt.Errorf("discover: %w", err)
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
	}
	paths, err := filepath.Glob(filepath.Join(root, recipeGlob))
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
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
	recipe := filepath.Join("recipes", filepath.Base(filepath.Dir(path)))
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
	files, err := filepath.Glob(filepath.Join(root, "recipes", "*", "*.srt.json"))
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
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

func relTo(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
