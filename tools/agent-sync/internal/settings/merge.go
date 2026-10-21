package settings

import (
	"slices"

	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
)

// permissionLists are the key paths whose arrays union when the overlay
// merges. Every other overlay array replaces the template's copy, so an
// overlay carrying a hook list still clobbers the template's hooks.
var permissionLists = []KeyPath{
	{"permissions", "allow"},
	{"permissions", "deny"},
	{"permissions", "ask"},
	{"permissions", "additionalDirectories"},
}

// unionLists returns base's entries in order, then overlay's entries base
// lacks, dropping duplicates (including any inside base) by Python ==.
func unionLists(base, overlay []any) []any {
	merged := []any{}
	for _, item := range append(append([]any{}, base...), overlay...) {
		if !pyjson.Contains(merged, item) {
			merged = append(merged, item)
		}
	}
	return merged
}

// deepMerge merges overlay into a copy of base; overlay wins. Objects merge
// key by key, arrays and scalars are replaced, except an array pair whose key
// path is in unionAt, which unions. A key new to base goes last, an existing
// one keeps its position. Neither input is modified.
func deepMerge(base, overlay *pyjson.Object, unionAt []KeyPath, path KeyPath) *pyjson.Object {
	merged := base.Clone()
	for _, key := range overlay.Keys() {
		val, _ := overlay.Get(key)
		here := append(append(KeyPath{}, path...), key)
		current, _ := merged.Get(key)
		curObj, curIsObj := current.(*pyjson.Object)
		valObj, valIsObj := val.(*pyjson.Object)
		curList, curIsList := current.([]any)
		valList, valIsList := val.([]any)
		switch {
		case curIsObj && valIsObj:
			merged.Set(key, deepMerge(curObj, valObj, unionAt, here))
		case containsPath(unionAt, here) && curIsList && valIsList:
			merged.Set(key, unionLists(curList, valList))
		default:
			merged.Set(key, val)
		}
	}
	return merged
}

// mergeOverlay merges the local overlay: permission lists union, everything
// else follows deepMerge.
func mergeOverlay(base, overlay *pyjson.Object) *pyjson.Object {
	return deepMerge(base, overlay, permissionLists, nil)
}

// getPath returns the value at path and whether every key on it exists.
func getPath(data any, path KeyPath) (any, bool) {
	node := data
	for _, key := range path {
		obj, ok := node.(*pyjson.Object)
		if !ok {
			return nil, false
		}
		if node, ok = obj.Get(key); !ok {
			return nil, false
		}
	}
	return node, true
}

// prune returns a deep copy of data without the key paths. A path whose key
// or parent is absent is skipped.
func prune(data *pyjson.Object, paths []KeyPath) *pyjson.Object {
	pruned := pyjson.DeepCopy(data).(*pyjson.Object)
	for _, path := range paths {
		parent, ok := getPath(pruned, path[:len(path)-1])
		if obj, isObj := parent.(*pyjson.Object); ok && isObj {
			obj.Delete(path[len(path)-1])
		}
	}
	return pruned
}

// pruneConflicts returns the prune paths the template still carries.
func pruneConflicts(template *pyjson.Object, paths []KeyPath) []KeyPath {
	var conflicts []KeyPath
	for _, path := range paths {
		if _, ok := getPath(template, path); ok {
			conflicts = append(conflicts, path)
		}
	}
	return conflicts
}

// isUnder reports whether keys is one of paths or nested inside one.
func isUnder(keys KeyPath, paths []KeyPath) bool {
	return slices.ContainsFunc(paths, func(path KeyPath) bool {
		return len(keys) >= len(path) && slices.Equal(keys[:len(path)], path)
	})
}

func containsPath(paths []KeyPath, p KeyPath) bool {
	return slices.ContainsFunc(paths, func(q KeyPath) bool { return slices.Equal(p, q) })
}

// diffKind says how a key differs between live and merged.
type diffKind int

const (
	diffChanged diffKind = iota // in both, values differ
	diffAdded                   // only in merged
	diffRemoved                 // only in live
)

// diff is one key whose value differs between live and merged.
type diff struct {
	kind          diffKind
	path          KeyPath
	before, after any
}

// collectDiff walks live and merged in sorted key order and returns every
// key that differs. Two objects at the same key are walked into rather than
// reported whole.
func collectDiff(live, merged *pyjson.Object, path KeyPath) []diff {
	keys := map[string]bool{}
	for _, k := range live.Keys() {
		keys[k] = true
	}
	for _, k := range merged.Keys() {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	// Byte order of UTF-8 is code point order, which is Python's sorted().
	slices.Sort(sorted)

	var diffs []diff
	for _, key := range sorted {
		full := append(append(KeyPath{}, path...), key)
		old, inLive := live.Get(key)
		val, inMerged := merged.Get(key)
		oldObj, oldIsObj := old.(*pyjson.Object)
		valObj, valIsObj := val.(*pyjson.Object)
		switch {
		case inLive && !inMerged:
			diffs = append(diffs, diff{kind: diffRemoved, path: full, before: old})
		case inMerged && !inLive:
			diffs = append(diffs, diff{kind: diffAdded, path: full, after: val})
		case oldIsObj && valIsObj:
			diffs = append(diffs, collectDiff(oldObj, valObj, full)...)
		// The script compared with != here, where NaN != NaN; Equal counts
		// two NaNs equal (a deliberate deviation, see pyjson.Equal).
		case !pyjson.Equal(old, val):
			diffs = append(diffs, diff{kind: diffChanged, path: full, before: old, after: val})
		}
	}
	return diffs
}

// localOnly is a key live carries that the expected config does not.
type localOnly struct {
	path string
	val  any
}

// findLocalOnly returns the keys in live absent from expected, in live's key
// order, walking into objects both sides carry.
func findLocalOnly(live, expected *pyjson.Object, prefix string) []localOnly {
	var found []localOnly
	for _, key := range live.Keys() {
		full := key
		if prefix != "" {
			full = prefix + "." + key
		}
		val, _ := live.Get(key)
		want, ok := expected.Get(key)
		if !ok {
			found = append(found, localOnly{path: full, val: val})
			continue
		}
		valObj, valIsObj := val.(*pyjson.Object)
		wantObj, wantIsObj := want.(*pyjson.Object)
		if valIsObj && wantIsObj {
			found = append(found, findLocalOnly(valObj, wantObj, full)...)
		}
	}
	return found
}
