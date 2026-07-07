package rules

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
)

// Rule describes a single humanizer detection rule. The fields are
// populated from the `humanizer-*` YAML comment headers in each rule
// file under vale/styles/Humanizer/.
type Rule struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Category        string `json:"category"`
	DefaultSeverity string `json:"default_severity"`
	Summary         string `json:"summary"`
	Rationale       string `json:"rationale,omitempty"`
	Before          string `json:"before,omitempty"`
	After           string `json:"after,omitempty"`
	Reference       string `json:"reference,omitempty"`
	// File is the relative path within the embedded pack (e.g.
	// "vale/styles/Humanizer/AIVocabulary.yml"). Useful for debugging and
	// for MCP clients that want to inspect the raw rule.
	File string `json:"file,omitempty"`
}

// Rule metadata is parsed once on first access, then cached. Both the
// ordered slice and an ID-indexed map are built so All() and Get() are
// each O(1) to O(n) as appropriate.
var (
	loadOnce    sync.Once
	loadedRules []Rule
	loadedByID  map[string]Rule
)

func load() {
	loadOnce.Do(func() {
		loadedRules = parseAllRules()
		loadedByID = make(map[string]Rule, len(loadedRules))
		for _, r := range loadedRules {
			loadedByID[r.ID] = r
		}
	})
}

// All returns every rule in the embedded pack, sorted by category then ID.
func All() []Rule {
	load()
	out := make([]Rule, len(loadedRules))
	copy(out, loadedRules)
	return out
}

// Get looks up a rule by ID ("Humanizer.AIVocabulary"). Returns
// (zero, false) if unknown.
func Get(id string) (Rule, bool) {
	load()
	r, ok := loadedByID[id]
	return r, ok
}

// byID returns the internal ID-indexed map for read-only use inside the
// package. Callers must not mutate the returned map.
func byID() map[string]Rule {
	load()
	return loadedByID
}

// IDs returns every known rule ID.
func IDs() []string {
	rs := All()
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

// Categories returns every distinct category that appears in the pack.
func Categories() []string {
	load()
	seen := map[string]struct{}{}
	for _, r := range loadedRules {
		seen[r.Category] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// parseAllRules walks the embedded pack and returns every rule with a
// populated metadata block. Rules whose YAML lacks the humanizer-id
// header are skipped.
func parseAllRules() []Rule {
	var out []Rule
	root := "vale/styles/Humanizer"
	entries, err := fs.ReadDir(valeFS, root)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		if name == "meta.yml" {
			continue
		}
		full := path.Join(root, name)
		data, err := fs.ReadFile(valeFS, full)
		if err != nil {
			continue
		}
		r, ok := parseMetadata(data)
		if !ok {
			continue
		}
		r.File = full
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// parseMetadata reads the `# humanizer-<key>: value` comment block at
// the top of a Vale rule file plus the `level: <sev>` setting. Supports
// single-line values and block values written as YAML-style `|` scalars.
//
// Returns (rule, true) when humanizer-id is set, (zero, false) otherwise.
func parseMetadata(data []byte) (Rule, bool) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var r Rule
	var currentKey string
	var blockBuf []string
	flushBlock := func() {
		if currentKey == "" {
			return
		}
		v := strings.TrimRight(strings.Join(blockBuf, "\n"), "\n")
		setField(&r, currentKey, v)
		currentKey = ""
		blockBuf = nil
	}
	inBlock := false
	for sc.Scan() {
		line := sc.Text()

		// Capture level: suggestion|warning|error.
		if !inBlock && strings.HasPrefix(line, "level:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "level:"))
			r.DefaultSeverity = v
			continue
		}

		if !strings.HasPrefix(line, "#") {
			flushBlock()
			inBlock = false
			continue
		}

		body := strings.TrimPrefix(line, "#")
		body = strings.TrimPrefix(body, " ")

		if inBlock {
			// YAML block value continues until we hit a non-indented comment
			// or a non-comment line. Block lines inside a `#`-comment keep
			// a leading "  " from YAML indentation; preserve it after the
			// comment marker is stripped.
			if !strings.HasPrefix(body, "humanizer-") {
				blockBuf = append(blockBuf, body)
				continue
			}
			// hit a new humanizer-* key — flush the previous block
			flushBlock()
			inBlock = false
		}

		if !strings.HasPrefix(body, "humanizer-") {
			continue
		}
		rawKey, rawValue, ok := strings.Cut(body, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(rawKey)
		value := strings.TrimSpace(rawValue)
		if value == "|" {
			currentKey = key
			inBlock = true
			blockBuf = nil
			continue
		}
		setField(&r, key, value)
	}
	flushBlock()
	if r.ID == "" {
		return Rule{}, false
	}
	return r, true
}

func setField(r *Rule, key, value string) {
	switch key {
	case "humanizer-id":
		r.ID = value
	case "humanizer-name":
		r.Name = value
	case "humanizer-category":
		r.Category = value
	case "humanizer-summary":
		r.Summary = value
	case "humanizer-rationale":
		r.Rationale = value
	case "humanizer-before":
		r.Before = value
	case "humanizer-after":
		r.After = value
	case "humanizer-reference":
		r.Reference = value
	}
}

// PackError indicates the embedded pack is malformed — unrecoverable and
// almost always a bug in this package, not in user input.
type PackError struct{ Msg string }

func (e *PackError) Error() string { return fmt.Sprintf("rules: %s", e.Msg) }
