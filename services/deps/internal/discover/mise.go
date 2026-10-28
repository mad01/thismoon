package discover

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/services/deps/internal/store"
)

// MiseLabelPrefix starts the repo label of every installed mise tool
// ("mise:npm:yarn"). A label is not a path, so it never collides with a repo
// root, and a per-repo rescan treats each label as one unit. The label carries
// no version, so it stays the same rescan key across tool upgrades.
const MiseLabelPrefix = "mise:"

// MiseEcosystem is the ecosystem of a discovery error about mise.lock itself:
// an unreadable lock hides every tool, npm and PyPI alike.
const MiseEcosystem = "mise"

// IsMiseLabel reports whether id names an installed mise tool, not a repo.
func IsMiseLabel(id string) bool { return strings.HasPrefix(id, MiseLabelPrefix) }

// miseBackends maps the mise backends discovery reads to their OSV ecosystem.
var miseBackends = map[string]string{"npm": "npm", "pipx": "PyPI"}

// Mise discovers the npm and PyPI packages that mise's npm: and pipx: backends
// installed: CLI tools that run on the host, outside any repo. mise.lock pins
// each tool's version, and that version's install directory holds the whole
// tree: an aube-lock.yaml (the pnpm v9 lockfile format) for npm, and a
// virtualenv's *.dist-info folders for pipx. Discovery reads those files and
// never runs mise, whose shims don't work inside the sandbox.
type Mise struct {
	LockPath    string // mise.lock; "" scans no tools
	InstallRoot string // mise's installs directory, one subdirectory per tool
}

// MiseTool is one tool version mise.lock pins and this machine installed.
type MiseTool struct {
	Label     string // MiseLabelPrefix + the tool's name in mise.lock
	Ecosystem string // npm | PyPI
	Package   string // what the backend installs: "@googleworkspace/cli", "httpie"
	Version   string // the version mise.lock pins
	Dir       string // the install directory of that version
}

// miseLock mirrors the part of mise.lock discovery reads: per tool name, one
// entry per pinned version. The per-platform tables are ignored.
type miseLock struct {
	Tools map[string][]struct {
		Version string `toml:"version"`
		Backend string `toml:"backend"`
	} `toml:"tools"`
}

// Discover returns the packages of every installed tool mise.lock pins, each
// tagged with the tool's label as its repo. A lock that can't be read or
// parsed is one DiscoveryError under the lock's path, since it hides every
// tool.
func (m Mise) Discover() ([]store.Dependency, []store.DiscoveryError) {
	tools, err := m.Locked()
	if err != nil {
		return nil, []store.DiscoveryError{{
			Repo:      m.LockPath,
			Ecosystem: MiseEcosystem,
			Error:     err.Error(),
		}}
	}
	return Tools(tools)
}

