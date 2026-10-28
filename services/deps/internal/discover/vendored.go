package discover

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/mad01/thismoon/services/deps/internal/jsdelivr"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// HashLookup maps a file's sha256 to the published file it is a copy of.
// *jsdelivr.Client implements it; found is false when no published file
// matches.
type HashLookup interface {
	Lookup(ctx context.Context, sum string) (file jsdelivr.File, found bool, err error)
}

// VendoredJS discovers browser libraries committed into a repo as plain
// files, outside any lockfile. A candidate is a .js file inside a directory
// named static whose name ends in .min.js or carries a version
// (cytoscape-3.31.0.min.js). First-party scripts like app.js are never hashed,
// so the network calls stay at one per vendored file. Unlike the other
// discoverers it walks into vendor directories, since static/vendor/ is where
// such files usually live. Each candidate is hashed and looked up: a match
// names the npm package and version for certain. Without one, the file name
// supplies them and the row carries the deps-vendored-modified signal. Apart
// from the module downloads go list may make, this is the one discoverer that
// reaches the network.
type VendoredJS struct {
	Lookup HashLookup
}

func (VendoredJS) Name() string { return "npm" }

// Discover fails the whole repo when any lookup fails: a file it could not look
// up is neither known clean nor known modified, so it is reported as a
// discovery error rather than a guess.
func (v VendoredJS) Discover(repoRoot string, opts Options) ([]Package, error) {
	files, err := findFiles(repoRoot, opts, skipDirKeepingVendor, vendoredCandidate)
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, path := range files {
		sum, err := fileSHA256(path)
		if err != nil {
			return nil, err
		}
		// Discovery runs without a context, as go list does; the client's
		// timeout bounds each call.
		file, found, err := v.Lookup.Lookup(context.Background(), sum)
		if err != nil {
			return nil, fmt.Errorf("look up %s: %w", path, err)
		}
		if p, ok := vendoredPackage(path, sum, file, found); ok {
			pkgs = append(pkgs, p)
		}
	}
	return pkgs, nil
}

// vendoredFileName splits a versioned library file name into the library name
// and its version: cytoscape-dagre-2.5.0.js, d3-7.9.0.min.js,
// foo-v1.0.0-rc.1.min.js.
var vendoredFileName = regexp.MustCompile(
	`^(.+?)-v?(\d+\.\d+\.\d+(?:-[0-9A-Za-z]+(?:\.\d+)*)?)(?:\.[0-9A-Za-z]+)*\.js$`,
)

// vendoredCandidate reports whether the file at the repo-relative path rel is
// worth hashing as a vendored library.
func vendoredCandidate(rel string) bool {
	name := filepath.Base(rel)
	if !strings.HasSuffix(name, ".js") {
		return false
	}
	if !slices.Contains(strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/"), "static") {
		return false
	}
	return strings.HasSuffix(name, ".min.js") || vendoredFileName.MatchString(name)
}

// vendoredPackage turns one lookup result into the package row, or reports
// false when there is nothing to name the file by. An npm match names it. A
// file with no match is named from its file name and carries the modified
// signal, and its version gains +sha256.<prefix> so the finding keys change
// whenever the file does (OSV ignores semver build metadata when matching). A
// GitHub match is published and unchanged but has no npm name, so the file
// name supplies one without the signal; the version stays jsDelivr's, so a
// different release swapped in under the same file name still changes it.
// Both rows named from the file name are marked NameFromFile: the npm package
// of that name may be unrelated (a first-party utils-0.1.0.js against npm's
// utils, unpublished in 2012), so its registry state says nothing about them.
func vendoredPackage(path, sum string, file jsdelivr.File, found bool) (Package, bool) {
	pkg := Package{
		Ecosystem:    "npm",
		ManifestPath: path,
		Direct:       true,
		Imported:     true, // npm reachability isn't computed; never hide.
	}
	if found && file.Type == "npm" {
		pkg.Name, pkg.Version = file.Name, file.Version
		return pkg, true
	}
	m := vendoredFileName.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return Package{}, false // an unversioned .min.js nobody published
	}
	pkg.Name, pkg.Version, pkg.NameFromFile = m[1], m[2], true
	if found {
		pkg.Version = strings.TrimPrefix(file.Version, "v") // GitHub tags are often v1.2.3
		return pkg, true
	}
	pkg.Version += "+sha256." + sum[:12]
	pkg.Advisories = []store.Advisory{{
		ID:   "deps-vendored-modified",
		Kind: store.KindSignal,
		Summary: "This file's sha256 matches no file jsDelivr has served: it was changed " +
			"after download, or never came from npm. Its name and version come from the file name.",
	}}
	return pkg, true
}

// fileSHA256 returns the hex-encoded sha256 of the file at path.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
