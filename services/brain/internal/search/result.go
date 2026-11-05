package search

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
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
	Path     string `json:"path"               jsonschema:"vault-relative note path, e.g. notes/topic/foo.md"`
	Evidence string `json:"evidence,omitempty" jsonschema:"trust tier of the matching note (A/B/C/D/mixed) if set"`
}

// LineHit is one matching line in content mode.
type LineHit struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Text     string `json:"text"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
	Evidence string `json:"evidence,omitempty" jsonschema:"trust tier of the matching note (A/B/C/D/mixed) if set"`
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
// annotating each hit with its note's evidence tier read from vaultRoot.
func NewResult(vaultRoot, mode string, matches []Match) Result {
	out := Result{OutputMode: mode}
	if len(matches) == 0 {
		return out
	}
	evidence := make(map[string]string)
	if mode == ModeContent {
		out.Lines = make([]LineHit, 0, len(matches))
		for _, m := range matches {
			out.Lines = append(out.Lines, LineHit{
				Path:     m.Path,
				Line:     m.Line,
				Column:   m.Column,
				Text:     m.Text,
				Before:   m.Before,
				After:    m.After,
				Evidence: evidenceForPath(vaultRoot, evidence, m.Path),
			})
		}
		out.Total = len(out.Lines)
		return out
	}
	seen := make(map[string]bool, len(matches))
	out.Files = make([]FileHit, 0, len(matches))
	for _, m := range matches {
		if seen[m.Path] {
			continue
		}
		seen[m.Path] = true
		out.Files = append(out.Files, FileHit{
			Path:     m.Path,
			Evidence: evidenceForPath(vaultRoot, evidence, m.Path),
		})
	}
	out.Total = len(out.Files)
	return out
}

// evidenceForPath returns the evidence tier of a vault-relative note,
// parsing each path at most once per call. Evidence is best-effort metadata:
// a note that cannot be read or parsed has none.
func evidenceForPath(vaultRoot string, cache map[string]string, relPath string) string {
	if ev, ok := cache[relPath]; ok {
		return ev
	}
	ev := ""
	data, err := os.ReadFile(filepath.Join(vaultRoot, filepath.FromSlash(relPath)))
	if err == nil {
		if fm, _, perr := markdown.ParseFrontmatter(data); perr == nil {
			ev = markdown.EvidenceOf(fm)
		}
	}
	cache[relPath] = ev
	return ev
}
