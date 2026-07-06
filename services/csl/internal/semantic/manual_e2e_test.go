package semantic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// TestManualE2E exercises the full real stack (download model, chunk, embed,
// store, search, expand) against the thismoon repo itself. Guarded so
// `go test ./...` stays hermetic; run with CSL_MANUAL_E2E=1.
func TestManualE2E(t *testing.T) {
	if os.Getenv("CSL_MANUAL_E2E") == "" {
		t.Skip("set CSL_MANUAL_E2E=1 to run the real-model e2e")
	}
	ctx := context.Background()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))

	dir := t.TempDir()
	modelDir := filepath.Join(dir, "models")
	idxDir := filepath.Join(dir, "idx")

	t.Log("ensuring model...")
	if err := EnsureModel(ctx, modelDir); err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	emb, err := NewHugotEmbedder(ctx, modelDir)
	if err != nil {
		t.Fatalf("NewHugotEmbedder: %v", err)
	}
	defer func() { _ = emb.Close() }()

	repo := finder.Repo{Name: "mad01/thismoon", Path: repoRoot}
	stats, err := IndexRepoSemantic(ctx, idxDir, repo, emb)
	if err != nil {
		t.Fatalf("IndexRepoSemantic: %v", err)
	}
	t.Logf("indexed: files scanned=%d embedded=%d chunks=%d", stats.FilesScanned, stats.FilesEmbedded, stats.ChunksEmbedded)

	for _, q := range []string{
		"compute a hash fingerprint of a repo's git state",
		"parse and validate a search query",
		"start a background daemon over a unix socket",
	} {
		res, err := SearchInProcess(ctx, idxDir, emb, q, 3, Filter{}, 0)
		if err != nil {
			t.Fatalf("SearchInProcess(%q): %v", q, err)
		}
		fmt.Printf("\n### query: %q\n", q)
		for i, r := range res {
			fmt.Printf("  %d. %s:%d-%d  score=%.3f  [%s]\n", i+1, r.Path, r.StartLine, r.EndLine, r.Score, r.Kind)
		}
		if len(res) == 0 {
			t.Errorf("no results for %q", q)
		}
	}
}
