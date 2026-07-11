package semantic

import (
	"os"
	"path/filepath"
	"testing"
)

// savedStore builds a one-file store for repoName at repoPath and writes it to
// indexDir as <safeName>.gob, returning the chunks used.
func savedStore(t *testing.T, emb Embedder, indexDir, repoName, repoPath string, chunks []Chunk) {
	t.Helper()
	s := NewStore(emb.Dim())
	s.SetRepoPath(repoPath)
	s.PutFile(chunks[0].Path, "hash", chunks, embedAll(t, emb, chunks))
	name := filepath.Base(repoName) + ".gob"
	if err := s.Save(filepath.Join(indexDir, name)); err != nil {
		t.Fatalf("save store %s: %v", repoName, err)
	}
}

func TestOpenIndexMissingDir(t *testing.T) {
	ix, err := OpenIndex(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("OpenIndex missing dir: %v", err)
	}
	if ix.Stores() != 0 || ix.Len() != 0 {
		t.Fatalf("empty index expected, got stores=%d len=%d", ix.Stores(), ix.Len())
	}
}

func TestOpenIndexMergesAndRanks(t *testing.T) {
	emb := newFakeEmbedder(8)
	indexDir := t.TempDir()

	aChunks := []Chunk{
		mkChunk("org/alpha", "a.go", "go", "function_declaration", 1, 10, "alpha go func"),
		mkChunk("org/alpha", "a.go", "go", "function_declaration", 12, 20, "beta go func"),
	}
	bChunks := []Chunk{
		mkChunk("org/bravo", "b.py", "python", "function_definition", 1, 5, "gamma py def"),
	}
	savedStore(t, emb, indexDir, "org_alpha", "/repos/alpha", aChunks)
	savedStore(t, emb, indexDir, "org_bravo", "/repos/bravo", bChunks)

	// A non-store file under indexDir must be ignored.
	if err := os.WriteFile(filepath.Join(indexDir, "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write state.json: %v", err)
	}

	ix, err := OpenIndex(indexDir)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	if ix.Stores() != 2 {
		t.Fatalf("Stores() = %d, want 2", ix.Stores())
	}
	if ix.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", ix.Len())
	}

	// Query equal to bravo's only chunk: its hit must rank first, across stores.
	q := embedAll(t, emb, bChunks)[0]
	hits := ix.Search(q, 10, Filter{})
	if len(hits) != 3 {
		t.Fatalf("got %d hits, want 3", len(hits))
	}
	top := hits[0]
	if top.Repo != "org/bravo" || top.RepoPath != "/repos/bravo" || top.Path != "b.py" {
		t.Fatalf("top hit = %+v, want org/bravo /repos/bravo b.py", top)
	}

	// Global top-k truncates across the merged set.
	if got := len(ix.Search(q, 1, Filter{})); got != 1 {
		t.Fatalf("k=1 returned %d hits, want 1", got)
	}

	// Filters propagate to each store.
	py := ix.Search(q, 10, Filter{Langs: []string{"python"}})
	if len(py) != 1 || py[0].Repo != "org/bravo" {
		t.Fatalf("lang filter = %+v, want one org/bravo hit", py)
	}
}

func TestExpandHit(t *testing.T) {
	dir := t.TempDir()
	content := "L1\nL2\nL3\nL4\nL5\n"
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tests := []struct {
		name  string
		start int
		end   int
		pad   int
		want  string
	}{
		{"exact range no pad", 2, 3, 0, "L2\nL3"},
		{"pad widens", 2, 3, 1, "L1\nL2\nL3\nL4"},
		{"clamp at top", 1, 2, 5, "L1\nL2\nL3\nL4\nL5"},
		{"clamp at bottom", 4, 5, 5, "L1\nL2\nL3\nL4\nL5"},
		{"single line", 3, 3, 0, "L3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := Hit{RepoPath: dir, Path: "f.txt", StartLine: tt.start, EndLine: tt.end}
			got, err := ExpandHit(h, tt.pad)
			if err != nil {
				t.Fatalf("ExpandHit: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ExpandHit = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExpandHitMissingFile(t *testing.T) {
	h := Hit{RepoPath: t.TempDir(), Path: "nope.txt", StartLine: 1, EndLine: 1}
	if _, err := ExpandHit(h, 0); err == nil {
		t.Fatalf("expected error for missing file")
	}
}
