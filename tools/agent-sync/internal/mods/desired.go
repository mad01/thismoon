package mods

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/mad01/thismoon/tools/agent-sync/internal/gate"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pypath"
)

// Where a marketplace and its plugins keep their manifests.
const (
	marketplaceManifest = ".claude-plugin/marketplace.json"
	pluginManifest      = ".claude-plugin/plugin.json"
)

// want is a mod's desired state on this machine.
type want int

const (
	// gatedOff: a gate keeps the mod off this machine; leave it alone.
	gatedOff want = iota
	// wantEnabled: installed and enabled.
	wantEnabled
	// wantDisabled: disabled when it is enabled, never installed for it.
	wantDisabled
	// wantRetired (`installed: false`): no install record.
	wantRetired
)

// mod is one entry of a mods file.
type mod struct {
	name string
	want want
}

// modsFile is one parsed mods file, the base or a drop-in: the directory
// marketplace it registers and the mods it reconciles from it.
type modsFile struct {
	source      string // the file, for messages
	marketplace string
	path        string // the marketplace directory, ~ expanded
	mods        []mod  // in file order
}

// loadSources reads the base mods file, then every *.json drop-in in sorted
// name order, hidden ones included, as pathlib's glob lists them, and gates
// their mods for this machine. A drop-in whose target is gone (a dangling
// symlink) is skipped with a note. Marketplace and mod names must be unique
// across all of them.
func loadSources(cfg Config) ([]modsFile, []string, error) {
	if _, err := os.Stat(cfg.File); errors.Is(err, fs.ErrNotExist) {
		return nil, nil, fmt.Errorf("mods file not found: %s", cfg.File)
	}
	m, err := gate.Current(cfg.ProfilesFile)
	if err != nil {
		return nil, nil, err
	}
	base, err := loadModsFile(cfg.File, m)
	if err != nil {
		return nil, nil, err
	}
	sources := []modsFile{base}
	var notes []string
	for _, path := range pypath.Glob(cfg.Dropins, "*.json") {
		if !isFile(path) {
			notes = append(notes, fmt.Sprintf(
				"claude-mods: drop-in %s points at a missing file (%s); skipped",
				path,
				pypath.RealPath(path),
			))
			continue
		}
		f, err := loadModsFile(path, m)
		if err != nil {
			return nil, nil, err
		}
		sources = append(sources, f)
	}
	if err := rejectDuplicates(sources); err != nil {
		return nil, nil, err
	}
	return sources, notes, nil
}

// loadModsFile parses one mods file and gates its mods for m. Each entry is
// validated before it is gated, so a malformed entry fails on every machine.
func loadModsFile(path string, m gate.Machine) (modsFile, error) {
	data, err := readObject(path, "top level", nil)
	if err != nil {
		return modsFile{}, err
	}
	mkt, err := expectObject(get(data, "marketplace"), "'marketplace'", path)
	if err != nil {
		return modsFile{}, err
	}
	name, dir := get(mkt, "name"), get(mkt, "path")
	if !pyjson.Truthy(name) || !pyjson.Truthy(dir) {
		return modsFile{}, fmt.Errorf("%s: 'marketplace' needs 'name' and 'path'", path)
	}
	entries, err := expectObject(get(data, "mods"), "'mods'", path)
	if err != nil {
		return modsFile{}, err
	}
	f := modsFile{source: path, marketplace: pyjson.Str(name)}
	for _, key := range entries.Keys() {
		if strings.HasPrefix(key, "_") {
			continue
		}
		spec, err := expectObject(get(entries, key), "mod "+pyjson.Repr(key), path)
		if err != nil {
			return modsFile{}, err
		}
		w, err := modWant(key, spec, path)
		if err != nil {
			return modsFile{}, err
		}
		allowed, err := allows(m, spec, key, path)
		if err != nil {
			return modsFile{}, err
		}
		if !allowed {
			w = gatedOff
		}
		f.mods = append(f.mods, mod{name: key, want: w})
	}
	if f.path, err = pypath.ExpandUser(pyjson.Str(dir)); err != nil {
		return modsFile{}, fmt.Errorf("%s: 'marketplace' path: %w", path, err)
	}
	return f, nil
}

// modWant is a mod's desired state before gating: its `enabled` flag (true
// when absent), or wantRetired for `installed: false`. A retired mod cannot
// also be enabled, so `enabled` true beside it is an error.
func modWant(name string, spec *pyjson.Object, path string) (want, error) {
	installed := true
	if v, ok := spec.Get("installed"); ok {
		b, isBool := v.(bool)
		if !isBool {
			return 0, fmt.Errorf("%s: mod %s: 'installed' must be true or false",
				path, pyjson.Repr(name))
		}
		installed = b
	}
	enabled, hasEnabled := spec.Get("enabled")
	if installed {
		if !hasEnabled || pyjson.Truthy(enabled) {
			return wantEnabled, nil
		}
		return wantDisabled, nil
	}
	if pyjson.Truthy(enabled) {
		return 0, fmt.Errorf("%s: mod %s sets 'installed' false and 'enabled' true; "+
			"drop 'enabled' or set it false", path, pyjson.Repr(name))
	}
	return wantRetired, nil
}

