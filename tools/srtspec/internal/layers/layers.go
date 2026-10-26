// Package layers computes the effective srt policy srt-run launches a label
// under: the settings file ~/.config/srt/<label>.json, the machine-local
// expansion layer ~/.config/srt/<label>.local.json beside it, and the hosts
// in SRT_RUN_ALLOW_DOMAINS. srtspec run merges through it at launch, and
// explain shows what a launch would get without launching anything.
//
// The expansion layer may only widen: add to allowedDomains, allowRead,
// allowWrite, allowMachLookup and allowUnixSockets, and set
// allowLocalBinding or enableWeakerNetworkIsolation to true. run refuses to
// launch on anything else, and so does Resolve.
package layers

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mad01/thismoon/kit/access"
)

// Layer names where an effective setting came from.
type Layer string

const (
	// Base is the settings file, rendered from the access manifest.
	Base Layer = "base"
	// Local is the machine-local expansion layer.
	Local Layer = "local"
	// Env is SRT_RUN_ALLOW_DOMAINS.
	Env Layer = "env"
)

// Input names what one launch reads.
type Input struct {
	Home         string // the user's home directory
	Label        string // the srt-run label
	AllowDomains string // the value of SRT_RUN_ALLOW_DOMAINS; "" when unset
}

// Entry is one effective setting: a list item, or a scalar's JSON value.
type Entry struct {
	Key   string // dotted path, e.g. network.allowedDomains
	Value string // "" for an empty list
	Layer Layer
}

// Policy is the effective policy for one label.
type Policy struct {
	Label     string
	BasePath  string
	LocalPath string // "" when the label has no expansion layer
	Env       string // SRT_RUN_ALLOW_DOMAINS as set; "" when unset
	Entries   []Entry
}

// The keys the expansion layer may set: lists it adds to, booleans it may
// turn on, and the top-level keys that hold them (plus _comment, which srt
// ignores).
var (
	listKeys = map[string][]string{
		"network":    {"allowedDomains", "allowMachLookup", "allowUnixSockets"},
		"filesystem": {"allowRead", "allowWrite"},
	}
	flagKeys = [][]string{
		{"network", "allowLocalBinding"},
		{"enableWeakerNetworkIsolation"},
	}
	topKeys = []string{"network", "filesystem", "enableWeakerNetworkIsolation", "_comment"}
)

// Paths returns the settings file and the expansion layer for label.
func Paths(home, label string) (base, local string) {
	dir := filepath.Join(home, ".config", "srt")
	return filepath.Join(dir, label+".json"), filepath.Join(dir, label+".local.json")
}

// Resolve reads the layers for in.Label and returns the effective policy. It
// returns an error wherever srt-run would refuse to launch: a missing or
// invalid settings file, an expansion layer that does more than widen, or a
// bare * host in SRT_RUN_ALLOW_DOMAINS.
func Resolve(in Input) (Policy, error) {
	basePath, localPath := Paths(in.Home, in.Label)
	base, err := ReadObject(basePath)
	if errors.Is(err, os.ErrNotExist) {
		return Policy{}, fmt.Errorf(
			"%s does not exist; srt fails closed and %s never starts",
			basePath,
			in.Label,
		)
	}
	if err != nil {
		return Policy{}, err
	}
	p := Policy{Label: in.Label, BasePath: basePath, Env: in.AllowDomains}
	p.Entries = flatten("", base, Base)

	local, err := ReadObject(localPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return Policy{}, fmt.Errorf("srt-run refuses to launch %s: %w", in.Label, err)
	default:
		if err := CheckLocal(base, local, in.Home); err != nil {
			return Policy{}, fmt.Errorf(
				"srt-run refuses to launch %s: %s: %w",
				in.Label,
				localPath,
				err,
			)
		}
		p.LocalPath = localPath
		p.Entries = widen(p.Entries, local)
	}

	hosts, err := Hosts(in.AllowDomains)
	if err != nil {
		return Policy{}, fmt.Errorf("srt-run refuses to launch %s: %w", in.Label, err)
	}
	for _, h := range hosts {
		p.Entries = addItem(p.Entries, "network.allowedDomains", h, Env)
	}
	slices.SortStableFunc(p.Entries, func(a, b Entry) int { return strings.Compare(a.Key, b.Key) })
	return p, nil
}

// ReadObject reads a settings file or expansion layer as a JSON object. A
// missing file comes back as os.ErrNotExist.
func ReadObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("%s is not a JSON object: %w", path, err)
	}
	return obj, nil
}