// Locked returns the npm: and pipx: tools mise.lock pins, sorted by label. One
// lock covers every machine class, so a tool with no install directory here
// is not installed on this machine and is left out. A missing lock means no
// mise, so no tools; a lock that can't be read or parsed is an error.
func (m Mise) Locked() ([]MiseTool, error) {
	if m.LockPath == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(m.LockPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var lock miseLock
	if err := toml.Unmarshal(raw, &lock); err != nil {
		return nil, fmt.Errorf("parse %s: %w", m.LockPath, err)
	}
	var tools []MiseTool
	for name, entries := range lock.Tools {
		for _, e := range entries {
			kind, pkg, _ := strings.Cut(cmp.Or(e.Backend, name), ":")
			eco, ok := miseBackends[kind]
			if !ok {
				continue
			}
			toolDir := filepath.Join(m.InstallRoot, miseDirName(name))
			if _, err := os.Stat(toolDir); errors.Is(err, fs.ErrNotExist) {
				continue
			}
			tools = append(tools, MiseTool{
				Label:     MiseLabelPrefix + name,
				Ecosystem: eco,
				Package:   pkg,
				Version:   e.Version,
				Dir:       filepath.Join(toolDir, e.Version),
			})
		}
	}
	slices.SortFunc(tools, func(a, b MiseTool) int {
		return cmp.Or(cmp.Compare(a.Label, b.Label), cmp.Compare(a.Version, b.Version))
	})
	return tools, nil
}

// Tools reads each tool's installed tree and tags its packages with the tool's
// label as the repo. A tool whose tree can't be read becomes a DiscoveryError
// and the rest go on, the way Repos treats a failing repo.
func Tools(tools []MiseTool) ([]store.Dependency, []store.DiscoveryError) {
	var (
		deps []store.Dependency
		errs []store.DiscoveryError
	)
	for _, t := range tools {
		pkgs, err := t.Packages()
		if err != nil {
			errs = append(errs, store.DiscoveryError{
				Repo:      t.Label,
				Ecosystem: t.Ecosystem,
				Error:     err.Error(),
			})
			continue
		}
		for _, p := range pkgs {
			deps = append(deps, dependency(t.Label, p))
		}
	}
	return deps, errs
}

// Packages reads the tool's installed tree: every package at its installed
// version, with the tool itself marked direct. A pinned version that has no
// install directory means the install lags the lock (mise install fixes it),
// and like an unreadable or empty tree it is an error, not an empty result.
func (t MiseTool) Packages() ([]Package, error) {
	if t.Version == "" {
		return nil, errors.New("mise.lock pins no version")
	}
	if _, err := os.Stat(t.Dir); err != nil {
		return nil, fmt.Errorf("mise.lock pins %s, but it is not installed: %w", t.Version, err)
	}
	var (
		pkgs []Package
		err  error
	)
	if t.Ecosystem == "npm" {
		pkgs, err = parseAubeLock(filepath.Join(t.Dir, "aube-lock.yaml"))
	} else {
		pkgs, err = readSitePackages(t.Dir, t.Package)
	}
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found under %s", t.Dir)
	}
	return pkgs, nil
}

// aubeLock mirrors the parts of an aube-lock.yaml (pnpm lockfile v9) that
// discovery reads: the root importer's declared dependencies, and the packages
// map, which lists every resolved package once under a "name@version" key.
type aubeLock struct {
	LockfileVersion string `yaml:"lockfileVersion"`
	Importers       map[string]struct {
		Dependencies         map[string]struct{} `yaml:"dependencies"`
		OptionalDependencies map[string]struct{} `yaml:"optionalDependencies"`
	} `yaml:"importers"`
	Packages map[string]struct{} `yaml:"packages"`
}

// parseAubeLock lists every package an aube-lock.yaml resolves, the root
// importer's dependencies marked direct. Only lockfile v9 is read: an older
// format keys packages differently and would parse to nothing.
func parseAubeLock(path string) ([]Package, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock aubeLock
	if err := yaml.Unmarshal(raw, &lock); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if major, _, _ := strings.Cut(lock.LockfileVersion, "."); major != "9" {
		return nil, fmt.Errorf(
			"parse %s: lockfileVersion %q, want 9", path, lock.LockfileVersion,
		)
	}
	root := lock.Importers["."]
	var pkgs []Package
	for key := range lock.Packages {
		name, version, ok := splitPackageKey(key)
		if !ok {
			continue
		}
		_, dep := root.Dependencies[name]
		_, optional := root.OptionalDependencies[name]
		pkgs = append(pkgs, Package{
			Ecosystem:    "npm",
			Name:         name,
			Version:      version,
			ManifestPath: path,
			Direct:       dep || optional,
			Imported:     true, // npm reachability isn't computed; never hide.
		})
	}
	return pkgs, nil
}

