package semantic

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
	Path        string   `json:"path"                   jsonschema:"vault-relative note path"`
	Title       string   `json:"title,omitempty"        jsonschema:"note title (frontmatter or filename stem)"`
	HeadingPath string   `json:"heading_path,omitempty" jsonschema:"' › '-joined heading hierarchy above this chunk"`
	Score       float32  `json:"score"                  jsonschema:"cosine similarity in [-1,1]; higher is better"`
	Snippet     string   `json:"snippet,omitempty"      jsonschema:"chunk body excerpt; only set in snippets mode"`
	Tier        string   `json:"tier,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Type        string   `json:"type,omitempty"`
	Status      string   `json:"status,omitempty"`
	Evidence    string   `json:"evidence,omitempty"     jsonschema:"trust tier of the source note (A/B/C/D/mixed)"`
}

// SearchResult is the JSON result of a semantic search.
type SearchResult struct {
	OutputMode string      `json:"output_mode"`
	Hits       []ResultHit `json:"hits"`
	Total      int         `json:"total"`
}

// NewSearchResult shapes ranked hits for output in mode. ModePaths keeps the
// first (best) hit per note and drops heading path and snippet.
func NewSearchResult(mode string, hits []Hit) SearchResult {
	out := SearchResult{OutputMode: mode}
	seen := make(map[string]bool)
	for _, h := range hits {
		r := ResultHit{
			Path:     h.Path,
			Title:    h.Title,
			Score:    h.Score,
			Tier:     h.Tier,
			Tags:     h.Tags,
			Type:     h.Type,
			Status:   h.Status,
			Evidence: h.Evidence,
		}
		if mode == ModePaths {
			if seen[h.Path] {
				continue
			}
			seen[h.Path] = true
		} else {
			r.HeadingPath = h.HeadingPath
			r.Snippet = h.Snippet
		}
		out.Hits = append(out.Hits, r)
	}
	out.Total = len(out.Hits)
	return out
}