// CheckLocal reports every way local does more than widen base, or nil when
// it only widens. home turns absolute paths under it back into ~ form before
// comparing.
func CheckLocal(base, local map[string]any, home string) error {
	reasons := LocalProblems(base, local, home)
	if len(reasons) == 0 {
		return nil
	}
	return errors.New(strings.Join(reasons, "; "))
}

// LocalProblems lists every way local does more than widen base, one reason
// per entry in the order run checks them, and nothing when it only widens.
func LocalProblems(base, local map[string]any, home string) []string {
	var reasons []string
	add := func(format string, args ...any) { reasons = append(reasons, fmt.Sprintf(format, args...)) }

	for _, k := range sortedKeys(local) {
		if !slices.Contains(topKeys, k) {
			add("key %s is not allowed; the layer may only widen", k)
		}
	}
	for _, section := range []string{"network", "filesystem"} {
		v, ok := local[section]
		if !ok {
			continue
		}
		obj, isObj := v.(map[string]any)
		if !isObj {
			add("%s must be an object", section)
			continue
		}
		for _, k := range sortedKeys(obj) {
			if !widens(section, k) {
				add("%s.%s is not allowed; the layer may only widen", section, k)
			}
		}
	}

	denies := map[string]bool{}
	for _, key := range []string{"denyRead", "denyWrite"} {
		for _, d := range stringItems(dig(base, "filesystem", key)) {
			denies[norm(d, home)] = true
		}
	}
	for _, section := range []string{"network", "filesystem"} {
		for _, k := range listKeys[section] {
			v, ok := lookup(local, section, k)
			if !ok {
				continue
			}
			items, valid := stringList(v)
			if !valid {
				add("%s.%s must be a list of non-empty strings", section, k)
				continue
			}
			for _, item := range items {
				for _, r := range itemReasons(section+"."+k, item, home, denies) {
					add("%s", r)
				}
			}
		}
	}
	for _, path := range flagKeys {
		if v, ok := lookup(local, path...); ok && v != true {
			add("%s may only be set to true", strings.Join(path, "."))
		}
	}
	return reasons
}

// Merge returns base widened by local and hosts: the settings run hands
// srt. Each list the layer adds to becomes the sorted union, each flag it
// sets turns on, and hosts (SRT_RUN_ALLOW_DOMAINS, already split) join
// network.allowedDomains, which comes out sorted and deduplicated either
// way. Merge checks nothing; call CheckLocal or LocalProblems first. Neither
// argument is modified.
func Merge(base, local map[string]any, hosts []string) map[string]any {
	out := maps.Clone(base)
	if out == nil {
		out = map[string]any{}
	}
	for _, section := range []string{"network", "filesystem"} {
		if m, ok := out[section].(map[string]any); ok {
			out[section] = maps.Clone(m)
		}
	}
	for _, section := range []string{"network", "filesystem"} {
		for _, k := range listKeys[section] {
			v, ok := lookup(local, section, k)
			if !ok {
				continue
			}
			items, _ := stringList(v)
			sec := object(out, section)
			sec[k] = anyList(union(stringItems(sec[k]), items))
		}
	}
	for _, path := range flagKeys {
		if v, ok := lookup(local, path...); ok && v == true {
			if len(path) == 1 {
				out[path[0]] = true
			} else {
				object(out, path[0])[path[1]] = true
			}
		}
	}
	net := object(out, "network")
	net["allowedDomains"] = anyList(union(stringItems(net["allowedDomains"]), hosts))
	return out
}

// object returns out[key] as a map, creating it when absent.
func object(out map[string]any, key string) map[string]any {
	if m, ok := out[key].(map[string]any); ok {
		return m
	}
	m := map[string]any{}
	out[key] = m
	return m
}

// union is the sorted, deduplicated set of both lists, the way jq's unique
// read them.
func union(a, b []string) []string {
	all := slices.Concat(a, b)
	slices.Sort(all)
	return slices.Compact(all)
}

