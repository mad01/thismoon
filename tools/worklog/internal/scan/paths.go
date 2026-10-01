package scan

import (
	"encoding/json"
	"maps"
	"path"
	"slices"
	"strings"
	"unicode"
)

// toolInputPaths returns every absolute or ~-rooted path token in the
// tool_use blocks of an assistant message. The input object is walked
// generically: every string value at any depth is tokenised the same way, so
// a Read's file_path, a Bash command's `cd <dir>` or `git -C <dir>`, a
// worklog_checkpoint cwd, and an Agent prompt that names a checkout all
// surface without per-tool rules. The cost is that any string carrying a
// path counts, a file body written with Write included.
func toolInputPaths(content json.RawMessage) []string {
	var blocks []struct {
		Type  string          `json:"type"`
		Input json.RawMessage `json:"input"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return nil
	}
	var out []string
	for _, b := range blocks {
		if b.Type != "tool_use" || len(b.Input) == 0 {
			continue
		}
		var in any
		if json.Unmarshal(b.Input, &in) != nil {
			continue
		}
		for _, s := range collectStrings(in, nil) {
			out = append(out, pathTokens(s)...)
		}
	}
	return out
}

// collectStrings appends every string value in a decoded JSON value to out,
// descending into arrays and objects. Object keys are visited in sorted order
// so the result is stable from one run to the next.
func collectStrings(v any, out []string) []string {
	switch x := v.(type) {
	case string:
		return append(out, x)
	case []any:
		for _, e := range x {
			out = collectStrings(e, out)
		}
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(x)) {
			out = collectStrings(x[k], out)
		}
	}
	return out
}

// pathDelimiters are the characters besides whitespace that end a path token
// in shell text: operators, brackets, backticks, and the = of --flag=/path.
const pathDelimiters = "`;&|<>()[]{}=,"

// pathTokens splits s on whitespace and shell delimiters and keeps the tokens
// that start with "/" or "~/". A double or single quote that opens a token
// holds it together up to the matching quote, so a quoted path with a space
// survives whole; a quote inside a word (an apostrophe in prose) is just a
// character. Trailing dots (prose, or Go's /...) are dropped so the last
// segment stays a clean directory name.
func pathTokens(s string) []string {
	var out []string
	var tok strings.Builder
	var quote rune
	flush := func() {
		t := strings.TrimRight(tok.String(), ".")
		tok.Reset()
		if t != "" && (strings.HasPrefix(t, "/") || strings.HasPrefix(t, "~/")) {
			out = append(out, t)
		}
	}
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				tok.WriteRune(r)
			}
		case (r == '"' || r == '\'') && tok.Len() == 0:
			quote = r
		case unicode.IsSpace(r) || strings.ContainsRune(pathDelimiters, r):
			flush()
		default:
			tok.WriteRune(r)
		}
	}
	flush()
	return out
}

// underRepoMarker reports whether p contains one of cfg.RepoPathMarkers: the
// gate that separates checkouts from tmp dirs and other scratch paths.
func underRepoMarker(cfg Config, p string) bool {
	for _, m := range cfg.RepoPathMarkers {
		if strings.Contains(p, m) {
			return true
		}
	}
	return false
}

// checkoutDir returns the checkout directory a path sits in, or "" when the
// path matches none of cfg.RepoPathMarkers (the gate repoName applies to a
// cwd). Under a checkout root the path is read GOPATH-style as host/org/repo
// and the checkout is those three segments. A root whose next segment is not
// host-shaped is a layout this cannot size, so it reports nothing rather than
// guessing. Away from a checkout root, the segment right after the first
// matching marker is the checkout.
func checkoutDir(cfg Config, p string) string {
	if !underRepoMarker(cfg, p) {
		return ""
	}
	for _, root := range cfg.CheckoutRoots {
		before, rest, ok := strings.Cut(p, root)
		if !ok {
			continue
		}
		segs := strings.SplitN(rest, "/", 4)
		if len(segs) < 3 || !strings.Contains(segs[0], ".") || segs[1] == "" || segs[2] == "" {
			return ""
		}
		return before + root + path.Join(segs[0], segs[1], segs[2])
	}
	for _, m := range cfg.RepoPathMarkers {
		before, rest, ok := strings.Cut(p, m)
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(rest, "/")
		if name == "" {
			return ""
		}
		return before + m + name
	}
	return ""
}
