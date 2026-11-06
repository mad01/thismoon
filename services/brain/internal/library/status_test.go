package library

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/semantic"
)

// TestSemanticHitsFollowTheNoteStatus pins that a semantic hit reports the
// note's status as it is on disk now, so a note retracted after it was
// embedded drops out of default results even when no re-sync can run.
func TestSemanticHitsFollowTheNoteStatus(t *testing.T) {
	f := newSemanticFixture(t, map[string]string{
		"notes/a.md": "---\ntitle: A\nstatus: validated\nreview_by: 2020-01-01\n---\nzebra stripes run\n",
		"notes/b.md": "---\ntitle: B\nstatus: validated\n---\nzebra stripes rest\n",
	})
	lib, v := f.open("model-a")
	f.refresh(lib, v, false)
	f.write(
		"notes/b.md",
		"---\ntitle: B\nstatus: retracted\nretracted_reason: error\n---\nzebra stripes rest\n",
	)
	// With the embedder's health probe failing, the query skips the re-sync
	// and the index still holds b's chunks as validated.
	lib, err := Open(Options{
		Config: lib.Config(),
		Embed:  f.embed.embed,
		Health: func(context.Context) error { return errors.New("ollama is down") },
	})
	if err != nil {
		t.Fatal(err)
	}

	query := func(include bool) map[string]semantic.ResultHit {
		t.Helper()
		res, err := lib.SemanticSearch(context.Background(), lib.Set(), SemanticQuery{
			Text: "zebra stripes", Mode: semantic.ModePaths,
			Filter: semantic.QueryOptions{K: 10}, IncludeRevoked: include,
		})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]semantic.ResultHit{}
		for _, h := range res.Hits {
			out[h.Path] = h
		}
		return out
	}
	hits := query(false)
	if _, ok := hits["notes/b.md"]; ok {
		t.Errorf("default results include the retracted note: %v", hits)
	}
	if a := hits["notes/a.md"]; a.ReviewBy != "2020-01-01" || !a.Stale {
		t.Errorf("hit a = %+v, want review_by 2020-01-01 and stale", a)
	}
	if b, ok := query(true)["notes/b.md"]; !ok || b.Status != "retracted" {
		t.Errorf("include revoked: hit b = %+v, want its current status retracted", b)
	}
}

// TestSemanticStatusFilterMatchesTheTierDefault pins that the semantic
// status filter matches a note's effective status: a note with no status
// line has its tier's default, raw in the inbox and validated elsewhere.
func TestSemanticStatusFilterMatchesTheTierDefault(t *testing.T) {
	f := newSemanticFixture(t, semanticNotes)
	lib, v := f.open("model-a")
	f.refresh(lib, v, false)
	paths := func(status string) []string {
		t.Helper()
		res, err := lib.SemanticSearch(context.Background(), lib.Set(), SemanticQuery{
			Text: "apple", Mode: semantic.ModePaths,
			Filter: semantic.QueryOptions{K: 10, Status: status},
		})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, h := range res.Hits {
			out = append(out, h.Path)
		}
		slices.Sort(out)
		return out
	}
	if got, want := paths("validated"), []string{"notes/a.md", "notes/b.md"}; !slices.Equal(got, want) {
		t.Errorf("status validated = %q, want %q", got, want)
	}
	if got, want := paths("Raw"), []string{"inbox/c.md"}; !slices.Equal(got, want) {
		t.Errorf("status Raw = %q, want %q", got, want)
	}
}