func anyList(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// widens reports whether the layer may set key k inside section.
func widens(section, k string) bool {
	if slices.Contains(listKeys[section], k) {
		return true
	}
	for _, path := range flagKeys {
		if len(path) == 2 && path[0] == section && path[1] == k {
			return true
		}
	}
	return false
}

// itemReasons checks one added list item.
func itemReasons(key, item, home string, denies map[string]bool) []string {
	var out []string
	switch key {
	case "network.allowedDomains":
		if item == "*" || strings.HasPrefix(item, "*:") {
			out = append(out, fmt.Sprintf("%s may not hold %q, which allows every host", key, item))
		}
	case "filesystem.allowRead", "filesystem.allowWrite":
		p := norm(item, home)
		if p == "" || p == "~" {
			out = append(out, fmt.Sprintf("%s %q would lift the ~ deny", key, item))
		}
		if denies[p] {
			out = append(out, fmt.Sprintf("%s %q would remove the deny on %s", key, item, p))
		}
		if key == "filesystem.allowWrite" {
			if dir, ok := access.ReachesPolicy(p); ok {
				out = append(
					out,
					fmt.Sprintf("%s %q reaches %s, where the policy lives", key, item, dir),
				)
			}
		}
	}
	return out
}

// norm writes a path the way the settings files do: an absolute path under
// home in ~ form, without a trailing slash.
func norm(p, home string) string {
	if home != "" {
		switch {
		case p == home:
			p = "~"
		case strings.HasPrefix(p, home+"/"):
			p = "~" + p[len(home):]
		}
	}
	return strings.TrimRight(p, "/")
}

// widen adds what a checked expansion layer contributes.
func widen(entries []Entry, local map[string]any) []Entry {
	for _, section := range []string{"network", "filesystem"} {
		for _, k := range listKeys[section] {
			v, ok := lookup(local, section, k)
			if !ok {
				continue
			}
			items, _ := stringList(v)
			for _, item := range items {
				entries = addItem(entries, section+"."+k, item, Local)
			}
		}
	}
	for _, path := range flagKeys {
		if v, ok := lookup(local, path...); ok && v == true {
			entries = setTrue(entries, strings.Join(path, "."))
		}
	}
	return entries
}

// Hosts splits SRT_RUN_ALLOW_DOMAINS (comma-separated host[:port], empty
// entries ignored), refusing a bare * or *:port entry.
func Hosts(v string) ([]string, error) {
	var hosts []string
	for _, h := range strings.Split(v, ",") {
		if h == "" {
			continue
		}
		if h == "*" || strings.HasPrefix(h, "*:") {
			return nil, fmt.Errorf("SRT_RUN_ALLOW_DOMAINS may not hold a bare * entry: %s", v)
		}
		hosts = append(hosts, h)
	}
	return hosts, nil
}

// flatten turns a settings tree into entries: one per list item, one per
// scalar, and an empty-valued one for an empty list.
func flatten(prefix string, obj map[string]any, layer Layer) []Entry {
	var out []Entry
	for _, k := range sortedKeys(obj) {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch v := obj[k].(type) {
		case map[string]any:
			out = append(out, flatten(key, v, layer)...)
		case []any:
			if len(v) == 0 {
				out = append(out, Entry{Key: key, Layer: layer})
			}
			for _, item := range v {
				out = append(out, Entry{Key: key, Value: display(item), Layer: layer})
			}
		default:
			out = append(out, Entry{Key: key, Value: display(v), Layer: layer})
		}
	}
	return out
}

// display renders a value for the explain table: strings bare, anything else
// as JSON.
func display(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// addItem appends a list item unless the key already holds it, replacing
// the placeholder entry of an empty list.
func addItem(entries []Entry, key, value string, layer Layer) []Entry {
	for i, e := range entries {
		if e.Key != key {
			continue
		}
		if e.Value == value {
			return entries
		}
		if e.Value == "" {
			entries[i] = Entry{Key: key, Value: value, Layer: layer}
			return entries
		}
	}
	return append(entries, Entry{Key: key, Value: value, Layer: layer})
}

// setTrue makes a boolean key true, crediting layer only when it changed it.
func setTrue(entries []Entry, key string) []Entry {
	for i, e := range entries {
		if e.Key == key {
			if e.Value != "true" {
				entries[i] = Entry{Key: key, Value: "true", Layer: Local}
			}
			return entries
		}
	}
	return append(entries, Entry{Key: key, Value: "true", Layer: Local})
}

func lookup(obj map[string]any, path ...string) (any, bool) {
	var cur any = obj
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func dig(obj map[string]any, path ...string) any {
	v, _ := lookup(obj, path...)
	return v
}

// stringList returns v as strings when it is a list of non-empty strings.
func stringList(v any) ([]string, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, x := range items {
		s, ok := x.(string)
		if !ok || s == "" {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// stringItems returns the string items of v, ignoring anything else.
func stringItems(v any) []string {
	items, _ := v.([]any)
	var out []string
	for _, x := range items {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
