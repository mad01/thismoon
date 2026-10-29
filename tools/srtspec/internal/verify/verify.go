// Package verify checks a repo's sandbox artifacts against their sources
// without writing anything: every srt manifest renders to a settings file
// that exists and holds the same bytes as the render, no settings file is
// left without a manifest, every component whose recipe launches a process
// declares spec.access, and every expansion layer kept beside a recipe only
// widens. It is the gate CI runs; generate fixes the renders.
package verify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/kit/access"
	"github.com/mad01/thismoon/tools/srtspec/internal/discover"
	"github.com/mad01/thismoon/tools/srtspec/internal/drift"
	"github.com/mad01/thismoon/tools/srtspec/internal/layers"
)

// Result is one check: the file it looked at, what held when it passed, and
// why it failed otherwise.
type Result struct {
	Path   string // the checked file, relative to the root
	Note   string // what held, e.g. "renders from services/events/service-info.yaml"
	Reason string // why the check failed; "" when it passed
}

// OK reports whether the check passed.
func (r Result) OK() bool { return r.Reason == "" }

// Run checks every artifact under root and returns one Result per check, in
// path order within each kind: settings files, then orphans, then components
// that launch a process without spec.access, then layers.
// home turns absolute paths in a layer back into ~ form, the way srt-run
// compares them. The error covers what stops the checks from running at
// all: an invalid manifest, an unreadable tree, or a root with nothing to
// verify, which would otherwise pass for the wrong reason.
func Run(root, home string) ([]Result, error) {
	targets, err := discover.Find(root)
	if err != nil {
		return nil, err
	}
	var results []Result
	for _, t := range targets {
		if t.Output == "" {
			continue
		}
		results = append(results, settings(root, t))
	}
	orphans, err := discover.Orphans(root, targets)
	if err != nil {
		return nil, err
	}
	for _, o := range orphans {
		results = append(results, Result{
			Path:   o,
			Reason: "no manifest renders it; declare spec.access or an access.yaml, or delete the file",
		})
	}
	undeclared, err := launches(root, targets)
	if err != nil {
		return nil, err
	}
	results = append(results, undeclared...)
	paths, err := discover.Layers(root)
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		results = append(results, layer(root, p, home))
	}
	if len(results) == 0 {
		return nil, fmt.Errorf(
			"verify: nothing under %s to check: no srt manifest, settings file or expansion layer",
			root,
		)
	}
	return results, nil
}

// launches fails every component whose recipe registers a t-man service
// while its service-info.yaml declares no spec.access. An unsandboxed
// process still declares one, as sandbox none with a reason.
func launches(root string, targets []discover.Target) ([]Result, error) {
	found, err := discover.Undeclared(root, targets)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(found))
	for _, l := range found {
		results = append(results, Result{
			Path: l.Path,
			Reason: fmt.Sprintf(
				"declares no spec.access, but %s runs %q; add one (sandbox none needs a reason)",
				l.File,
				l.Marker,
			),
		})
	}
	return results, nil
}

// settings checks that the file a manifest renders to exists and holds the
// render's bytes.
func settings(root string, t discover.Target) Result {
	r := Result{Path: t.Output, Note: "renders from " + t.Path}
	want, err := access.Render(t.Manifest)
	if err != nil {
		r.Reason = fmt.Sprintf("%s: %v", t.Path, err)
		return r
	}
	got, err := os.ReadFile(filepath.Join(root, t.Output))
	if errors.Is(err, os.ErrNotExist) {
		r.Reason = fmt.Sprintf("does not exist; %s renders it: run srtspec generate", t.Path)
		return r
	}
	if err != nil {
		r.Reason = err.Error()
		return r
	}
	if bytes.Equal(got, want) {
		return r
	}
	diff, err := drift.Diff(t.Output, got, t.Path, want)
	switch {
	case err != nil:
		r.Reason = fmt.Sprintf("is not a render: %v", err)
	case diff == "":
		r.Reason = fmt.Sprintf(
			"matches %s in meaning but not in layout (a hand edit); run srtspec generate",
			t.Path,
		)
	default:
		r.Reason = fmt.Sprintf(
			"drifted from %s; run srtspec generate, or srtspec check for the diff",
			t.Path,
		)
	}
	return r
}

// layer checks that an expansion layer only widens the settings file beside
// it. When the repo holds the layer but not that file (the base ships from
// another repo), the layer is checked against an empty base: every rule
// still applies except the one that catches an allow carving back a deny.
func layer(root, path, home string) Result {
	r := Result{Path: path}
	local, err := readObject(filepath.Join(root, path))
	if err != nil {
		r.Reason = err.Error()
		return r
	}
	label := strings.TrimSuffix(filepath.Base(path), ".local.json")
	basePath := filepath.Join(filepath.Dir(path), label+".srt.json")
	base, err := readObject(filepath.Join(root, basePath))
	switch {
	case errors.Is(err, os.ErrNotExist):
		base = map[string]any{}
		r.Note = fmt.Sprintf(
			"only widens; no %s beside it, so carve-backs of its denies are unchecked",
			filepath.Base(basePath),
		)
	case err != nil:
		r.Reason = fmt.Sprintf("%s %v", basePath, err)
		return r
	default:
		r.Note = "only widens " + basePath
	}
	if err := layers.CheckLocal(base, local, home); err != nil {
		r.Reason = "srt-run would refuse it: " + err.Error()
	}
	return r
}

// readObject reads a settings or layer file as a JSON object. A missing
// file comes back as os.ErrNotExist so the caller can tell it apart.
func readObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("is not a JSON object: %w", err)
	}
	return obj, nil
}
