package semantic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knights-analytics/hugot"
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

	vecs, err := emb.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("embedder returned no vectors for query")
	}

	hits := idx.Search(vecs[0], k, f)
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

// NeedsModelDownload reports whether the embedding model is missing from modelDir.
func NeedsModelDownload(modelDir string) bool {
	subDir := strings.ReplaceAll(hugotModelName, "/", "_")
	_, err := os.Stat(filepath.Join(modelDir, subDir))
	return os.IsNotExist(err)
}

// EnsureModel makes sure the embedding model is present under modelDir,
// downloading it from Hugging Face on first use. It is idempotent: if the model
// subdirectory already exists it is a no-op (no network call).
func EnsureModel(ctx context.Context, modelDir string) error {
	subDir := strings.ReplaceAll(hugotModelName, "/", "_")
	if _, err := os.Stat(filepath.Join(modelDir, subDir)); err == nil {
		return nil // already downloaded
	}
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return fmt.Errorf("create model dir %s: %w", modelDir, err)
	}

	opts := hugot.NewDownloadOptions()
	opts.OnnxFilePath = hugotOnnxFilename
	if _, err := hugot.DownloadModel(ctx, hugotModelName, modelDir, opts); err != nil {
		return fmt.Errorf("download embedding model %s: %w", hugotModelName, err)
	}
	return nil
}
