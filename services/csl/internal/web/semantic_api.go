package web

import (
	"net/http"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// semanticHitJSON is one hit in the /api/semantic_search payload. FileURL links
// the hit to its source on the git host, like the lexical results do.
type semanticHitJSON struct {
	Repo      string  `json:"repo"`
	Path      string  `json:"path"`
	Lang      string  `json:"lang,omitempty"`
	Kind      string  `json:"kind,omitempty"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Score     float32 `json:"score"`
	Snippet   string  `json:"snippet,omitempty"`
	FileURL   string  `json:"fileURL,omitempty"`
}

// semanticResponse is the /api/semantic_search payload. When Available is false,
// Note carries a build hint and Hits is empty.
type semanticResponse struct {
	Available bool              `json:"available"`
	Note      string            `json:"note,omitempty"`
	Query     string            `json:"query"`
	Hits      []semanticHitJSON `json:"hits"`
}

func (s *Server) handleSemanticSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "missing required query parameter 'q'")
		return
	}
	if len(query) > maxQueryLength {
		writeError(w, http.StatusBadRequest, "query too long")
		return
	}

	req := SemanticRequest{
		Query:  query,
		K:      clampInt(q.Get("k"), defaultSemanticK, 1, maxLimit),
		Repo:   q.Get("repo"),
		Lang:   q.Get("lang"),
		Expand: clampInt(q.Get("expand"), 0, 0, maxContext),
	}

	res, err := s.svc.SemanticSearch(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, s.semanticResponseFrom(query, res))
}

// semanticResponseFrom maps the Service result to JSON, enriching each hit with
// a remote source URL (best-effort: missing repo metadata just omits the link).
func (s *Server) semanticResponseFrom(query string, res SemanticResult) semanticResponse {
	repoMap := s.repoMetaByName()
	hits := make([]semanticHitJSON, len(res.Hits))
	for i, h := range res.Hits {
		hits[i] = semanticHitJSON{
			Repo:      h.Repo,
			Path:      h.Path,
			Lang:      h.Lang,
			Kind:      h.Kind,
			StartLine: h.StartLine,
			EndLine:   h.EndLine,
			Score:     h.Score,
			Snippet:   h.Snippet,
			FileURL:   finder.FileURL(repoMap[h.Repo], h.Path, h.StartLine),
		}
	}
	return semanticResponse{
		Available: res.Available,
		Note:      res.Note,
		Query:     query,
		Hits:      hits,
	}
}

// repoMetaByName indexes the discovered repos by name for remote-URL building.
// A discovery error is non-fatal here: hits simply render without a source link.
func (s *Server) repoMetaByName() map[string]finder.Repo {
	repos, err := s.svc.Repos()
	if err != nil {
		return nil
	}
	m := make(map[string]finder.Repo, len(repos))
	for _, rp := range repos {
		m[rp.Name] = rp
	}
	return m
}
