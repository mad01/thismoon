package hybrid

// ResultHit is one post-fusion (and optionally post-rerank) hybrid search
// result, the JSON shape the CLI and brain_hybrid_search share.
type ResultHit struct {
	Path        string   `json:"path"`
	Title       string   `json:"title,omitempty"`
	HeadingPath string   `json:"heading_path,omitempty"`
	Score       float64  `json:"score"                  jsonschema:"post-fusion (and post-rerank when enabled) score; higher is better"`
	Sources     []string `json:"sources"                jsonschema:"which retrievers found this hit: zoekt, vector, or both"`
	Snippet     string   `json:"snippet,omitempty"`
	Tier        string   `json:"tier,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Type        string   `json:"type,omitempty"`
	Status      string   `json:"status,omitempty"`
	Evidence    string   `json:"evidence,omitempty"`
}

// Result is the JSON result of a hybrid search.
type Result struct {
	Hits  []ResultHit `json:"hits"`
	Total int         `json:"total"`
}

// NewResult wraps hits with their count; nil hits become an empty list.
func NewResult(hits []ResultHit) Result {
	if hits == nil {
		hits = []ResultHit{}
	}
	return Result{Hits: hits, Total: len(hits)}
}
