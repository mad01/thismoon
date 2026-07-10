package semantic

import (
	"context"
	"encoding/gob"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// mkChunk builds a Chunk whose EmbedText is the embedding source.
func mkChunk(repo, path, lang, kind string, start, end int, embedText string) Chunk {
	return Chunk{
		Repo:      repo,
		Path:      path,
		Lang:      lang,
		Kind:      kind,
		StartLine: start,
		EndLine:   end,
		Text:      embedText,
		EmbedText: embedText,
	}
}

// embedAll is a test helper that embeds every chunk's EmbedText.
func embedAll(t *testing.T, emb Embedder, chunks []Chunk) [][]float32 {
	t.Helper()
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.EmbedText
	}
	vecs, err := emb.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	return vecs
}

// planStore builds a store with two files and three chunks for reuse.
func planStore(t *testing.T, emb Embedder) (*Store, []Chunk) {
	t.Helper()
	c1 := mkChunk("org/repo", "a.go", "go", "function_declaration", 1, 10, "alpha go func")
	c2 := mkChunk("org/repo", "a.go", "go", "function_declaration", 12, 20, "beta go func")
	c3 := mkChunk("org/repo", "b.py", "python", "function_definition", 1, 5, "gamma py def")

	s := NewStore(emb.Dim())
	s.SetRepoPath("/repos/org/repo")
	aChunks := []Chunk{c1, c2}
	s.PutFile("a.go", "hash-a", aChunks, embedAll(t, emb, aChunks))
	bChunks := []Chunk{c3}
	s.PutFile("b.py", "hash-b", bChunks, embedAll(t, emb, bChunks))
	return s, []Chunk{c1, c2, c3}
}

func TestStoreSearchNearest(t *testing.T) {
	emb := newFakeEmbedder(8)
	s, chunks := planStore(t, emb)

	q := embedAll(t, emb, chunks[:1])[0] // query == c1's embedding
	hits := s.Search(q, 5, Filter{})
	if len(hits) == 0 {
		t.Fatalf("no hits")
	}
	top := hits[0]
	if top.Path != "a.go" || top.StartLine != 1 {
		t.Fatalf("nearest hit = %s:%d, want a.go:1", top.Path, top.StartLine)
	}
	if top.Repo != "org/repo" || top.RepoPath != "/repos/org/repo" {
		t.Fatalf("hit repo metadata = %q/%q, want org/repo //repos/org/repo", top.Repo, top.RepoPath)
	}
	if math.Abs(float64(top.Score)-1.0) > 1e-4 {
		t.Fatalf("top score = %v, want ~1.0", top.Score)
	}
}

func TestStoreSearchRespectsK(t *testing.T) {
	emb := newFakeEmbedder(8)
	s, chunks := planStore(t, emb)

	q := embedAll(t, emb, chunks[:1])[0]
	if got := len(s.Search(q, 1, Filter{})); got != 1 {
		t.Fatalf("k=1 returned %d hits, want 1", got)
	}
	if got := len(s.Search(q, 10, Filter{})); got != 3 {
		t.Fatalf("k=10 returned %d hits, want 3", got)
	}
}

func TestStoreFilterByLang(t *testing.T) {
	emb := newFakeEmbedder(8)
	s, chunks := planStore(t, emb)

	q := embedAll(t, emb, chunks[:1])[0]
	hits := s.Search(q, 10, Filter{Langs: []string{"python"}})
	if len(hits) != 1 || hits[0].Lang != "python" {
		t.Fatalf("lang filter = %+v, want one python hit", hits)
	}
}

