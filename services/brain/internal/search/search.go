// Package search provides local note search backed by zoekt indexes.
// It indexes an Obsidian-style markdown vault (single root, tiered into
// inbox/notes/outputs/MOCs) and supports the same query syntax as the
// code-search MCP server.
package search

import "github.com/mad01/thismoon/services/brain/internal/vault"

// Match represents a single search hit in a note.
type Match struct {
	// Vault is the name of the vault the note is in.
	Vault string `json:"vault"`
	// Path is the vault-relative path to the note (e.g. "notes/topic/foo.md").
	Path string `json:"path"`
	// Score is zoekt's rank of the note; higher is better. It orders hits
	// from several vaults.
	Score float64 `json:"-"`
	// Line is the 1-based line number of the match.
	Line int `json:"line"`
	// Column is the 1-based column of the match start.
	Column int `json:"column"`
	// Text is the content of the matching line.
	Text string `json:"text"`
	// Before holds context lines before the match (when requested).
	Before string `json:"before,omitempty"`
	// After holds context lines after the match (when requested).
	After string `json:"after,omitempty"`
}

// SearchOptions configures a search query against the vault.
type SearchOptions struct {
	// Pattern is the zoekt query string (regex, boolean, filters).
	Pattern string
	// File restricts results to files matching this regex (vault-relative path).
	File string
	// Tag restricts results to notes with this tag in frontmatter.
	Tag string
	// Type restricts results to notes with this frontmatter type.
	Type string
	// Status restricts results to notes with this effective status: the
	// stored status, else the default of the note's tier (vault.StatusOf).
	Status string
	// Tier restricts results to a single vault tier (inbox/notes/outputs/MOCs).
	// Empty means search all tiers.
	Tier vault.Tier
	// Lang restricts results to files of this language (e.g. "markdown").
	Lang string
	// CaseSensitive disables smart case matching.
	CaseSensitive bool
	// Limit caps the number of files returned (default 50).
	Limit int
	// ContextLines is the number of context lines around each match.
	// Only applies when OutputMode is "content".
	ContextLines int
	// OutputMode controls output verbosity: "files_with_matches" or "content".
	OutputMode string
	// IncludeRevoked keeps superseded, retracted and withdrawn notes, which
	// a search leaves out by default. A Status filter naming one of those
	// statuses keeps its notes too.
	IncludeRevoked bool
}

// Count groupings.
const (
	GroupTier  = "tier"
	GroupTag   = "tag"
	GroupVault = "vault"
)

// CountOptions configures a count query.
type CountOptions struct {
	// Pattern is the zoekt query string.
	Pattern string
	// Tier restricts to a single vault tier. Empty means all tiers.
	Tier vault.Tier
	// GroupBy groups counts by GroupTier or GroupTag within one index.
	// Empty means no grouping. GroupVault is the caller's to apply across
	// indexes.
	GroupBy string
}

// CountResult holds one bucket of a grouped count query.
type CountResult struct {
	// Group is the tier name, tag or vault, depending on GroupBy.
	Group string `json:"group" jsonschema:"the tier name, tag or vault, depending on group_by"`
	// Count is the number of matches in this group.
	Count int `json:"count"`
}

// QueryInfo holds the result of parsing and validating a query.
type QueryInfo struct {
	// Valid indicates whether the query parsed successfully.
	Valid bool `json:"valid"`
	// Parsed is the string representation of the parsed query tree.
	Parsed string `json:"parsed,omitempty" jsonschema:"string representation of the parsed query tree when valid"`
	// Error is the parse error message, if any.
	Error string `json:"error,omitempty"  jsonschema:"parse error message when not valid"`
	// Hint is a suggestion for fixing the query.
	Hint string `json:"hint,omitempty"   jsonschema:"suggestion for fixing the query when not valid"`
}