// splitPackageKey splits a pnpm v9 package key ("left-pad@1.3.0",
// "@scope/name@1.0.0") into name and version. A peer-dependency suffix
// ("(react@18.2.0)") is dropped. A tarball, git or file: dependency has no
// registry version OSV can check, so it returns ok=false.
func splitPackageKey(key string) (name, version string, ok bool) {
	if i := strings.Index(key, "("); i >= 0 {
		key = key[:i]
	}
	i := strings.LastIndex(key, "@")
	if i <= 0 {
		return "", "", false
	}
	name, version = key[:i], key[i+1:]
	if version == "" || strings.ContainsAny(version, ":/") {
		return "", "", false
	}
	return name, version, true
}

// readSitePackages lists the *.dist-info folders of the virtualenv a pipx
// install holds (<dir>/<venv>/lib/python3.*/site-packages). A folder is named
// <name>-<version>.dist-info with any "-" in the name escaped to "_", so the
// first "-" ends the name. Names are PEP 503 normalized; the package the tool
// installs (tool) is the direct one.
func readSitePackages(dir, tool string) ([]Package, error) {
	sites, err := filepath.Glob(filepath.Join(dir, "*", "lib", "python3*", "site-packages"))
	if err != nil {
		return nil, err
	}
	if len(sites) == 0 {
		return nil, fmt.Errorf("no site-packages under %s", dir)
	}
	if i := strings.Index(tool, "["); i >= 0 {
		tool = tool[:i] // pipx:name[extra] installs name
	}
	tool = normalizePyPI(tool)
	seen := map[string]bool{}
	var pkgs []Package
	for _, site := range sites {
		entries, err := os.ReadDir(site)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			base, isInfo := strings.CutSuffix(e.Name(), ".dist-info")
			name, version, found := strings.Cut(base, "-")
			if !isInfo || !found || name == "" || version == "" {
				continue
			}
			name = normalizePyPI(name)
			if seen[name+"@"+version] {
				continue
			}
			seen[name+"@"+version] = true
			pkgs = append(pkgs, Package{
				Ecosystem:    "PyPI",
				Name:         name,
				Version:      version,
				ManifestPath: site,
				Direct:       name == tool,
				Imported:     true, // PyPI reachability isn't computed; never hide.
			})
		}
	}
	return pkgs, nil
}

// miseDirName is the install directory mise gives a tool: the tool's name in
// kebab case, as mise's heck dependency writes it. Each run of
// non-alphanumeric characters is a word break, and so are a lower-to-upper
// step and the start of a capitalized word after an acronym. So
// "npm:@googleworkspace/cli" is "npm-googleworkspace-cli" and "pipx:HTTPServer"
// is "pipx-http-server".
func miseDirName(tool string) string {
	notAlnum := func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }
	var words []string
	for _, field := range strings.FieldsFunc(tool, notAlnum) {
		words = append(words, splitCase(field)...)
	}
	return strings.ToLower(strings.Join(words, "-"))
}

// letterCase is the case of the letters splitCase has seen since the last word
// break; digits leave it unchanged.
type letterCase int

const (
	caseNone letterCase = iota
	caseLower
	caseUpper
)

// splitCase splits one alphanumeric run at its case boundaries, heck's rule:
// after a lowercase letter that an uppercase one follows ("fooBar"), and
// before the last capital of an uppercase run that a lowercase letter follows
// ("HTTPServer").
func splitCase(word string) []string {
	runes := []rune(word)
	var out []string
	start, seen := 0, caseNone
	for i := 0; i+1 < len(runes); i++ {
		c, next := runes[i], runes[i+1]
		current := seen
		if unicode.IsLower(c) {
			current = caseLower
		} else if unicode.IsUpper(c) {
			current = caseUpper
		}
		switch {
		case current == caseLower && unicode.IsUpper(next):
			out = append(out, string(runes[start:i+1]))
			start, seen = i+1, caseNone
		case seen == caseUpper && unicode.IsUpper(c) && unicode.IsLower(next):
			out = append(out, string(runes[start:i]))
			start, seen = i, caseNone
		default:
			seen = current
		}
	}
	return append(out, string(runes[start:]))
}
