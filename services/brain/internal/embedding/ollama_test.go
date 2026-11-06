package embedding

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbed_RoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Model != "test-model" {
			t.Errorf("model = %q, want test-model", req.Model)
		}
		if req.Prompt == "" {
			t.Error("prompt should not be empty")
		}
		_ = json.NewEncoder(w).Encode(embedResponse{Embedding: []float32{3, 4}})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-model")
	v, err := c.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	// Input was {3,4} → norm=5 → expected {0.6, 0.8}.
	if len(v) != 2 {
		t.Fatalf("dim = %d, want 2", len(v))
	}
	if math.Abs(float64(v[0])-0.6) > 1e-5 || math.Abs(float64(v[1])-0.8) > 1e-5 {
		t.Errorf("unnormalized output: %v", v)
	}
}

func TestEmbed_EmptyText(t *testing.T) {
	c := New("http://localhost:0", "x")
	if _, err := c.Embed(context.Background(), "  "); err == nil {
		t.Fatal("want error for empty text")
	}
}

func TestEmbed_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()
	c := New(srv.URL, "missing")
	_, err := c.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("err missing status: %v", err)
	}
}

func TestEmbed_EmptyEmbedding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(embedResponse{Embedding: nil})
	}))
	defer srv.Close()
	c := New(srv.URL, "x")
	_, err := c.Embed(context.Background(), "hello")
	if err == nil || !strings.Contains(err.Error(), "empty embedding") {
		t.Fatalf("want empty-embedding error, got %v", err)
	}
}

func TestHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := New(srv.URL, "x")
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   []float32
		want []float32
	}{
		{"unit", []float32{1, 0, 0}, []float32{1, 0, 0}},
		{"zero", []float32{0, 0}, []float32{0, 0}},
		{"3-4-5", []float32{3, 4}, []float32{0.6, 0.8}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Normalize(tc.in)
			for i, g := range got {
				if math.Abs(float64(g)-float64(tc.want[i])) > 1e-5 {
					t.Errorf("[%d] got %f, want %f", i, g, tc.want[i])
				}
			}
		})
	}
}
