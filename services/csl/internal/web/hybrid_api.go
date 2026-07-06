package web

import (
	"net/http"

	"github.com/mad01/thismoon/services/csl/internal/hybrid"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// defaultHybridLimit is the result count used when the request omits limit.
const defaultHybridLimit = 50

// hybridHitJSON is one fused hit in the /api/hybrid_search payload.
type hybridHitJSON struct {
	Repo     string  `json:"repo"`
	Path     string  `json:"path"`
	Score    float64 `json:"score"`
	LexRank  int     `json:"lex_rank"`
	LexLine  int     `json:"lex_line,omitempty"`
	LexText  string  `json:"lex_text,omitempty"`
	SemRank  int     `json:"sem_rank"`
	SemStart int     `json:"sem_start,omitempty"`
	SemEnd   int     `json:"sem_end,omitempty"`
	SemScore float32 `json:"sem_score,omitempty"`
	Snippet  string  `json:"snippet,omitempty"`
	FileURL  string  `json:"fileURL,omitempty"`
}

// hybridResponse is the /api/hybrid_search payload. When SemanticAvailable is
// false, Note carries a build hint and hits are lexical-only.
type hybridResponse struct {
	SemanticAvailable bool            `json:"semantic_available"`
	Note              string          `json:"note,omitempty"`
	Query             string          `json:"query"`
	Hits              []hybridHitJSON `json:"hits"`
}

func (s *Server) handleHybridSearch(w http.ResponseWriter, r *http.Request) {
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

	req := HybridRequest{
		Query:  query,
		Limit:  clampInt(q.Get("limit"), defaultHybridLimit, 1, maxLimit),
		RRFK:   clampInt(q.Get("rrf_k"), hybrid.DefaultK, 1, 1000),
		Repo:   q.Get("repo"),
		Lang:   q.Get("lang"),
		Expand: clampInt(q.Get("expand"), 0, 0, maxContext),
	}

	res, err := s.svc.HybridSearch(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, s.hybridResponseFrom(query, res))
}

// hybridResponseFrom maps the Service result to JSON, enriching each hit with
// a remote source URL. The link line is the lexical line when lex is present,
// otherwise the semantic start line.
func (s *Server) hybridResponseFrom(query string, res HybridResult) hybridResponse {
	repoMap := s.repoMetaByName()
	hits := make([]hybridHitJSON, len(res.Hits))
	for i, h := range res.Hits {
		line := h.SemStart
		if h.LexRank != 0 {
			line = h.LexLine
		}
		hits[i] = hybridHitJSON{
			Repo:     h.Repo,
			Path:     h.Path,
			Score:    h.Score,
			LexRank:  h.LexRank,
			LexLine:  h.LexLine,
			LexText:  h.LexText,
			SemRank:  h.SemRank,
			SemStart: h.SemStart,
			SemEnd:   h.SemEnd,
			SemScore: h.SemScore,
			Snippet:  h.Snippet,
			FileURL:  finder.FileURL(repoMap[h.Repo], h.Path, line),
		}
	}
	return hybridResponse{
		SemanticAvailable: res.SemanticAvailable,
		Note:              res.Note,
		Query:             query,
		Hits:              hits,
	}
}
