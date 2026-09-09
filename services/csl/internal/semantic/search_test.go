package semantic

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// plantStore writes a one-chunk store for repo into indexDir, with the chunk's
// source line living in a real file under srcDir so ExpandHit can read it back.
func plantStore(
	t *testing.T,
	indexDir string,
	repo finder.Repo,
	emb Embedder,
	rel, embedText, line string,
) {
	t.Helper()
	srcPath := filepath.Join(repo.Path, rel)
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	vecs, err := emb.Embed(context.Background(), []string{embedText})
	if err != nil {
		t.Fatalf("embed chunk: %v", err)
	}
	chunks := []Chunk{{
		Repo:      repo.Name,
		Path:      rel,
		Lang:      "go",
		Kind:      "func",
		StartLine: 1,
		EndLine:   1,
		EmbedText: embedText,
	}}

	store := NewStore(emb.Dim())
	store.SetRepoPath(repo.Path)
	store.PutFile(rel, "hash-"+rel, chunks, vecs)
	if err := store.Save(StorePathForRepo(indexDir, repo)); err != nil {
		t.Fatalf("save store: %v", err)
	}
}

func TestSearchInProcess(t *testing.T) {
	indexDir := t.TempDir()
	emb := newFakeEmbedder(16)

	const query = "the special query string"

	repoA := finder.Repo{Name: "org/match", Path: t.TempDir()}
	repoB := finder.Repo{Name: "org/other", Path: t.TempDir()}

	// repoA's chunk text equals the query → identical (normalized) vector →
	// cosine 1.0, so it must rank above repoB's unrelated chunk.
	plantStore(t, indexDir, repoA, emb, "main.go", query, "func matchedHere() {}")
	plantStore(
		t,
		indexDir,
		repoB,
		emb,
		"util.go",
		"completely unrelated content",
		"func somethingElse() {}",
	)

	results, err := SearchInProcess(context.Background(), indexDir, emb, query, 10, Filter{}, 0)
	if err != nil {
		t.Fatalf("SearchInProcess: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	top := results[0]
	if top.Repo != "org/match" || top.Path != "main.go" {
		t.Fatalf("top hit = %s/%s, want org/match/main.go", top.Repo, top.Path)
	}
	if top.Score < results[1].Score {
		t.Fatalf("ranking wrong: top score %f < next %f", top.Score, results[1].Score)
	}
	if !strings.Contains(top.Snippet, "matchedHere") {
		t.Fatalf("top snippet = %q, want it to contain the source line", top.Snippet)
	}
}

func TestSearchInProcessEmptyIndex(t *testing.T) {
	indexDir := t.TempDir() // no stores planted
	emb := newFakeEmbedder(8)

	results, err := SearchInProcess(context.Background(), indexDir, emb, "anything", 5, Filter{}, 0)
	if err != nil {
		t.Fatalf("SearchInProcess on empty index: %v", err)
	}
	if results != nil {
		t.Fatalf("want nil results for empty index, got %v", results)
	}
}

func TestSearchInProcessRepoFilter(t *testing.T) {
	indexDir := t.TempDir()
	emb := newFakeEmbedder(16)

	repoA := finder.Repo{Name: "org/match", Path: t.TempDir()}
	repoB := finder.Repo{Name: "org/other", Path: t.TempDir()}
	plantStore(t, indexDir, repoA, emb, "main.go", "alpha text", "func a() {}")
	plantStore(t, indexDir, repoB, emb, "util.go", "beta text", "func b() {}")

	results, err := SearchInProcess(
		context.Background(),
		indexDir,
		emb,
		"alpha text",
		10,
		Filter{Repos: []string{"match"}},
		0,
	)
	if err != nil {
		t.Fatalf("SearchInProcess: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (filtered to org/match)", len(results))
	}
	if results[0].Repo != "org/match" {
		t.Fatalf("filtered hit = %s, want org/match", results[0].Repo)
	}
}
