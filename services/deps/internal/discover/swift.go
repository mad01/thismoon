package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Swift discovers Swift package dependencies from Package.resolved files
// (format v2/v3), which pin every remote package to an exact version. OSV's
// SwiftURL ecosystem keys packages on the repository URL without scheme or
// .git suffix (github.com/vapor/vapor), so names derive from each pin's
// location. Branch- and commit-only pins carry no version OSV can check and
// are skipped. Package.resolved is flat — it doesn't say which pins the
// project declares directly — so every package is marked Direct rather than
// understating a real dependency as transitive; Swift reachability isn't
// computed, so everything is Imported.
type Swift struct{}

func (Swift) Name() string { return "SwiftURL" }

func (s Swift) Discover(repoRoot string, opts Options) ([]Package, error) {
	manifests, err := findManifests(repoRoot, "Package.resolved", opts)
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, m := range manifests {
		parsed, err := parseSwiftResolved(m)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", m, err)
		}
		pkgs = append(pkgs, parsed...)
	}
	return pkgs, nil
}

// swiftResolved mirrors the Package.resolved v2/v3 shape: a flat pins array
// where each remote pin carries its git URL and resolved state.
type swiftResolved struct {
	Pins []struct {
		Kind     string `json:"kind"`
		Location string `json:"location"`
		State    struct {
			Version string `json:"version"`
		} `json:"state"`
	} `json:"pins"`
}

func parseSwiftResolved(path string) ([]Package, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var res swiftResolved
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, pin := range res.Pins {
		if pin.Kind != "remoteSourceControl" || pin.State.Version == "" {
			continue // local/registry pins and branch pins have no checkable version
		}
		name := swiftPackageName(pin.Location)
		if name == "" {
			continue
		}
		pkgs = append(pkgs, Package{
			Ecosystem:    "SwiftURL",
			Name:         name,
			Version:      pin.State.Version,
			ManifestPath: path,
			Direct:       true,
			Imported:     true, // Swift reachability isn't computed; never hide.
		})
	}
	return pkgs, nil
}

// swiftPackageName converts a pin location to OSV's SwiftURL name: the repo
// URL without scheme, credentials, or .git suffix
// ("https://github.com/vapor/vapor.git" -> "github.com/vapor/vapor",
// "git@github.com:vapor/vapor.git" -> "github.com/vapor/vapor").
func swiftPackageName(location string) string {
	loc := location
	for _, scheme := range []string{"https://", "http://", "git://", "ssh://"} {
		loc = strings.TrimPrefix(loc, scheme)
	}
	if at := strings.Index(loc, "@"); at >= 0 && !strings.Contains(loc[:at], "/") {
		loc = strings.Replace(loc[at+1:], ":", "/", 1)
	}
	loc = strings.TrimSuffix(loc, "/")
	return strings.TrimSuffix(loc, ".git")
}
