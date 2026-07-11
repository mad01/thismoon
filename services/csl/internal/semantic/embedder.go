// Package semantic provides text embedding and source-code chunking for
// semantic (vector) search over local repositories.
package semantic

import "context"

// Embedder turns text into fixed-dimension float vectors.
//
// Embed is the document path (bare chunk text); EmbedQuery is the search-query
// path, letting instruction-tuned models prefix the retrieval task on the
// query side only. Embed must return one vector per input text, in the same
// order, each of length Dim().
type Embedder interface {
	// Embed returns one embedding vector per input text.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// EmbedQuery returns the embedding vector for a single search query.
	EmbedQuery(ctx context.Context, query string) ([]float32, error)
	// Dim returns the dimensionality of every vector Embed produces.
	Dim() int
}
