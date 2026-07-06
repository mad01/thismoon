package semantic

import (
	"encoding/gob"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// storedChunk is the persisted, lean form of a Chunk: locator metadata plus its
// L2-normalized embedding. The chunk's Text/EmbedText are deliberately NOT
// stored — result snippets are re-read from the source file by line range.
type storedChunk struct {
	Path      string
	Lang      string
	Kind      string
	StartLine int
	EndLine   int
	Vector    []float32
}

// fileEntry is the set of chunks for a single source file, keyed by a content
// hash so re-indexing can skip unchanged files.
type fileEntry struct {
	ContentHash string
	Chunks      []storedChunk
}

// Store is a flat, file-level-incremental vector store for ONE repo. It holds
// every chunk's normalized embedding in memory and scores queries by dot
// product. Safe for concurrent use.
type Store struct {
	mu       sync.RWMutex
	dim      int
	repoName string
	repoPath string               // absolute path to the repo on disk
	files    map[string]fileEntry // key: repo-relative file path
}

// NewStore returns an empty store of the given embedding dimensionality.
func NewStore(dim int) *Store {
	return &Store{dim: dim, files: make(map[string]fileEntry)}
}

// Dim returns the embedding dimensionality.
func (s *Store) Dim() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dim
}

// RepoName returns the repo name this store covers ("" until first PutFile).
func (s *Store) RepoName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.repoName
}

// RepoPath returns the absolute path to the repo this store covers ("" until
// set by SetRepoPath).
func (s *Store) RepoPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.repoPath
}

// SetRepoPath records the repo's absolute on-disk path so search hits can be
// expanded against the original source files.
func (s *Store) SetRepoPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.repoPath = path
}

// chunkCount returns the total number of chunks held across all files.
func (s *Store) chunkCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, e := range s.files {
		n += len(e.Chunks)
	}
	return n
}

// Files returns a copy of the path -> contentHash map for incremental diffing.
func (s *Store) Files() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.files))
	for path, e := range s.files {
		out[path] = e.ContentHash
	}
	return out
}

// PutFile replaces all chunks for path with chunks, normalizing vecs. The
// caller must pass len(chunks) == len(vecs).
func (s *Store) PutFile(path, contentHash string, chunks []Chunk, vecs [][]float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.repoName == "" && len(chunks) > 0 {
		s.repoName = chunks[0].Repo
	}
	stored := make([]storedChunk, len(chunks))
	for i, c := range chunks {
		stored[i] = storedChunk{
			Path:      c.Path,
			Lang:      c.Lang,
			Kind:      c.Kind,
			StartLine: c.StartLine,
			EndLine:   c.EndLine,
			Vector:    normalize(vecs[i]),
		}
	}
	s.files[path] = fileEntry{ContentHash: contentHash, Chunks: stored}
}

// DeleteFile removes a file and all its chunks from the store.
func (s *Store) DeleteFile(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, path)
}

// Filter narrows a search. Empty slices mean no filter. Repos matches as a
// substring against the store's repo name; Langs matches exactly.
type Filter struct {
	Repos []string
	Langs []string
}

// Hit is a single scored search result.
type Hit struct {
	Repo      string
	RepoPath  string
	Path      string
	Lang      string
	Kind      string
	StartLine int
	EndLine   int
	Score     float32
}

// Search returns the top-k chunks by cosine similarity (dot product over
// normalized vectors) among chunks passing f. Ties break by path then
// StartLine. k <= 0 returns all matching hits.
func (s *Store) Search(query []float32, k int, f Filter) []Hit {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.repoMatches(f) {
		return nil
	}
	q := normalize(query)
	langOK := langMatcher(f.Langs)

	var hits []Hit
	for _, e := range s.files {
		for _, c := range e.Chunks {
			if !langOK(c.Lang) {
				continue
			}
			hits = append(hits, Hit{
				Repo:      s.repoName,
				RepoPath:  s.repoPath,
				Path:      c.Path,
				Lang:      c.Lang,
				Kind:      c.Kind,
				StartLine: c.StartLine,
				EndLine:   c.EndLine,
				Score:     dot(q, c.Vector),
			})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Path != hits[j].Path {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].StartLine < hits[j].StartLine
	})
	if k > 0 && len(hits) > k {
		hits = hits[:k]
	}
	return hits
}

// repoMatches reports whether the store's repo passes f.Repos.
func (s *Store) repoMatches(f Filter) bool {
	if len(f.Repos) == 0 {
		return true
	}
	for _, want := range f.Repos {
		if want != "" && strings.Contains(s.repoName, want) {
			return true
		}
	}
	return false
}

// langMatcher returns a predicate that accepts a lang against the filter set.
func langMatcher(langs []string) func(string) bool {
	if len(langs) == 0 {
		return func(string) bool { return true }
	}
	set := make(map[string]bool, len(langs))
	for _, l := range langs {
		set[l] = true
	}
	return func(lang string) bool { return set[lang] }
}

// persisted is the on-disk gob representation. encoding/gob keeps this simple
// (KISS); a denser binary format is a future optimization.
type persisted struct {
	Dim      int
	RepoName string
	RepoPath string
	Files    map[string]fileEntry
}

// Save writes the store to path atomically (temp file then rename).
func (s *Store) Save(path string) error {
	s.mu.RLock()
	snap := persisted{Dim: s.dim, RepoName: s.repoName, RepoPath: s.repoPath, Files: s.files}
	s.mu.RUnlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create store dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp store file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	if err := gob.NewEncoder(tmp).Encode(snap); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("encode store %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close store %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename store %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}

// LoadStore reads a store from path. A missing file yields a fresh empty store.
func LoadStore(path string) (*Store, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewStore(0), nil
		}
		return nil, fmt.Errorf("open store %s: %w", path, err)
	}
	defer f.Close()

	var snap persisted
	if err := gob.NewDecoder(f).Decode(&snap); err != nil {
		return nil, fmt.Errorf("decode store %s: %w", path, err)
	}
	if snap.Files == nil {
		snap.Files = make(map[string]fileEntry)
	}
	return &Store{dim: snap.Dim, repoName: snap.RepoName, repoPath: snap.RepoPath, files: snap.Files}, nil
}

// normalize returns an L2-normalized copy of v (unchanged if its norm is zero).
func normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	out := make([]float32, len(v))
	if sum == 0 {
		copy(out, v)
		return out
	}
	inv := float32(1 / math.Sqrt(sum))
	for i, x := range v {
		out[i] = x * inv
	}
	return out
}

// dot returns the dot product of two equal-length-ish vectors.
func dot(a, b []float32) float32 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var s float32
	for i := 0; i < n; i++ {
		s += a[i] * b[i]
	}
	return s
}
