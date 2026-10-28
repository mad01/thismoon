package discover

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// JSDelivrScripts discovers the npm packages a shell script downloads from
// jsDelivr (cdn.jsdelivr.net/npm/<name>@<version>/...), such as a service
// that fetches its browser libraries at install time. The URL carries the npm
// name, and the version is either written in the URL or a $VAR the script sets
// to a literal, so no script's variable names are known in advance. It reads
// every *.sh file the shared walker reaches, and keeps only exact versions: a
// range, a tag like latest, or a variable it cannot resolve pins nothing OSV
// can check. It touches no network.
type JSDelivrScripts struct{}

func (JSDelivrScripts) Name() string { return "npm" }

func (JSDelivrScripts) Discover(repoRoot string, opts Options) ([]Package, error) {
	scripts, err := findFiles(repoRoot, opts, func(rel string) bool {
		return strings.HasSuffix(rel, ".sh")
	})
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, script := range scripts {
		raw, err := os.ReadFile(script)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", script, err)
		}
		pkgs = append(pkgs, parseJSDelivrScript(string(raw), script)...)
	}
	return pkgs, nil
}

var (
	// jsdelivrNPMURL matches a jsDelivr npm URL through its version token: the
	// package name, scoped or not, then whatever follows the @ up to the next
	// path, query or quoting boundary.
	jsdelivrNPMURL = regexp.MustCompile(
		`cdn\.jsdelivr\.net/npm/((?:@[a-z0-9][a-z0-9._~-]*/)?[a-z0-9][a-z0-9._~-]*)@([^/\s"'?#]+)`,
	)
	// shellAssign matches a plain assignment of a literal: NAME=value,
	// NAME="value" or NAME='value', optionally after export or readonly. A value
	// that expands anything is left out.
	shellAssign = regexp.MustCompile(
		`(?m)^[ \t]*(?:export[ \t]+|readonly[ \t]+)?([A-Za-z_][A-Za-z0-9_]*)=` +
			`(?:"([^"$]*)"|'([^']*)'|([^\s"'$;]+))`,
	)
	// shellVarRef matches a version token that is a variable reference, $NAME
	// or ${NAME}.
	shellVarRef = regexp.MustCompile(`^\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?$`)
	// exactVersion matches a full semver version, with an optional prerelease.
	exactVersion = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)
)

// parseJSDelivrScript returns one direct npm package per distinct name@version
// the script downloads from jsDelivr, in the order the script first names
// them. Two URLs of the same package version (its JS and its CSS) are one
// package.
func parseJSDelivrScript(script, path string) []Package {
	vars := map[string]string{}
	for _, m := range shellAssign.FindAllStringSubmatch(script, -1) {
		vars[m[1]] = m[2] + m[3] + m[4] // only the matched alternative is non-empty
	}
	seen := map[string]bool{}
	var pkgs []Package
	for _, m := range jsdelivrNPMURL.FindAllStringSubmatch(script, -1) {
		name, version := m[1], m[2]
		if ref := shellVarRef.FindStringSubmatch(version); ref != nil {
			version = vars[ref[1]]
		}
		if !exactVersion.MatchString(version) || seen[name+"@"+version] {
			continue
		}
		seen[name+"@"+version] = true
		pkgs = append(pkgs, Package{
			Ecosystem:    "npm",
			Name:         name,
			Version:      version,
			ManifestPath: path,
			Direct:       true,
			Imported:     true, // npm reachability isn't computed; never hide.
		})
	}
	return pkgs
}