func TestStoreFilterByRepo(t *testing.T) {
	emb := newFakeEmbedder(8)
	s, chunks := planStore(t, emb)
	q := embedAll(t, emb, chunks[:1])[0]

	if got := len(s.Search(q, 10, Filter{Repos: []string{"org"}})); got != 3 {
		t.Fatalf("repo substring 'org' returned %d hits, want 3", got)
	}
	if got := len(s.Search(q, 10, Filter{Repos: []string{"nomatch"}})); got != 0 {
		t.Fatalf("repo 'nomatch' returned %d hits, want 0", got)
	}
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	emb := newFakeEmbedder(8)
	s, chunks := planStore(t, emb)

	path := filepath.Join(t.TempDir(), "store.gob")
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}

	if loaded.Dim() != s.Dim() {
		t.Fatalf("dim = %d, want %d", loaded.Dim(), s.Dim())
	}
	if len(loaded.Files()) != len(s.Files()) {
		t.Fatalf("files = %v, want %v", loaded.Files(), s.Files())
	}
	for path, hash := range s.Files() {
		if loaded.Files()[path] != hash {
			t.Fatalf("file %s hash mismatch: %s vs %s", path, loaded.Files()[path], hash)
		}
	}

	q := embedAll(t, emb, chunks[:1])[0]
	a := s.Search(q, 10, Filter{})
	b := loaded.Search(q, 10, Filter{})
	if len(a) != len(b) {
		t.Fatalf("hit count differs after reload: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("hit %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}

func TestStoreChunkerVersionRoundTrip(t *testing.T) {
	s := NewStore(8)
	if s.ChunkerVersion() != chunkerVersion {
		t.Fatalf("NewStore ChunkerVersion = %d, want %d", s.ChunkerVersion(), chunkerVersion)
	}

	path := filepath.Join(t.TempDir(), "store.gob")
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}
	if loaded.ChunkerVersion() != chunkerVersion {
		t.Fatalf("loaded ChunkerVersion = %d, want %d", loaded.ChunkerVersion(), chunkerVersion)
	}
}

func TestLoadStoreLegacyGobReportsVersionZero(t *testing.T) {
	// Stores written before chunker versioning lack the field entirely; gob
	// must decode them with ChunkerVersion 0 so the indexer rebuilds them.
	type legacyPersisted struct {
		Dim      int
		RepoName string
		RepoPath string
		Files    map[string]fileEntry
	}
	path := filepath.Join(t.TempDir(), "legacy.gob")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	snap := legacyPersisted{Dim: 8, RepoName: "org/repo", Files: map[string]fileEntry{}}
	if err := gob.NewEncoder(f).Encode(snap); err != nil {
		t.Fatalf("encode legacy store: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore legacy: %v", err)
	}
	if loaded.ChunkerVersion() != 0 {
		t.Fatalf("legacy ChunkerVersion = %d, want 0", loaded.ChunkerVersion())
	}
}

func TestLoadStoreMissingFile(t *testing.T) {
	s, err := LoadStore(filepath.Join(t.TempDir(), "does-not-exist.gob"))
	if err != nil {
		t.Fatalf("LoadStore missing: %v", err)
	}
	if len(s.Files()) != 0 {
		t.Fatalf("expected empty store, got %d files", len(s.Files()))
	}
}

func TestStorePutFileReplaces(t *testing.T) {
	emb := newFakeEmbedder(8)
	s, _ := planStore(t, emb)

	// Replace a.go with a single new chunk.
	repl := []Chunk{mkChunk("org/repo", "a.go", "go", "type_declaration", 1, 3, "delta go type")}
	s.PutFile("a.go", "hash-a2", repl, embedAll(t, emb, repl))

	q := embedAll(t, emb, repl)[0]
	hits := s.Search(q, 10, Filter{})
	// a.go now contributes only one chunk; b.py still one => 2 total.
	if len(hits) != 2 {
		t.Fatalf("after replace got %d hits, want 2", len(hits))
	}
	if s.Files()["a.go"] != "hash-a2" {
		t.Fatalf("hash not updated: %s", s.Files()["a.go"])
	}
}

func TestStoreDeleteFile(t *testing.T) {
	emb := newFakeEmbedder(8)
	s, chunks := planStore(t, emb)

	s.DeleteFile("b.py")
	if _, ok := s.Files()["b.py"]; ok {
		t.Fatalf("b.py still present after delete")
	}
	q := embedAll(t, emb, chunks[2:])[0] // c3 embedding
	for _, h := range s.Search(q, 10, Filter{}) {
		if h.Path == "b.py" {
			t.Fatalf("deleted file still in search results")
		}
	}
}
