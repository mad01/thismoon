package mods

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/tools/agent-sync/internal/claudecode"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pypath"
)

// state is the plugin state Claude Code persists, which the sync reads and
// never writes:
//
//	<config>/plugins/known_marketplaces.json  marketplace name to source path
//	<config>/plugins/installed_plugins.json   install records (plugin id to scopes)
//	<config>/settings.json                    enabledPlugins (plugin id to bool)
type state struct {
	// marketplaces maps each registered directory marketplace to the path
	// it was recorded at.
	marketplaces map[string]string
	// installed maps each plugin id with a user-scope install record to the
	// version the record carries, "" when it carries none.
	installed map[string]string
	// enabled is enabledPlugins, bool values only.
	enabled map[string]bool
}

// claudeDir is Claude Code's config directory, which holds the three files
// the claude plugin CLI keeps its state in: $CLAUDE_CONFIG_DIR, or
// ~/.claude when that is unset or empty, with a leading ~ expanded.
func claudeDir() (string, error) {
	dir := os.Getenv(claudecode.ConfigDirEnv)
	if dir == "" {
		dir = claudecode.DefaultConfigDir
	}
	return pypath.ExpandUser(dir)
}

// readState reads the three state files under dir. A missing file is empty
// state; a malformed one is an error naming the file.
func readState(dir string) (state, error) {
	plugins := pypath.Join(dir, "plugins")
	marketplaces, err := readMarketplaces(pypath.Join(plugins, "known_marketplaces.json"))
	if err != nil {
		return state{}, err
	}
	installed, err := readInstalled(pypath.Join(plugins, "installed_plugins.json"))
	if err != nil {
		return state{}, err
	}
	enabled, err := readEnabled(pypath.Join(dir, "settings.json"))
	if err != nil {
		return state{}, err
	}
	return state{marketplaces: marketplaces, installed: installed, enabled: enabled}, nil
}

// readMarketplaces returns the directory marketplaces known_marketplaces.json
// records. An entry with no source, or one that is not a directory, is not
// one of them.
func readMarketplaces(path string) (map[string]string, error) {
	doc, err := readObject(path, "top level", pyjson.NewObject())
	if err != nil {
		return nil, err
	}
	marketplaces := map[string]string{}
	for _, name := range doc.Keys() {
		entry, err := expectObject(get(doc, name), "marketplace "+pyjson.Repr(name), path)
		if err != nil {
			return nil, err
		}
		raw := get(entry, "source")
		if raw == nil {
			continue
		}
		source, err := expectObject(raw, "marketplace "+pyjson.Repr(name)+" source", path)
		if err != nil {
			return nil, err
		}
		if kind, _ := get(source, "source").(string); kind == "directory" &&
			pyjson.Truthy(get(source, "path")) {
			marketplaces[name] = pypath.Clean(pyjson.Str(get(source, "path")))
		}
	}
	return marketplaces, nil
}

// readInstalled returns the user-scope install records in
// installed_plugins.json: plugin id to the recorded version.
func readInstalled(path string) (map[string]string, error) {
	doc, err := readObject(path, "top level", pyjson.NewObject())
	if err != nil {
		return nil, err
	}
	raw := get(doc, "plugins")
	if !pyjson.Truthy(raw) {
		raw = pyjson.NewObject()
	}
	records, err := expectObject(raw, "'plugins'", path)
	if err != nil {
		return nil, err
	}
	installed := map[string]string{}
	for _, id := range records.Keys() {
		entries, ok := get(records, id).([]any)
		if !ok {
			return nil, fmt.Errorf(
				"%s: plugin %s must be an array of records",
				path,
				pyjson.Repr(id),
			)
		}
		if user := userRecord(entries); user != nil {
			version, _ := get(user, "version").(string)
			installed[id] = version
		}
	}
	return installed, nil
}

// userRecord returns the first user-scope record among entries, or nil.
func userRecord(entries []any) *pyjson.Object {
	for _, e := range entries {
		if obj, ok := e.(*pyjson.Object); ok {
			if scope, _ := get(obj, "scope").(string); scope == "user" {
				return obj
			}
		}
	}
	return nil
}

// readEnabled returns the bool entries of enabledPlugins in settings.json.
func readEnabled(path string) (map[string]bool, error) {
	doc, err := readObject(path, "top level", pyjson.NewObject())
	if err != nil {
		return nil, err
	}
	raw := get(doc, "enabledPlugins")
	if !pyjson.Truthy(raw) {
		raw = pyjson.NewObject()
	}
	plugins, err := expectObject(raw, "'enabledPlugins'", path)
	if err != nil {
		return nil, err
	}
	enabled := map[string]bool{}
	for _, id := range plugins.Keys() {
		if b, ok := get(plugins, id).(bool); ok {
			enabled[id] = b
		}
	}
	return enabled, nil
}
