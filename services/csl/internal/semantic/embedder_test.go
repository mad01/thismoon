package semantic

import (
	"context"
	"hash/fnv"
	"testing"
)

// fakeEmbedder is a deterministic, network-free Embedder for tests in this
// package. It hashes each input text into a fixed-dimension vector so the same
// text always yields the same embedding.
type fakeEmbedder struct {
	dim int
}

func newFakeEmbedder(dim int) *fakeEmbedder {
	if dim <= 0 {
		dim = 8
	}
	return &fakeEmbedder{dim: dim}
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = hashVector(t, f.dim)
	}
	return out, nil
}

// EmbedQuery hashes the query like any document text — the fake has no
// instruction tuning to exercise.
func (f *fakeEmbedder) EmbedQuery(_ context.Context, query string) ([]float32, error) {
	return hashVector(query, f.dim), nil
}

func (f *fakeEmbedder) Dim() int { return f.dim }

// hashVector maps text deterministically into a dim-length vector in [0,1).
func hashVector(text string, dim int) []float32 {
	v := make([]float32, dim)
	for i := 0; i < dim; i++ {
		h := fnv.New32a()
		_, _ = h.Write([]byte(text))
		_, _ = h.Write([]byte{byte(i)})
		v[i] = float32(h.Sum32()%1000) / 1000.0
	}
	return v
}

func TestFakeEmbedderSatisfiesInterface(t *testing.T) {
	var _ Embedder = newFakeEmbedder(8)
}

func TestFakeEmbedderDeterministicAndStable(t *testing.T) {
	const dim = 16
	emb := newFakeEmbedder(dim)
	ctx := context.Background()

	if got := emb.Dim(); got != dim {
		t.Fatalf("Dim() = %d, want %d", got, dim)
	}

	texts := []string{"alpha", "beta", "alpha"}
	a, err := emb.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	b, err := emb.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("Embed (second call): %v", err)
	}

	if len(a) != len(texts) {
		t.Fatalf("got %d vectors, want %d", len(a), len(texts))
	}
	for i := range a {
		if len(a[i]) != dim {
			t.Fatalf("vector %d has dim %d, want %d", i, len(a[i]), dim)
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				t.Fatalf("non-deterministic at [%d][%d]: %v vs %v", i, j, a[i][j], b[i][j])
			}
		}
	}

	// Same input text => identical vector (index 0 and 2 are both "alpha").
	for j := 0; j < dim; j++ {
		if a[0][j] != a[2][j] {
			t.Fatalf("identical input produced different vectors at dim %d", j)
		}
	}
}
