// Package semantic provides text embedding and source-code chunking for
// semantic (vector) search over local repositories.
package semantic

import "context"

// Embedder turns text into fixed-dimension float vectors.
//
// Implementations may call out to a local model (HugotEmbedder) or a remote
// service (OllamaEmbedder). Embed must return one vector per input text, in
// the same order, each of length Dim().
type Embedder interface {
	// Embed returns one embedding vector per input text.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Dim returns the dimensionality of every vector Embed produces.
	Dim() int
}
