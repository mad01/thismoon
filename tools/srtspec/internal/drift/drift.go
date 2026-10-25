// Package drift compares a committed srt settings file to what its manifest
// renders, by meaning rather than bytes. Key order, indentation, and the
// order and duplicates of a string list do not change what srt enforces, so
// they do not count as drift; anything else does.
package drift

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/mad01/thismoon/kit/access"
)

// Canonical re-encodes a settings file in the fleet's canonical form with
// every string list sorted and deduplicated.
func Canonical(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("drift: not JSON: %w", err)
	}
	return access.Encode(sortLists(v))
}

func sortLists(v any) any {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			v[k] = sortLists(x)
		}
		return v
	case []any:
		strs := make([]string, 0, len(v))
		for _, x := range v {
			s, ok := x.(string)
			if !ok {
				for i, y := range v {
					v[i] = sortLists(y)
				}
				return v
			}
			strs = append(strs, s)
		}
		slices.Sort(strs)
		strs = slices.Compact(strs)
		out := make([]any, len(strs))
		for i, s := range strs {
			out[i] = s
		}
		return out
	default:
		return v
	}
}

// Diff returns a unified diff from the canonical form of got (the committed
// file) to that of want (the render), or "" when they agree.
func Diff(gotName string, got []byte, wantName string, want []byte) (string, error) {
	g, err := Canonical(got)
	if err != nil {
		return "", fmt.Errorf("%s: %w", gotName, err)
	}
	w, err := Canonical(want)
	if err != nil {
		return "", fmt.Errorf("%s: %w", wantName, err)
	}
	if string(g) == string(w) {
		return "", nil
	}
	return difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(g)),
		B:        difflib.SplitLines(string(w)),
		FromFile: gotName,
		ToFile:   wantName + " (rendered)",
		Context:  2,
	})
}
