package semantic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// errContextLength is how Ollama answers a prompt longer than the embed
// model's context.
var errContextLength = errors.New(`embed: ollama 500 Internal Server Error: ` +
	`{"error":"the input length exceeds the context length"}`)

// limitEmbed is hashEmbed behind an input limit: a text over max bytes fails
// the way Ollama fails a prompt the model's context cannot hold.
type limitEmbed struct{ max int }

func (e *limitEmbed) embed(ctx context.Context, text string) ([]float32, error) {
	if len(text) > e.max {
		return nil, errContextLength
	}
	return hashEmbed(ctx, text)
}

// TestSyncSkipsANoteTheEmbedderRejects pins that one note the embedder
// refuses fails alone: the sync embeds the rest, names the note and its
// reason, and leaves no chunk of it behind, not even the chunks of the
// version embedded before.
func TestSyncSkipsANoteTheEmbedderRejects(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("notes/a.md", "---\ntitle: A\n---\nalpha apple orchard\n")
	write("notes/b.md", "---\ntitle: B\n---\nbravo zebra stripes\n")
	write("MOCs/big.md", "---\ntitle: Big\n---\ncharlie cherry tree\n")
	embedder := &limitEmbed{max: 1000}
	idx, err := Open(t.TempDir(), embedder.embed)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := Sync(ctx, idx, root, SyncOptions{})
	if err != nil || len(first.Failed) != 0 {
		t.Fatalf("first sync = %+v, %v; want every note embedded", first, err)
	}

	// One line with no break: under the chunk bound, over the embedder's limit.
	write("MOCs/big.md", "---\ntitle: Big\n---\ncharlie "+strings.Repeat("x", 1500)+"\n")
	write("notes/a.md", "---\ntitle: A\n---\nalpha apple orchard, revised\n")
	report, err := Sync(ctx, idx, root, SyncOptions{Held: first.Notes})
	if err != nil {
		t.Fatalf("sync with a rejected note = %v, want the rest synced", err)
	}
	if reason := report.Failed["MOCs/big.md"]; len(report.Failed) != 1 ||
		!strings.Contains(reason, "exceeds the context length") {
		t.Errorf("Failed = %v, want MOCs/big.md with the embedder's reason", report.Failed)
	}
	if _, held := report.Notes["MOCs/big.md"]; held {
		t.Error("the rejected note is in Notes, so the next sync would not retry it")
	}
	if _, ok := report.Notes["notes/a.md"]; !ok || report.Embedded != 1 || report.Removed != 0 {
		t.Errorf("sync = %+v, want notes/a.md re-embedded and nothing removed", report)
	}
	hits, err := idx.Query(ctx, "charlie cherry tree alpha apple", QueryOptions{K: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.Path == "MOCs/big.md" {
			t.Errorf("hit %q of the rejected note, want none of its chunks left", h.Snippet)
		}
	}
}
