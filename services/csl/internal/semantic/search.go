package semantic

import (
	"context"
	"fmt"
)

// Result is a semantic search Hit widened with the source snippet behind it.
type Result struct {
	Hit
	Snippet string
}

// SearchInProcess runs a semantic query against the on-disk vector index without
// the daemon: it loads every per-repo store under indexDir, embeds the query,
// ranks the top-k chunks, and expands each hit back to its source snippet. An
// empty index (no chunks) yields nil, nil — the caller surfaces "not built".
func SearchInProcess(
	ctx context.Context,
	indexDir string,
	emb Embedder,
	query string,
	k int,
	f Filter,
	expand int,
) ([]Result, error) {
	idx, err := OpenIndex(indexDir)
	if err != nil {
		return nil, fmt.Errorf("open semantic index: %w", err)
	}
	if idx.Len() == 0 {
		return nil, nil
	}

	vec, err := emb.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	hits := idx.Search(vec, k, f)
	results := make([]Result, len(hits))
	for i, h := range hits {
		snippet, expandErr := ExpandHit(h, expand) // best-effort
		if expandErr != nil {
			snippet = ""
		}
		results[i] = Result{Hit: h, Snippet: snippet}
	}
	return results, nil
}
