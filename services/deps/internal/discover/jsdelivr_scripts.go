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
// to one literal value, so no script's variable names are known in advance. It
// reads every *.sh file the shared walker reaches, ignores comment lines, and
// keeps only exact versions: a range, a tag like latest, or a variable it
// cannot resolve pins nothing OSV can check. It touches no network.
type JSDelivrScripts struct{}

func (JSDelivrScripts) Name() string { return "npm" }

func (JSDelivrScripts) Discover(repoRoot string, opts Options) ([]Package, error) {
	scripts, err := findFiles(repoRoot, opts, skipDir, func(rel string) bool {
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
	// shellComment matches a comment line: its first non-blank character is #.
	shellComment = regexp.MustCompile(`(?m)^[ \t]*#.*$`)
	// shellAssignment matches the NAME= that starts an assignment wherever a
	// command or an assignment word can begin: after a line start, a blank, ;,
	// &, | or (. The export, readonly, local and declare forms match too, since
	// a blank precedes the name.
	shellAssignment = regexp.MustCompile(`(?m)(?:^|[\s;&|(])([A-Za-z_][A-Za-z0-9_]*)=`)
	// shellLiteral matches a literal value at the start of its input, value,
	// "value" or 'value', and the blank, ; or end of script that ends it. A
	// value that expands anything or runs on into more of the word
	// (1.2.3$SUFFIX, "4.5.6"-rc.1) doesn't match; \x60 is the backtick.
	shellLiteral = regexp.MustCompile(
		`^(?:"([^"$\x60\\]*)"|'([^']*)'|([^\s"'$;\x60\\]+))(?:[\s;]|$)`,
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
// package. A URL on a comment line is skipped: it is history or a note, not a
// download.
func parseJSDelivrScript(script, path string) []Package {
	script = shellComment.ReplaceAllString(script, "")
	vars := shellLiterals(script)
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

// shellLiterals maps each variable the script always assigns the same literal
// to that literal. A variable it also assigns a different literal, or anything
// that is not a literal, is left out: the value a URL sees then depends on the
// branches the script takes, so picking one could name the wrong version.
func shellLiterals(script string) map[string]string {
	vars := map[string]string{}
	ambiguous := map[string]bool{}
	for _, m := range shellAssignment.FindAllStringSubmatchIndex(script, -1) {
		name := script[m[2]:m[3]]
		lit := shellLiteral.FindStringSubmatch(script[m[1]:])
		if lit == nil {
			ambiguous[name] = true
			continue
		}
		value := lit[1] + lit[2] + lit[3] // only the matched alternative is non-empty
		if prev, ok := vars[name]; ok && prev != value {
			ambiguous[name] = true
		}
		vars[name] = value
	}
	for name := range ambiguous {
		delete(vars, name)
	}
	return vars
}
