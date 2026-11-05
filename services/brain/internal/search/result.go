package search

import (
	"errors"
	"sort"
)

// ErrInvalidQuery means a zoekt query failed to parse.
var ErrInvalidQuery = errors.New("search: invalid query")

// Output modes for Search and NewResult.
const (
	// ModeFiles returns each matching note once.
	ModeFiles = "files_with_matches"
	// ModeContent returns every matching line.
	ModeContent = "content"
)

// FileHit is one matching note in files_with_matches mode.
type FileHit struct {
	Vault    string `json:"vault"               jsonschema:"name of the vault holding the note"`
	Path     string `json:"path"                jsonschema:"vault-relative note path, e.g. notes/topic/foo.md"`
	Evidence string `json:"evidence,omitempty"  jsonschema:"trust tier of the matching note (A/B/C/D/mixed) if set"`
	Status   string `json:"status,omitempty"    jsonschema:"the note's lifecycle status as stored (raw, validated, disputed, ...)"`
	ReviewBy string `json:"review_by,omitempty" jsonschema:"the date the note is due for review, when set"`
	Stale    bool   `json:"stale,omitempty"     jsonschema:"true when review_by has passed: re-check the note before relying on it"`
}

// LineHit is one matching line in content mode.
type LineHit struct {
	Vault    string `json:"vault"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Text     string `json:"text"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
	Evidence string `json:"evidence,omitempty"  jsonschema:"trust tier of the matching note (A/B/C/D/mixed) if set"`
	Status   string `json:"status,omitempty"    jsonschema:"the note's lifecycle status as stored"`
	ReviewBy string `json:"review_by,omitempty" jsonschema:"the date the note is due for review, when set"`
	Stale    bool   `json:"stale,omitempty"     jsonschema:"true when review_by has passed"`
}

// Result is the JSON result of a search, shared by the CLI and brain_search.
type Result struct {
	OutputMode string    `json:"output_mode"     jsonschema:"echoes the output mode actually used"`
	Files      []FileHit `json:"files,omitempty" jsonschema:"unique file paths that matched; set when output_mode is files_with_matches"`
	Lines      []LineHit `json:"lines,omitempty" jsonschema:"matching lines; set when output_mode is content"`
	Total      int       `json:"total"           jsonschema:"total number of match records returned"`
}

// Counts is the JSON result of a count, shared by the CLI and brain_count.
type Counts struct {
	Total  int           `json:"total"            jsonschema:"total match count"`
	Groups []CountResult `json:"groups,omitempty" jsonschema:"per-group counts; empty when group_by is not set"`
}

// NewResult shapes matches for output in mode (ModeFiles or ModeContent),
// annotating each hit with its note's evidence tier, status, review_by and
// staleness, which meta reads from disk.
func NewResult(meta *MetaReader, mode string, matches []Match) Result {
	out := Result{OutputMode: mode}
	if len(matches) == 0 {
		return out
	}
	if mode == ModeContent {
		out.Lines = make([]LineHit, 0, len(matches))
		for _, m := range matches {
			n := meta.Meta(m.Vault, m.Path)
			out.Lines = append(out.Lines, LineHit{
				Vault:    m.Vault,
				Path:     m.Path,
				Line:     m.Line,
				Column:   m.Column,
				Text:     m.Text,
				Before:   m.Before,
				After:    m.After,
				Evidence: n.Evidence,
				Status:   n.Status,
				ReviewBy: n.ReviewBy,
				Stale:    n.Stale,
			})
		}
		out.Total = len(out.Lines)
		return out
	}
	seen := make(map[string]bool, len(matches))
	out.Files = make([]FileHit, 0, len(matches))
	for _, m := range matches {
		key := m.Vault + ":" + m.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		n := meta.Meta(m.Vault, m.Path)
		out.Files = append(out.Files, FileHit{
			Vault:    m.Vault,
			Path:     m.Path,
			Evidence: n.Evidence,
			Status:   n.Status,
			ReviewBy: n.ReviewBy,
			Stale:    n.Stale,
		})
	}
	out.Total = len(out.Files)
	return out
}

// MergeMatches merges the matches of several vaults, each list in its own
// rank order, into one list of at most limit notes. Notes are ordered by
// zoekt score, best first; a tie keeps the order of lists, then of matches.
// A note's matching lines stay together.
func MergeMatches(lists [][]Match, limit int) []Match {
	type note struct {
		score   float64
		matches []Match
	}
	var notes []*note
	for _, list := range lists {
		byPath := make(map[string]*note)
		for _, m := range list {
			n, ok := byPath[m.Path]
			if !ok {
				n = &note{score: m.Score}
				byPath[m.Path] = n
				notes = append(notes, n)
			}
			n.matches = append(n.matches, m)
		}
	}
	sort.SliceStable(notes, func(i, j int) bool { return notes[i].score > notes[j].score })
	if limit > 0 && len(notes) > limit {
		notes = notes[:limit]
	}
	var out []Match
	for _, n := range notes {
		out = append(out, n.matches...)
	}
	return out
}

// MergeCounts adds per-group counts from several indexes into one list,
// largest count first, then by group name.
func MergeCounts(lists ...[]CountResult) []CountResult {
	sums := make(map[string]int)
	for _, list := range lists {
		for _, c := range list {
			sums[c.Group] += c.Count
		}
	}
	out := make([]CountResult, 0, len(sums))
	for group, count := range sums {
		out = append(out, CountResult{Group: group, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Group < out[j].Group
	})
	return out
}
