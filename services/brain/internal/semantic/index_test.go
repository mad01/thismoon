package semantic

import (
	"context"
	"hash/fnv"
	"math"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/chunk"
)

// hashEmbed produces a deterministic, normalized vector from the input string
// without calling Ollama. The two-dimensional split keeps "queryable":
// strings sharing words score higher than strings that don't.
func hashEmbed(_ context.Context, text string) ([]float32, error) {
	const dim = 16
	v := make([]float32, dim)
	// Hash trick: bucket each whitespace-separated word into one of dim
	// slots. This guarantees similar texts produce similar vectors.
	for _, w := range fields(text) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(w))
		v[int(h.Sum32())%dim] += 1
	}
	// Normalize.
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		v[0] = 1
		return v, nil
	}
	n := math.Sqrt(sum)
	for i := range v {
		v[i] = float32(float64(v[i]) / n)
	}
	return v, nil
}

func fields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == ',' || r == '.' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func TestIndex_UpsertQuery(t *testing.T) {
	dir := t.TempDir()
	idx, err := Open(dir, hashEmbed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	chunks := []chunk.Chunk{
		{
			ID:     "a-0001",
			Path:   "notes/a.md",
			Title:  "A",
			Body:   "kubernetes graceful shutdown handling",
			Tier:   "notes",
			Tags:   []string{"infra"},
			Type:   "note",
			Status: "validated",
		},
		{
			ID:       "b-0002",
			Path:     "notes/b.md",
			NoteID:   "01JABCDEFGHJKMNPQRSTVWXYZ0",
			Title:    "B",
			Body:     "concept alpha builds on foundational ideas",
			Tier:     "notes",
			Tags:     []string{"ai/research"},
			Type:     "note",
			Status:   "validated",
			Evidence: "A",
		},
		{
			ID:     "c-0003",
			Path:   "outputs/c.md",
			Title:  "C",
			Body:   "kubernetes pod lifecycle and signals",
			Tier:   "outputs",
			Tags:   []string{"infra"},
			Type:   "output",
			Status: "validated",
		},
	}

	ctx := context.Background()
	if err := idx.Upsert(ctx, chunks, 1); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got := idx.Count(); got != 3 {
		t.Fatalf("Count = %d, want 3", got)
	}

	// Query that should rank kubernetes notes above the AI/research note.
	hits, err := idx.Query(ctx, "kubernetes signals shutdown", QueryOptions{K: 3})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits")
	}
	if hits[0].Path != "notes/a.md" && hits[0].Path != "outputs/c.md" {
		t.Errorf("top hit = %s, want a kubernetes note", hits[0].Path)
	}

	// Tier filter.
	hits, err = idx.Query(ctx, "kubernetes", QueryOptions{K: 3, Tier: "outputs"})
	if err != nil {
		t.Fatalf("Query tier: %v", err)
	}
	for _, h := range hits {
		if h.Tier != "outputs" {
			t.Errorf("tier filter leaked: %+v", h)
		}
	}

	// Tag filter — exact-match via flattened key.
	hits, err = idx.Query(ctx, "alpha", QueryOptions{K: 3, Tag: "ai/research"})
	if err != nil {
		t.Fatalf("Query tag: %v", err)
	}
	for _, h := range hits {
		found := false
		for _, t := range h.Tags {
			if t == "ai/research" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tag filter leaked: %+v", h)
		}
	}

	// Evidence filter.
	hits, err = idx.Query(ctx, "alpha", QueryOptions{K: 3, Evidence: "A"})
	if err != nil {
		t.Fatalf("Query evidence: %v", err)
	}
	for _, h := range hits {
		if h.Evidence != "A" {
			t.Errorf("evidence filter leaked: %+v", h)
		}
		if h.NoteID != "01JABCDEFGHJKMNPQRSTVWXYZ0" {
			t.Errorf("hit NoteID = %q, want the note id stored with the chunk", h.NoteID)
		}
	}
}

func TestIndex_DeleteByPath(t *testing.T) {
	dir := t.TempDir()
	idx, _ := Open(dir, hashEmbed)
	ctx := context.Background()

	_ = idx.Upsert(ctx, []chunk.Chunk{
		{ID: "a-0001", Path: "notes/a.md", Title: "A", Body: "alpha"},
		{ID: "a-0002", Path: "notes/a.md", Title: "A", Body: "beta"},
		{ID: "b-0001", Path: "notes/b.md", Title: "B", Body: "gamma"},
	}, 1)
	if got := idx.Count(); got != 3 {
		t.Fatalf("Count = %d, want 3", got)
	}
	if err := idx.DeleteByPath(ctx, "notes/a.md"); err != nil {
		t.Fatalf("DeleteByPath: %v", err)
	}
	if got := idx.Count(); got != 1 {
		t.Errorf("after delete: Count = %d, want 1", got)
	}
}

func TestIndex_UpsertReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	idx, _ := Open(dir, hashEmbed)
	ctx := context.Background()

	_ = idx.Upsert(ctx, []chunk.Chunk{{ID: "a-0001", Path: "notes/a.md", Body: "first"}}, 1)
	_ = idx.Upsert(ctx, []chunk.Chunk{{ID: "a-0001", Path: "notes/a.md", Body: "second"}}, 1)

	if got := idx.Count(); got != 1 {
		t.Errorf("Count = %d, want 1 (upsert should replace)", got)
	}
}

func TestIndex_EmptyQuery(t *testing.T) {
	dir := t.TempDir()
	idx, _ := Open(dir, hashEmbed)
	if _, err := idx.Query(context.Background(), "", QueryOptions{}); err == nil {
		t.Error("want error for empty query")
	}
}

func TestIndex_QueryOnEmptyIndex(t *testing.T) {
	dir := t.TempDir()
	idx, _ := Open(dir, hashEmbed)
	hits, err := idx.Query(context.Background(), "anything", QueryOptions{K: 5})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("hits = %d, want 0", len(hits))
	}
}

func TestIndex_ResetEmptiesThePersistedIndex(t *testing.T) {
	dir := t.TempDir()
	idx, err := Open(dir, hashEmbed)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	chunks := []chunk.Chunk{{ID: "a-0000", Path: "notes/a.md", Body: "alpha"}}
	if err := idx.Upsert(ctx, chunks, 1); err != nil {
		t.Fatal(err)
	}
	if err := idx.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if n := idx.Count(); n != 0 {
		t.Errorf("Count after Reset = %d, want 0", n)
	}
	if err := idx.Upsert(ctx, chunks, 1); err != nil {
		t.Fatalf("Upsert after Reset: %v", err)
	}
	reopened, err := Open(dir, hashEmbed)
	if err != nil {
		t.Fatal(err)
	}
	if n := reopened.Count(); n != 1 {
		t.Errorf("reopened Count = %d, want the one chunk added after Reset", n)
	}
}
