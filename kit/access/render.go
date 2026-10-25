package access

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Render returns the srt settings JSON for m. It starts from the fleet
// default (no host, ~ read-denied, writes only to the temp roots), applies
// the declared access, merges m.Extra last, and checks the result still
// holds the fleet invariants.
//
// The output is deterministic: keys sorted, lists in manifest order with
// duplicates dropped, 2-space indent, trailing newline. Rendering the same
// manifest twice gives the same bytes, which is what lets CI diff it.
func Render(m Manifest) ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if m.Sandbox != SandboxSRT {
		return nil, fmt.Errorf(
			"access: sandbox is %s; only an srt manifest renders to srt settings",
			m.Sandbox,
		)
	}
	s := m.settings()
	if err := mergeInto(s, m.Extra, "extra"); err != nil {
		return nil, err
	}
	if err := CheckSettings(s); err != nil {
		return nil, err
	}
	return Encode(s)
}

// settings builds the settings tree for m before Extra. Lists are []any so
// Extra merges into them without a type switch per key.
func (m Manifest) settings() map[string]any {
	network := map[string]any{
		"allowedDomains": list(m.hosts()),
		"deniedDomains":  list(nil),
	}
	if m.Listen > 0 || len(m.Local) > 0 {
		network["allowLocalBinding"] = true
	}
	writes := dedupe(append(m.resolve(m.Writes), tempDirs...))
	s := map[string]any{
		"network": network,
		"filesystem": map[string]any{
			"denyRead":   list(dedupe(append([]string{"~"}, m.Deny...))),
			"allowRead":  list(m.resolve(m.Reads)),
			"allowWrite": list(writes),
			"denyWrite":  list(nil),
		},
	}
	if m.TLS {
		s["enableWeakerNetworkIsolation"] = true
	}
	return s
}

// hosts returns the egress entries that render, skipping EgressAtLaunch.
func (m Manifest) hosts() []string {
	var out []string
	for _, e := range m.Egress {
		if e != EgressAtLaunch {
			out = append(out, e)
		}
	}
	return dedupe(out)
}

func list(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

// mergeInto merges src into dst: objects merge key by key, lists union in
// order, and any other value from src replaces dst's. A src key whose type
// clashes with dst's (an object over a list) is an error naming the path.
func mergeInto(dst, src map[string]any, path string) error {
	keys := make([]string, 0, len(src))
	for k := range src {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		at := path + "." + k
		v := normalize(src[k])
		cur, ok := dst[k]
		if !ok {
			dst[k] = v
			continue
		}
		switch cur := cur.(type) {
		case map[string]any:
			sub, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("access: %s: want an object", at)
			}
			if err := mergeInto(cur, sub, at); err != nil {
				return err
			}
		case []any:
			items, ok := v.([]any)
			if !ok {
				return fmt.Errorf("access: %s: want a list", at)
			}
			dst[k] = union(cur, items)
		default:
			if _, isMap := v.(map[string]any); isMap {
				return fmt.Errorf("access: %s: want a scalar", at)
			}
			dst[k] = v
		}
	}
	return nil
}

// normalize turns decoded YAML into JSON-shaped values: nested maps with
// non-string keys become map[string]any, so Encode never fails on them.
func normalize(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[k] = normalize(x)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[fmt.Sprint(k)] = normalize(x)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = normalize(x)
		}
		return out
	default:
		return v
	}
}

// union appends the items of add that a does not already hold. Items compare
// by their JSON form, so it works for strings and objects alike.
func union(a, add []any) []any {
	seen := make(map[string]bool, len(a))
	out := append([]any(nil), a...)
	for _, x := range a {
		seen[jsonKey(x)] = true
	}
	for _, x := range add {
		if k := jsonKey(x); !seen[k] {
			seen[k] = true
			out = append(out, x)
		}
	}
	return out
}

func jsonKey(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// requiredLists are the six lists srt refuses a settings file without, even
// when empty.
var requiredLists = [][2]string{
	{"network", "allowedDomains"},
	{"network", "deniedDomains"},
	{"filesystem", "denyRead"},
	{"filesystem", "allowRead"},
	{"filesystem", "allowWrite"},
	{"filesystem", "denyWrite"},
}

// CheckSettings reports a settings tree that breaks a fleet invariant: a
// list srt requires is missing, a bare * host is allowed, ~ is not
// read-denied, or a write reaches the policy directories.
func CheckSettings(s map[string]any) error {
	var p problems
	lists := map[string][]string{}
	for _, kv := range requiredLists {
		section, _ := s[kv[0]].(map[string]any)
		items, ok := section[kv[1]].([]any)
		if !ok {
			p.addf("%s.%s must be a list", kv[0], kv[1])
			continue
		}
		strs := make([]string, 0, len(items))
		for _, x := range items {
			if str, ok := x.(string); ok {
				strs = append(strs, str)
			}
		}
		lists[kv[0]+"."+kv[1]] = strs
	}
	for _, d := range lists["network.allowedDomains"] {
		if d == "*" || strings.HasPrefix(d, "*:") {
			p.addf("network.allowedDomains holds %q, which allows every host", d)
		}
	}
	if _, ok := lists["filesystem.denyRead"]; ok &&
		!slices.Contains(lists["filesystem.denyRead"], "~") {
		p.addf("filesystem.denyRead must hold ~")
	}
	for _, w := range lists["filesystem.allowWrite"] {
		if dir, ok := ReachesPolicy(w); ok {
			p.addf("filesystem.allowWrite %q reaches %s, where the policy lives", w, dir)
		}
	}
	return p.err()
}

// Encode writes a settings tree as the fleet's canonical JSON: sorted keys,
// 2-space indent, no HTML escaping, trailing newline.
func Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("access: encode settings: %w", err)
	}
	return buf.Bytes(), nil
}
