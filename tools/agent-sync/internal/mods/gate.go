package mods

import (
	"fmt"

	"github.com/mad01/thismoon/tools/agent-sync/internal/gate"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
)

// allows reports whether spec's hosts and profiles gates let the mod run on
// m. As in the script, the host gate goes first and the profiles gate is
// not read when it fails.
func allows(m gate.Machine, spec *pyjson.Object, mod, path string) (bool, error) {
	hosts, err := gateValues(spec, "hosts", mod, path)
	if err != nil || !m.HostAllowed(hosts) {
		return false, err
	}
	profiles, err := gateValues(spec, "profiles", mod, path)
	if err != nil {
		return false, err
	}
	return m.ProfileAllowed(profiles), nil
}

// gateValues returns the values of spec's gate key, each rendered with
// Python's str(), or nil when the gate is empty or missing and so lets every
// machine through. A gate iterates the way Python iterates it: a list by
// element, an object by key, a string by character. Anything else crashed
// the script; here it is an error.
func gateValues(spec *pyjson.Object, key, mod, path string) ([]string, error) {
	raw, _ := spec.Get(key)
	if !pyjson.Truthy(raw) {
		return nil, nil
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
		return nil, fmt.Errorf("%s: mod %s: '%s' must be an array, got %s",
			path, pyjson.Repr(mod), key, pyjson.Describe(t))
	}
	values := make([]string, len(items))
	for i, item := range items {
		values[i] = pyjson.Str(item)
	}
	return values, nil
}
