package semantic

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Index is an aggregate, read-only view over every per-repo vector Store under
// a semantic index directory. It fans a query out to each store and merges the
// per-store hits into a single global ranking.
type Index struct {
	stores []*Store
}

// OpenIndex loads every per-repo store (*.gob, excluding the state file) under
// indexDir into a single aggregate. A missing indexDir is not an error: it
// yields an empty Index. Individual stores that fail to load are skipped so one
// corrupt file does not sink the whole index.
func OpenIndex(indexDir string) (*Index, error) {
	entries, err := os.ReadDir(indexDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &Index{}, nil
		}
		return nil, fmt.Errorf("read semantic index dir %s: %w", indexDir, err)
	}

	ix := &Index{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".gob") {
			continue
		}
		store, err := LoadStore(filepath.Join(indexDir, e.Name()))
		if err != nil {
			continue // skip unreadable store; the rest still serve
		}
		ix.stores = append(ix.stores, store)
	}
	return ix, nil
}

// Search fans query out to every store and merges into a global top-k by score.
// Ties break deterministically by repo, then path, then StartLine. k <= 0
// returns all matching hits.
func (ix *Index) Search(query []float32, k int, f Filter) []Hit {
	var hits []Hit
	for _, s := range ix.stores {
		hits = append(hits, s.Search(query, k, f)...)
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Repo != hits[j].Repo {
			return hits[i].Repo < hits[j].Repo
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

// Len returns the total number of chunks across all stores.
func (ix *Index) Len() int {
	n := 0
	for _, s := range ix.stores {
		n += s.chunkCount()
	}
	return n
}

// Stores returns the number of per-repo stores in the aggregate.
func (ix *Index) Stores() int {
	return len(ix.stores)
}

// ExpandHit reads the source file behind h and returns the text of lines
// [StartLine-padLines .. EndLine+padLines] (1-based, inclusive, clamped to the
// file's bounds). This is the parent-child retrieval step: a chunk hit is
// widened back to readable context. Best-effort; errors are wrapped with %w.
func ExpandHit(h Hit, padLines int) (string, error) {
	if padLines < 0 {
		padLines = 0
	}
	path := filepath.Join(h.RepoPath, h.Path)
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open source %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	start := h.StartLine - padLines
	if start < 1 {
		start = 1
	}
	end := h.EndLine + padLines

	var b strings.Builder
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxFileSize)
	line := 0
	for scanner.Scan() {
		line++
		if line < start {
			continue
		}
		if line > end {
			break
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read source %s: %w", path, err)
	}
	return b.String(), nil
}
