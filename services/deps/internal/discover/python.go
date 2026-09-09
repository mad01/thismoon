package discover

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Python discovers PyPI dependencies from requirements.txt files. Only exact
// `name==version` pins are emitted — OSV needs a concrete version, so bare
// names, ranges (>=, ~=), wildcards, option lines (-r, --index-url), and
// direct URL references are skipped. requirements.txt declares direct deps
// only (no lockfile format is in use across the scanned repos), and Python
// reachability isn't computed, so every package is Direct and Imported.
type Python struct{}

func (Python) Name() string { return "PyPI" }

func (Python) Discover(repoRoot string, opts Options) ([]Package, error) {
	manifests, err := findManifests(repoRoot, "requirements.txt", opts)
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, m := range manifests {
		parsed, err := parseRequirements(m)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", m, err)
		}
		pkgs = append(pkgs, parsed...)
	}
	return pkgs, nil
}

func parseRequirements(path string) ([]Package, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	for line := range strings.SplitSeq(string(raw), "\n") {
		name, version, ok := parseRequirementLine(line)
		if !ok {
			continue
		}
		pkgs = append(pkgs, Package{
			Ecosystem:    "PyPI",
			Name:         name,
			Version:      version,
			ManifestPath: path,
			Direct:       true,
			Imported:     true, // PyPI reachability isn't computed; never hide.
		})
	}
	return pkgs, nil
}

// parseRequirementLine extracts an exact `name==version` pin from one
// requirements.txt line, with comments, extras ([server]), and environment
// markers (; python_version < "3.12") stripped. Anything that doesn't resolve
// to a single concrete version returns ok=false.
func parseRequirementLine(line string) (name, version string, ok bool) {
	if i := strings.Index(line, "#"); i >= 0 {
		line = line[:i]
	}
	if i := strings.Index(line, ";"); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	// Skip blanks, pip options (-r/-e/--index-url), and direct references
	// (name @ https://...) — none carry an OSV-checkable pin.
	if line == "" || strings.HasPrefix(line, "-") || strings.Contains(line, "@") {
		return "", "", false
	}
	name, spec, found := strings.Cut(line, "==")
	if !found {
		return "", "", false
	}
	// Any operator char left of the "==" means a range or ordering constraint
	// (e.g. "foo>=1,==1.*" or "foo~=1.2"), not an exact pin.
	if strings.ContainsAny(name, "<>!~=") {
		return "", "", false
	}
	name = strings.TrimSpace(name)
	if i := strings.Index(name, "["); i >= 0 {
		name = name[:i]
	}
	version = strings.TrimSpace(spec)
	// A multi-specifier keeps only the pinned part: "1.2.3,<2" -> "1.2.3".
	if i := strings.Index(version, ","); i >= 0 {
		version = strings.TrimSpace(version[:i])
	}
	// "===" (arbitrary equality) leaves a leading "="; "==1.*" is not exact.
	if name == "" || version == "" || strings.HasPrefix(version, "=") ||
		strings.Contains(version, "*") {
		return "", "", false
	}
	return normalizePyPI(name), version, true
}

var pyPINameSep = regexp.MustCompile(`[-_.]+`)

// normalizePyPI applies PEP 503 name normalization (lowercase, runs of -_.
// collapse to a single hyphen) — the form OSV's PyPI ecosystem is keyed on.
func normalizePyPI(name string) string {
	return pyPINameSep.ReplaceAllString(strings.ToLower(name), "-")
}
