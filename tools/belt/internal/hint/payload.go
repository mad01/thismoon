package hint

import (
	"encoding/json"
	"sort"
	"strings"
)

// maxPaths bounds how many result paths feed subject derivation. The common
// directory converges long before this; the cap only stops a huge result set
// from costing more than the hint is worth.
const maxPaths = 200

// PathsFromResponse pulls repo-relative file paths out of a csl search
// response. The response arrives as whatever the MCP returned — sometimes a
// JSON object, sometimes a JSON string wrapping one — and its shape differs
// between output modes (`content` yields lines[].path, files_with_matches
// yields a flat list). Rather than encode every variant, this walks the
// decoded value and collects every "path" string it finds, which is stable
// across all of them.
func PathsFromResponse(raw json.RawMessage) []string {
	v := decodeMaybeString(raw)
	if v == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	collectPaths(v, seen, &out)
	sort.Strings(out)
	if len(out) > maxPaths {
		out = out[:maxPaths]
	}
	return out
}

// RepoFromResponse reads the repo a search reported hits in, preferring the
// response over the request: a search given a regex repo filter resolves to a
// concrete repo name only in its results.
func RepoFromResponse(raw json.RawMessage) string {
	v := decodeMaybeString(raw)
	if v == nil {
		return ""
	}
	counts := map[string]int{}
	collectRepos(v, counts)
	best, n := "", 0
	for repo, c := range counts {
		if c > n || (c == n && repo < best) {
			best, n = repo, c
		}
	}
	return best
}

// decodeMaybeString handles the two envelopes a tool response arrives in: a
// JSON value, or a JSON string whose contents are themselves JSON.
func decodeMaybeString(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	if s, ok := v.(string); ok {
		var inner any
		if err := json.Unmarshal([]byte(s), &inner); err != nil {
			return nil
		}
		return inner
	}
	return v
}

func collectPaths(v any, seen map[string]bool, out *[]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if s, ok := val.(string); ok && (k == "path" || k == "file") && looksLikePath(s) {
				if !seen[s] {
					seen[s] = true
					*out = append(*out, s)
				}
				continue
			}
			collectPaths(val, seen, out)
		}
	case []any:
		for _, item := range t {
			collectPaths(item, seen, out)
		}
	}
}

func collectRepos(v any, counts map[string]int) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if s, ok := val.(string); ok && k == "repo" && s != "" {
				counts[s]++
				continue
			}
			collectRepos(val, counts)
		}
	case []any:
		for _, item := range t {
			collectRepos(item, counts)
		}
	}
}

// looksLikePath filters out the incidental strings that share the key name,
// keeping values that plausibly name a file in a repo.
func looksLikePath(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \t\n") && strings.Contains(s, ".")
}
