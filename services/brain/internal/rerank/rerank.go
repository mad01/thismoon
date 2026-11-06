// Package rerank scores fused hybrid-search candidates with a small local
// LLM (via Ollama /api/chat) and blends the result with the retrieval
// position. The reranker is opt-in per call: it adds 1–5s of latency on
// M-series Macs and is rarely worth the cost for short, exact-keyword
// queries.
//
// The blending schedule mirrors QMD's "position-aware" recipe:
//   - ranks 1–3   : keep the retriever ordering (rerank ignored)
//   - ranks 4–10  : 60% retriever + 40% rerank
//   - ranks 11+   : 40% retriever + 60% rerank (trust the LLM more for
//     the long tail).
//
// The package never panics on a malformed LLM reply: a non-numeric or
// out-of-range response degrades to score 0.5 (neutral), so a flaky
// model can't poison the ordering.
package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Reranker calls Ollama /api/chat to score candidates against a query.
type Reranker struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

// New returns a Reranker with sane HTTP defaults. The chat models we use
// (llama3.2:3b, qwen2.5:3b) typically reply in <1s per candidate on
// M-series, but we cap individual calls at 30s to survive cold starts.
func New(baseURL, model string) *Reranker {
	return &Reranker{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Candidate is one item to rerank. Score is the retrieval-side score
// (typically the RRF fused score). Snippet is the chunk text the model
// will read.
type Candidate struct {
	ID            string
	Snippet       string
	RetrievalRank int     // 1-based rank from the fused list
	Score         float64 // retrieval-side score
}

// Scored is the reranker's output for one candidate.
type Scored struct {
	ID         string
	Final      float64 // blended score (higher = better)
	Retrieval  float64 // input retrieval score, unchanged
	LLM        float64 // raw 0..1 from the model (0.5 if unparseable)
	Reasonable bool    // false if the model reply could not be parsed
}

// Score blends each candidate's retrieval score with an LLM relevance
// rating. Returns scored items sorted descending by Final. The slice
// length matches the input.
//
// Top-3 candidates are passed through unchanged (LLM=0, Final=Score) per
// the QMD rule.
func (r *Reranker) Score(ctx context.Context, query string, cands []Candidate) ([]Scored, error) {
	out := make([]Scored, len(cands))
	for i, c := range cands {
		out[i] = Scored{
			ID:        c.ID,
			Retrieval: c.Score,
			Final:     c.Score,
			LLM:       0.5,
		}
	}
	if len(cands) == 0 {
		return out, nil
	}

	for i, c := range cands {
		// Top-3 retrieval winners: trust the retriever; skip the LLM call
		// to save latency.
		if c.RetrievalRank > 0 && c.RetrievalRank <= 3 {
			continue
		}
		llm, ok, err := r.scoreOne(ctx, query, c.Snippet)
		if err != nil {
			return nil, err
		}
		out[i].LLM = llm
		out[i].Reasonable = ok
		out[i].Final = blend(c.RetrievalRank, c.Score, llm)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Final > out[j].Final
	})
	return out, nil
}

// scoreOne calls Ollama and parses the numeric reply. Returns score, ok,
// err. ok=false means the reply was unparseable; score is 0.5 in that
// case so callers don't see NaN.
func (r *Reranker) scoreOne(ctx context.Context, query, snippet string) (float64, bool, error) {
	prompt := fmt.Sprintf(
		"Rate the relevance of the passage to the query on a scale from 0.0 (irrelevant) "+
			"to 1.0 (highly relevant). Respond with only the number.\n\n"+
			"QUERY: %s\n\nPASSAGE:\n%s",
		query, truncate(snippet, 1500),
	)

	body, err := json.Marshal(chatRequest{
		Model: r.Model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
		Options: chatOptions{
			Temperature: 0,
			NumPredict:  16,
		},
	})
	if err != nil {
		return 0.5, false, fmt.Errorf("rerank: marshal: %w", err)
	}
	endpoint, err := url.JoinPath(r.BaseURL, "/api/chat")
	if err != nil {
		return 0.5, false, fmt.Errorf("rerank: url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0.5, false, fmt.Errorf("rerank: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.HTTP.Do(req)
	if err != nil {
		return 0.5, false, fmt.Errorf("rerank: ollama unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var msg bytes.Buffer
		_, _ = msg.ReadFrom(resp.Body)
		return 0.5, false, fmt.Errorf(
			"rerank: ollama %s: %s",
			resp.Status,
			strings.TrimSpace(msg.String()),
		)
	}

	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0.5, false, fmt.Errorf("rerank: decode: %w", err)
	}
	score, ok := parseScore(out.Message.Content)
	return score, ok, nil
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Options  chatOptions   `json:"options,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
}

// parseScore extracts the first decimal number in [0, 1] from text.
// Returns (score, true) on success, (0.5, false) when no valid number
// is found — neutral fallback so a malformed reply doesn't tank ranking.
func parseScore(text string) (float64, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0.5, false
	}
	// Walk through the text picking up the first numeric run. Acceptable:
	// "0.7", "0.85", "1", ".42".
	for i := 0; i < len(text); i++ {
		c := text[i]
		if (c >= '0' && c <= '9') || c == '.' {
			j := i
			for j < len(text) && ((text[j] >= '0' && text[j] <= '9') || text[j] == '.') {
				j++
			}
			n, err := strconv.ParseFloat(text[i:j], 64)
			if err != nil {
				return 0.5, false
			}
			if n < 0 {
				n = 0
			}
			if n > 1 {
				n = 1
			}
			return n, true
		}
	}
	return 0.5, false
}

// blend combines retrieval score (any positive scale) and LLM score (0..1)
// using the position-aware weights described in the package doc.
//
// The retrieval score is normalized to a 0..1 range by passing it through
// a small saturating function — RRF scores are typically <= 0.07, so a
// linear scale would make every item land in the low end of the blend.
// We use min(1, score * 10) which is empirically reasonable for k=60 RRF.
func blend(rank int, retrieval, llm float64) float64 {
	rNorm := retrieval * 10
	if rNorm > 1 {
		rNorm = 1
	}
	if rNorm < 0 {
		rNorm = 0
	}
	if rank <= 3 && rank > 0 {
		return retrieval // unchanged
	}
	wRet, wLLM := 0.6, 0.4
	if rank >= 11 || rank == 0 {
		wRet, wLLM = 0.4, 0.6
	}
	// Final keeps the original retrieval magnitude order (so adjacent
	// items with similar retrieval scores stay near each other) but
	// shifts by the rerank delta.
	return wRet*rNorm + wLLM*llm
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
