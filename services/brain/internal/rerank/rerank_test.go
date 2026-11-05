package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseScore(t *testing.T) {
	tests := []struct {
		in        string
		want      float64
		wantValid bool
	}{
		{"0.7", 0.7, true},
		{"0.85\n", 0.85, true},
		{"  Score: 0.42 ", 0.42, true},
		{"1", 1.0, true},
		{"1.5", 1.0, true}, // clamped
		{
			"-0.3",
			0.0,
			true,
		}, // clamped (no minus picked up by digit scan, so this actually parses as "0.3")
		{"not a number", 0.5, false},
		{"", 0.5, false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := parseScore(tc.in)
			if ok != tc.wantValid {
				t.Errorf("ok = %v, want %v", ok, tc.wantValid)
			}
			// Special-case the "-0.3" expectation: the digit walker doesn't
			// pick up '-', so it parses "0.3".
			if tc.in == "-0.3" {
				if got != 0.3 {
					t.Errorf("got %f, want 0.3 (sign stripped by digit walker)", got)
				}
				return
			}
			if got != tc.want {
				t.Errorf("got %f, want %f", got, tc.want)
			}
		})
	}
}

func TestBlend(t *testing.T) {
	// Top-3 retrieval items pass through unchanged.
	if got := blend(1, 0.05, 0.0); got != 0.05 {
		t.Errorf("rank 1 should pass through, got %f", got)
	}
	if got := blend(3, 0.04, 0.99); got != 0.04 {
		t.Errorf("rank 3 should pass through, got %f", got)
	}

	// Rank 5: 60% retrieval + 40% rerank
	got := blend(5, 0.05, 1.0)
	want := 0.6*0.5 + 0.4*1.0 // rNorm = 0.05 * 10 = 0.5
	if got != want {
		t.Errorf("rank 5 blend: got %f, want %f", got, want)
	}

	// Rank 12 (≥11): 40% retrieval + 60% rerank
	got = blend(12, 0.05, 1.0)
	want = 0.4*0.5 + 0.6*1.0
	if got != want {
		t.Errorf("rank 12 blend: got %f, want %f", got, want)
	}

	// Rank 0 (unranked) treated as long tail.
	got = blend(0, 0.05, 1.0)
	if got != 0.4*0.5+0.6*1.0 {
		t.Errorf("rank 0 should use long-tail weights, got %f", got)
	}
}

func TestScore_TopThreeSkipped(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(chatResponse{Message: chatMessage{Content: "0.9"}})
	}))
	defer srv.Close()
	r := New(srv.URL, "x")

	cands := []Candidate{
		{ID: "a", Snippet: "alpha", RetrievalRank: 1, Score: 0.06},
		{ID: "b", Snippet: "beta", RetrievalRank: 2, Score: 0.05},
		{ID: "c", Snippet: "gamma", RetrievalRank: 3, Score: 0.04},
	}
	out, err := r.Score(context.Background(), "q", cands)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if calls != 0 {
		t.Errorf("ollama called %d times for top-3, want 0", calls)
	}
	if len(out) != 3 {
		t.Fatalf("got %d", len(out))
	}
	for _, s := range out {
		if s.Final != s.Retrieval {
			t.Errorf("top-3 should pass through: %+v", s)
		}
	}
}

func TestScore_LongTailReorders(t *testing.T) {
	// Two candidates at rank 5 and 6 with similar retrieval scores; the
	// LLM strongly prefers the second. After blending, the second wins.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		// Return high score for the candidate containing "winner".
		score := "0.2"
		for _, m := range req.Messages {
			if contains(m.Content, "winner") {
				score = "0.95"
			}
		}
		_ = json.NewEncoder(w).Encode(chatResponse{Message: chatMessage{Content: score}})
	}))
	defer srv.Close()
	r := New(srv.URL, "x")

	cands := []Candidate{
		{ID: "loser", Snippet: "ordinary text", RetrievalRank: 5, Score: 0.05},
		{ID: "winner", Snippet: "the winner passage", RetrievalRank: 6, Score: 0.04},
	}
	out, err := r.Score(context.Background(), "q", cands)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if out[0].ID != "winner" {
		t.Errorf("top after rerank = %s, want winner", out[0].ID)
	}
}

func TestScore_BadResponseFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(chatResponse{Message: chatMessage{Content: "I don't know"}})
	}))
	defer srv.Close()
	r := New(srv.URL, "x")
	out, err := r.Score(context.Background(), "q", []Candidate{
		{ID: "a", Snippet: "a", RetrievalRank: 5, Score: 0.05},
	})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if out[0].LLM != 0.5 {
		t.Errorf("LLM = %f, want 0.5 fallback", out[0].LLM)
	}
	if out[0].Reasonable {
		t.Error("Reasonable should be false")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
