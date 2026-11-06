// Package semantic wraps chromem-go to provide a vault-aware vector index
// over note chunks. Each vault's Index persists at semantic/<vault>/ in
// brain's state directory (or a configured path) and is keyed by chunk ID;
// metadata carries the path, note id, tier, tags (flattened as t_<name>=1),
// evidence, type and status. A Manifest beside it records each embedded
// note's content hash, so Sync re-embeds only the notes that changed and
// purges the chunks of notes that vanished.
package semantic

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	chromem "github.com/philippgille/chromem-go"

	"github.com/mad01/thismoon/services/brain/internal/chunk"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
)

// CollectionName is the single chromem-go collection used by brain.
const CollectionName = "brain-vault"

// EmbedFunc matches chromem-go's EmbeddingFunc signature so tests can pass
// a deterministic stub without depending on a live Ollama.
type EmbedFunc = chromem.EmbeddingFunc

// Index is a brain-flavored handle over chromem-go.
type Index struct {
	db    *chromem.DB
	col   *chromem.Collection
	dir   string
	embed EmbedFunc
}

// Open opens (or creates) the persistent semantic index at dir and returns
// a wired Index. The collection is created on first run and reused on every
// subsequent call.
//
// embed is the function used to vectorize chunk bodies and queries. In
// production it's an Ollama call; in tests it's a deterministic stub.
func Open(dir string, embed EmbedFunc) (*Index, error) {
	if dir == "" {
		return nil, fmt.Errorf("semantic.Open: dir is required")
	}
	if embed == nil {
		return nil, fmt.Errorf("semantic.Open: embed function is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("semantic.Open: mkdir %s: %w", dir, err)
	}
	db, err := chromem.NewPersistentDB(dir, false)
	if err != nil {
		return nil, fmt.Errorf("semantic.Open: open db at %s: %w", dir, err)
	}
	col, err := db.GetOrCreateCollection(CollectionName, nil, embed)
	if err != nil {
		return nil, fmt.Errorf("semantic.Open: collection: %w", err)
	}
	return &Index{db: db, col: col, dir: dir, embed: embed}, nil
}

// Count returns the number of chunks currently in the index.
func (i *Index) Count() int { return i.col.Count() }

// Dir returns the on-disk directory backing this index.
func (i *Index) Dir() string { return i.dir }

// Upsert adds chunks to the index. Existing chunk IDs are deleted and
// re-added. Embedding is performed by chromem-go via the embed function the
// Index was opened with; concurrency follows chromem-go's recommended
// "AddDocuments with parallelism" pattern.
//
// Pass concurrency <= 0 to use the default (4 — Ollama is generally
// embedding-bound, not request-bound).
func (i *Index) Upsert(ctx context.Context, chunks []chunk.Chunk, concurrency int) error {
	if len(chunks) == 0 {
		return nil
	}
	// Delete existing IDs first so re-indexing the same chunk doesn't double up.
	ids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		ids = append(ids, c.ID)
	}
	if err := i.col.Delete(ctx, nil, nil, ids...); err != nil {
		return fmt.Errorf("semantic.Upsert: delete existing: %w", err)
	}

	docs := make([]chromem.Document, 0, len(chunks))
	for _, c := range chunks {
		docs = append(docs, chromem.Document{
			ID:       c.ID,
			Metadata: chunkMetadata(c),
			Content:  c.Body,
		})
	}
	if concurrency <= 0 {
		concurrency = 4
	}
	if err := i.col.AddDocuments(ctx, docs, concurrency); err != nil {
		return fmt.Errorf("semantic.Upsert: add: %w", err)
	}
	return nil
}

// DeleteByPath removes every chunk whose metadata.path equals path.
func (i *Index) DeleteByPath(ctx context.Context, path string) error {
	if path == "" {
		return fmt.Errorf("semantic.DeleteByPath: empty path")
	}
	if err := i.col.Delete(ctx, map[string]string{"path": path}, nil, nil...); err != nil {
		return fmt.Errorf("semantic.DeleteByPath %s: %w", path, err)
	}
	return nil
}

// QueryOptions configures a semantic query.
type QueryOptions struct {
	// K is the number of results to return after filtering. K <= 0 → 10.
	K int
	// Tier restricts to a single vault tier ("inbox" / "notes" / "outputs"
	// / "MOCs"). Empty means all tiers.
	Tier string
	// Tag, if non-empty, requires that the chunk's note carries this tag.
	Tag string
	// Type, if non-empty, restricts to chunks whose note frontmatter type
	// matches.
	Type string
	// Status, if non-empty, restricts to an effective status, in any case:
	// the stored one, else the default of the note's tier.
	Status string
	// Evidence, if non-empty, restricts to a single evidence tier.
	Evidence string
}

