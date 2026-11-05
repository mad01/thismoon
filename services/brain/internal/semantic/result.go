package semantic

import "sort"

// Output modes for NewSearchResult.
const (
	// ModeSnippets returns one hit per chunk, with heading path and snippet.
	ModeSnippets = "snippets"
	// ModePaths returns each note once, without heading path or snippet.
	ModePaths = "paths"
)

// ResultHit is one semantic search hit, the JSON shape the CLI and
// brain_semantic_search share.
type ResultHit struct {
	Vault       string   `json:"vault"                  jsonschema:"name of the vault holding the note"`
	Path        string   `json:"path"                   jsonschema:"vault-relative note path"`
	ID          string   `json:"id,omitempty"           jsonschema:"the note's id, when it has one"`
	Title       string   `json:"title,omitempty"        jsonschema:"note title (frontmatter or filename stem)"`
	HeadingPath string   `json:"heading_path,omitempty" jsonschema:"' › '-joined heading hierarchy above this chunk"`
	Score       float32  `json:"score"                  jsonschema:"cosine similarity in [-1,1]; higher is better"`
	Snippet     string   `json:"snippet,omitempty"      jsonschema:"chunk body excerpt; only set in snippets mode"`
	Tier        string   `json:"tier,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Type        string   `json:"type,omitempty"`
	Status      string   `json:"status,omitempty"       jsonschema:"the note's lifecycle status as stored, read from the note now"`
	Evidence    string   `json:"evidence,omitempty"     jsonschema:"trust tier of the source note (A/B/C/D/mixed)"`
	ReviewBy    string   `json:"review_by,omitempty"    jsonschema:"the date the note is due for review, when set"`
	Stale       bool     `json:"stale,omitempty"        jsonschema:"true when review_by has passed"`
}

// SearchResult is the JSON result of a semantic search.
type SearchResult struct {
	OutputMode string      `json:"output_mode"`
	Hits       []ResultHit `json:"hits"`
	Total      int         `json:"total"`
	Warnings   []string    `json:"warnings,omitempty" jsonschema:"why part of the index may be stale, e.g. Ollama was unreachable for a refresh"`
}

// NewSearchResult shapes ranked hits for output in mode. ModePaths keeps the
// first (best) hit per note and drops heading path and snippet.
//
// Hits from several vaults are merged by MergeHits first.
func NewSearchResult(mode string, hits []Hit) SearchResult {
	out := SearchResult{OutputMode: mode}
	seen := make(map[string]bool)
	for _, h := range hits {
		r := ResultHit{
			Vault:    h.Vault,
			Path:     h.Path,
			ID:       h.NoteID,
			Title:    h.Title,
			Score:    h.Score,
			Tier:     h.Tier,
			Tags:     h.Tags,
			Type:     h.Type,
			Status:   h.Status,
			Evidence: h.Evidence,
			ReviewBy: h.ReviewBy,
			Stale:    h.Stale,
		}
		if mode == ModePaths {
			key := h.Vault + ":" + h.Path
			if seen[key] {
				continue
			}
			seen[key] = true
		} else {
			r.HeadingPath = h.HeadingPath
			r.Snippet = h.Snippet
		}
		out.Hits = append(out.Hits, r)
	}
	out.Total = len(out.Hits)
	return out
}

// MergeHits merges the ranked hits of several vaults into one list of at
// most k, by score, best first; a tie keeps the order of lists.
func MergeHits(lists [][]Hit, k int) []Hit {
	var out []Hit
	for _, l := range lists {
		out = append(out, l...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if k > 0 && len(out) > k {
		out = out[:k]
	}
	return out
}
