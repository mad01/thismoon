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

// gateValues returns the values of spec's gate key as gate.Values reads
// them. A value Python cannot iterate crashed the script; here it is an
// error.
func gateValues(spec *pyjson.Object, key, mod, path string) ([]string, error) {
	raw, _ := spec.Get(key)
	values, ok := gate.Values(raw)
	if !ok {
		return nil, fmt.Errorf("%s: mod %s: '%s' must be an array, got %s",
			path, pyjson.Repr(mod), key, pyjson.Describe(raw))
	}
	return values, nil
}
