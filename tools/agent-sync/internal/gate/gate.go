// Package gate decides whether an entry gated to some machines (a mod, an
// MCP server) applies to this one, the way sync_mods.py and sync_mcp.py
// decided it. An entry can carry a hosts list (host names) and a profiles
// list (ralph profile tags). Each gate passes when it is empty, or when one
// of its values matches this machine, compared lowercased. An entry applies
// when both gates pass; the scripts looked at the host gate first, and a
// caller that reads the profiles gate only after the host gate passed keeps
// their error behavior too.
//
// Values reads a gate's JSON value the way both scripts iterated it. The
// sections word their own errors around it, and mcp adds one rule: a hosts
// list must hold strings.
package gate

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
)

// DefaultProfilesFile is ralph's machine-local config, whose top-level
// `profiles = [...]` array holds the machine's profile tags. Both scripts
// read it from here.
const DefaultProfilesFile = "~/.config/ralph/config.local.toml"

// Machine is what the gates compare against.
type Machine struct {
	// Host is the host name, lowercased.
	Host string
	// Profiles holds the profile tags, lowercased.
	Profiles map[string]bool
}

// Current returns this machine: its host name and the profile tags in
// profilesFile.
func Current(profilesFile string) (Machine, error) {
	host, err := os.Hostname()
	if err != nil {
		return Machine{}, fmt.Errorf("gate: host name: %w", err)
	}
	return Machine{Host: strings.ToLower(host), Profiles: readProfiles(profilesFile)}, nil
}

// HostAllowed reports whether the hosts gate lets m through: an empty gate
// lets every machine through, otherwise one entry must be m's host name.
func (m Machine) HostAllowed(hosts []string) bool {
	if len(hosts) == 0 {
		return true
	}
	for _, h := range hosts {
		if strings.ToLower(h) == m.Host {
			return true
		}
	}
	return false
}

// ProfileAllowed reports whether the profiles gate lets m through: an empty
// gate lets every machine through, otherwise m must carry one of the tags.
func (m Machine) ProfileAllowed(profiles []string) bool {
	if len(profiles) == 0 {
		return true
	}
	for _, p := range profiles {
		if m.Profiles[strings.ToLower(p)] {
			return true
		}
	}
	return false
}

// Values returns the values of a gate as the scripts iterated it, each
// rendered with Python's str(): a list by element, an object by key, a
// string by character. A falsy gate (null, false, 0, "", [] or {}) is nil,
// which lets every machine through, and so is a missing one. ok is false for
// a nonzero number or true, which Python cannot iterate: the scripts crashed
// on it with a TypeError.
func Values(raw any) (values []string, ok bool) {
	if !pyjson.Truthy(raw) {
		return nil, true
	}
	var items []any
	switch t := raw.(type) {
	case []any:
		items = t
	case *pyjson.Object:
		for _, k := range t.Keys() {
			items = append(items, k)
		}
	case string:
		for _, r := range t {
			items = append(items, string(r))
		}
	default:
		return nil, false
	}
	values = make([]string, len(items))
	for i, item := range items {
		values[i] = pyjson.Str(item)
	}
	return values, true
}

// readProfiles returns the `profiles = [...]` tags in the TOML file at path,
// each rendered with Python's str() and lowercased, as both scripts read
// them. A missing or malformed file, or a `profiles` that is not an array,
// gives no tags: an untagged machine.
func readProfiles(path string) map[string]bool {
	tags := map[string]bool{}
	var doc map[string]any
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		return tags
	}
	list, ok := doc["profiles"].([]any)
	if !ok {
		return tags
	}
	for _, p := range list {
		tags[strings.ToLower(tomlStr(p))] = true
	}
	return tags
}

// tomlStr renders a decoded TOML value the way Python's str() renders what
// tomllib decodes.
func tomlStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case int64:
		return strconv.FormatInt(t, 10)
	case bool, float64:
		return pyjson.Str(t)
	default:
		return fmt.Sprint(t)
	}
}