// Hit is one ranked result from Query.
type Hit struct {
	// ID is the chunk ID.
	ID string
	// Vault is the name of the vault the note is in. The index does not
	// store it; the caller that queried a vault's index sets it.
	Vault string
	// Path is the vault-relative note path.
	Path string
	// NoteID is the note's frontmatter id, empty when the note had none.
	NoteID string
	// Title is the note title (frontmatter.title or filename stem).
	Title string
	// HeadingPath joins the headings above this chunk with " › ".
	HeadingPath string
	// Snippet is the chunk body (caller may truncate for display).
	Snippet string
	// Score is chromem-go's cosine similarity in [-1, 1]. Higher = better.
	Score float32
	// Tier of the source note.
	Tier string
	// Tags of the source note.
	Tags []string
	// Type of the source note.
	Type string
	// Status of the source note.
	Status string
	// Evidence tier of the source note ("" if unset).
	Evidence string
	// ReviewBy and Stale are the note's review date and whether it has
	// passed. The index does not store them; the library reads them from
	// the note, along with its current Status.
	ReviewBy string
	Stale    bool
	// Seq is the chunk's sequence number within the note.
	Seq int
}

// Query runs a vector search. Filters are applied as a chromem-go where
// clause when single-valued (tier/type/status/evidence) and as a flattened
// metadata key for tag (`t_<tag>=1`). To avoid filter starvation, we ask
// chromem-go for K * 4 hits when filters are present and post-filter to K.
func (i *Index) Query(ctx context.Context, q string, opts QueryOptions) ([]Hit, error) {
	if strings.TrimSpace(q) == "" {
		return nil, fmt.Errorf("semantic.Query: empty query")
	}
	if i.col.Count() == 0 {
		return nil, nil
	}

	k := opts.K
	if k <= 0 {
		k = 10
	}

	where := map[string]string{}
	if opts.Tier != "" {
		where["tier"] = opts.Tier
	}
	if opts.Type != "" {
		where["type"] = opts.Type
	}
	if opts.Status != "" {
		where["status"] = lifecycle.Normalize(opts.Status)
	}
	if opts.Evidence != "" {
		where["evidence"] = opts.Evidence
	}
	if opts.Tag != "" {
		where["t_"+strings.ToLower(opts.Tag)] = "1"
	}
	if len(where) == 0 {
		where = nil
	}

	// Asking for more than we need is cheap (brute force; chromem still
	// scans every doc) and lets us survive any future filter mismatches.
	want := k
	if want > i.col.Count() {
		want = i.col.Count()
	}

	results, err := i.col.Query(ctx, q, want, where, nil)
	if err != nil {
		return nil, fmt.Errorf("semantic.Query: %w", err)
	}

	hits := make([]Hit, 0, len(results))
	for _, r := range results {
		hits = append(hits, hitFromResult(r))
	}
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits, nil
}

// hitFromResult unpacks a chromem-go Result into our Hit shape.
func hitFromResult(r chromem.Result) Hit {
	seq := 0
	if s, ok := r.Metadata["seq"]; ok {
		if n, err := strconv.Atoi(s); err == nil {
			seq = n
		}
	}
	tags := unmarshalTags(r.Metadata["tags"])
	return Hit{
		ID:          r.ID,
		Path:        r.Metadata["path"],
		NoteID:      r.Metadata["note_id"],
		Title:       r.Metadata["title"],
		HeadingPath: r.Metadata["heading_path"],
		Snippet:     r.Content,
		Score:       r.Similarity,
		Tier:        r.Metadata["tier"],
		Tags:        tags,
		Type:        r.Metadata["type"],
		Status:      r.Metadata["status"],
		Evidence:    r.Metadata["evidence"],
		Seq:         seq,
	}
}

// chunkMetadata builds the metadata map for a chunk. chromem-go metadata is
// strictly map[string]string, so:
//   - tags are joined with "|" under "tags" for round-trip and per-tag
//     keys "t_<name>=1" for cheap exact-match filtering.
//   - seq is rendered as decimal text.
func chunkMetadata(c chunk.Chunk) map[string]string {
	m := map[string]string{
		"path":         c.Path,
		"note_id":      c.NoteID,
		"title":        c.Title,
		"heading_path": c.HeadingPath,
		"tier":         c.Tier,
		"type":         c.Type,
		"status":       c.Status,
		"evidence":     c.Evidence,
		"hash":         c.Hash,
		"tags":         strings.Join(c.Tags, "|"),
		"seq":          strconv.Itoa(c.Seq),
	}
	for _, t := range c.Tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		m["t_"+t] = "1"
	}
	return m
}

func unmarshalTags(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Reset empties the index by dropping its collection and creating it again.
// chromem-go refuses a Delete without a filter, so this is the way to clear
// every document.
func (i *Index) Reset(_ context.Context) error {
	if err := i.db.DeleteCollection(CollectionName); err != nil {
		return fmt.Errorf("semantic.Reset: drop collection: %w", err)
	}
	col, err := i.db.GetOrCreateCollection(CollectionName, nil, i.embed)
	if err != nil {
		return fmt.Errorf("semantic.Reset: collection: %w", err)
	}
	i.col = col
	return nil
}