// rejectDuplicates fails on a marketplace or mod name that two files
// declare. Plugin ids are <mod>@<marketplace>, so a mod name repeated under
// another marketplace would not collide inside Claude Code; it is rejected
// anyway so a mod name means one thing across the fleet.
func rejectDuplicates(sources []modsFile) error {
	marketplaces := map[string]string{}
	mods := map[string]string{}
	for _, f := range sources {
		if first, dup := marketplaces[f.marketplace]; dup {
			return fmt.Errorf("%s: marketplace %s is already declared by %s; "+
				"drop-in marketplace names must differ", f.source, pyjson.Repr(f.marketplace), first)
		}
		marketplaces[f.marketplace] = f.source
		for _, m := range f.mods {
			if first, dup := mods[m.name]; dup {
				return fmt.Errorf("%s: mod %s is already declared by %s; "+
					"mod names must be unique across mods.json and mods.d/",
					f.source, pyjson.Repr(m.name), first)
			}
			mods[m.name] = f.source
		}
	}
	return nil
}

// listing returns the plugins the marketplace lists, name to its manifest
// `source`, and false when the marketplace is not on disk. Claude Code
// registers a marketplace under the name in its manifest, so the declared
// name must match it.
func listing(f modsFile) (map[string]any, bool, error) {
	manifest := pypath.Join(f.path, marketplaceManifest)
	if !isFile(manifest) {
		return nil, false, nil
	}
	data, err := readObject(manifest, "top level", nil)
	if err != nil {
		return nil, false, err
	}
	if name, ok := get(data, "name").(string); !ok || name != f.marketplace {
		return nil, false, fmt.Errorf("%s: marketplace %s does not match the name %s in %s; "+
			"Claude Code registers under the manifest's name",
			f.source, pyjson.Repr(f.marketplace), pyjson.Repr(get(data, "name")), manifest)
	}
	plugins, ok := get(data, "plugins").([]any)
	if !ok {
		return nil, false, fmt.Errorf("%s: 'plugins' must be an array", manifest)
	}
	listed := map[string]any{}
	for _, p := range plugins {
		obj, ok := p.(*pyjson.Object)
		if !ok || !pyjson.Truthy(get(obj, "name")) {
			continue
		}
		listed[pyjson.Str(get(obj, "name"))] = get(obj, "source")
	}
	return listed, true, nil
}

// folderVersions reads the folder version of every listed mod f enables,
// for the update rule. A version that cannot be read is a note, never an
// error: that mod is not checked for updates.
func folderVersions(f modsFile, listed map[string]any) (map[string]string, []string) {
	versions := map[string]string{}
	var notes []string
	for _, m := range f.mods {
		source, ok := listed[m.name]
		if m.want != wantEnabled || !ok {
			continue
		}
		v, err := folderVersion(f.path, source)
		if err != nil {
			notes = append(notes, fmt.Sprintf(
				"note: %s@%s: cannot tell its marketplace version (%s); not checked for updates",
				m.name, f.marketplace, pypath.ErrorText(err),
			))
			continue
		}
		versions[m.name] = v
	}
	return versions, notes
}

// folderVersion returns the `version` in the plugin.json of the plugin the
// marketplace at dir lists at source.
func folderVersion(dir string, source any) (string, error) {
	rel, ok := source.(string)
	if !ok || rel == "" {
		return "", fmt.Errorf("manifest source %s is not a relative path", pyjson.Repr(source))
	}
	manifest := pypath.Join(pypath.Join(dir, rel), pluginManifest)
	data, err := pyjson.ReadFile(manifest)
	if err != nil {
		return "", err
	}
	var version any
	if obj, ok := data.(*pyjson.Object); ok {
		version = get(obj, "version")
	}
	if v, ok := version.(string); ok && v != "" {
		return v, nil
	}
	return "", fmt.Errorf("%s has no 'version'", manifest)
}

// readObject reads the JSON file at path, which must hold an object. A
// missing file reads as missing, which is nil (and so not an object) unless
// the caller passes a default.
func readObject(path, what string, missing *pyjson.Object) (*pyjson.Object, error) {
	v, err := pyjson.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if missing != nil {
			return missing, nil
		}
		v = nil
	case err != nil:
		return nil, fmt.Errorf("%s: %s", path, pypath.ErrorText(err))
	}
	return expectObject(v, what, path)
}

// expectObject returns v when it is an object, and otherwise an error naming
// the file and the field.
func expectObject(v any, what, path string) (*pyjson.Object, error) {
	obj, ok := v.(*pyjson.Object)
	if !ok {
		return nil, fmt.Errorf("%s: %s must be an object", path, what)
	}
	return obj, nil
}

// get is dict.get(key): the value, or nil (None) when key is absent.
func get(obj *pyjson.Object, key string) any {
	v, _ := obj.Get(key)
	return v
}

// isFile is Path.is_file(): a regular file once symlinks are followed.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
