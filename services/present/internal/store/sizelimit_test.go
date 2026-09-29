package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// limitBytes is small enough that a short page fits and a page with a
// paragraph of content does not, which keeps the fixtures readable.
const limitBytes = 512

func limitedFS(t *testing.T) (Store, *FS) {
	t.Helper()
	fs, err := NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	return WithSizeLimit(fs, limitBytes), fs
}

func TestSizeLimitRefusesAnOversizedCreate(t *testing.T) {
	limited, fs := limitedFS(t)
	ctx := context.Background()

	if _, err := limited.Create(ctx, Draft{Title: "Small", Content: "<p>fits</p>"}); err != nil {
		t.Fatalf("create under the limit: %v", err)
	}
	_, err := limited.Create(ctx, Draft{
		Title:   "Big",
		Content: "<p>" + strings.Repeat("x", limitBytes) + "</p>",
	})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("create over the limit: err = %v, want ErrTooLarge", err)
	}
	pages, err := fs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Title != "Small" {
		t.Fatalf("store holds %d pages, want only the small one", len(pages))
	}
}

func TestSizeLimitRefusesAnOversizedUpdate(t *testing.T) {
	limited, fs := limitedFS(t)
	ctx := context.Background()
	p, err := limited.Create(ctx, Draft{Title: "T", Content: "<p>v1</p>"})
	if err != nil {
		t.Fatal(err)
	}

	big := "<p>" + strings.Repeat("x", limitBytes) + "</p>"
	if _, err := limited.Update(ctx, p.ID, Patch{Content: &big}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("update over the limit: err = %v, want ErrTooLarge", err)
	}
	got, err := fs.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "<p>v1</p>" || got.Version != 1 {
		t.Fatalf(
			"page after a refused update = %q v%d, want the original",
			got.Content,
			got.Version,
		)
	}

	small := "<p>v2</p>"
	if _, err := limited.Update(ctx, p.ID, Patch{Content: &small}); err != nil {
		t.Fatalf("update under the limit: %v", err)
	}
}

func TestSizeLimitCountsTheSourcesToo(t *testing.T) {
	limited, fs := limitedFS(t)
	ctx := context.Background()
	p, err := limited.Create(ctx, Draft{Title: "T", Content: "<p>v1</p>"})
	if err != nil {
		t.Fatal(err)
	}

	big := []byte(`{"note":"` + strings.Repeat("d", limitBytes) + `"}`)
	if err := limited.SaveDoc(ctx, p.ID, big); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("SaveDoc over the limit: err = %v, want ErrTooLarge", err)
	}
	if _, err := fs.LoadDoc(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a refused SaveDoc must write nothing: err = %v", err)
	}
	if err := limited.SaveGraphSource(ctx, p.ID, big); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("SaveGraphSource over the limit: err = %v, want ErrTooLarge", err)
	}
	if _, err := fs.LoadGraphSource(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a refused SaveGraphSource must write nothing: err = %v", err)
	}

	small := []byte(`{"sections":[]}`)
	if err := limited.SaveDoc(ctx, p.ID, small); err != nil {
		t.Fatalf("SaveDoc under the limit: %v", err)
	}
	// The doc now counts against the page's budget, so content that fit on
	// its own no longer does.
	almost := "<p>" + strings.Repeat("y", limitBytes-len(small)-64) + "</p>"
	if _, err := limited.Update(ctx, p.ID, Patch{Content: &almost}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("update that fits only without its doc: err = %v, want ErrTooLarge", err)
	}
}

func TestSizeLimitPassesReadsAndDeletesThrough(t *testing.T) {
	limited, _ := limitedFS(t)
	ctx := context.Background()
	p, err := limited.Create(ctx, Draft{Title: "T", Content: "<p>v1</p>"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := limited.Get(ctx, p.ID); err != nil || got.Title != "T" {
		t.Fatalf("Get through the wrapper = %+v, %v", got, err)
	}
	if err := limited.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete through the wrapper: %v", err)
	}
	if _, err := limited.Get(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete: err = %v, want ErrNotFound", err)
	}
	if _, err := limited.Update(ctx, p.ID, Patch{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update of a missing page: err = %v, want ErrNotFound", err)
	}
}

func TestSizeLimitCountsTheDeckToo(t *testing.T) {
	limited, fs := limitedFS(t)
	ctx := context.Background()

	big := "<wk-section>" + strings.Repeat("s", limitBytes) + "</wk-section>"
	if _, err := limited.Create(ctx, Draft{Title: "Big deck", Deck: big}); !errors.Is(
		err, ErrTooLarge,
	) {
		t.Fatalf("create with an oversized deck: err = %v, want ErrTooLarge", err)
	}
	bigSrc := []byte(`{"sections":[{"h":"` + strings.Repeat("d", limitBytes) + `"}]}`)
	if _, err := limited.Create(ctx, Draft{
		Title: "Big deck source", Deck: "<wk-section>x</wk-section>", DeckSource: bigSrc,
	}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("create with an oversized deck source: err = %v, want ErrTooLarge", err)
	}

	p, err := limited.Create(ctx, Draft{Title: "T", Content: "<p>v1</p>"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := limited.Update(ctx, p.ID, Patch{Deck: &big}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("update adding an oversized deck: err = %v, want ErrTooLarge", err)
	}
	if got, _ := fs.Get(ctx, p.ID); got.HasDeck || got.Version != 1 {
		t.Fatalf("a refused update must not change the page: %+v", got)
	}
	if err := limited.SaveDeckSource(ctx, p.ID, bigSrc); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("SaveDeckSource over the limit: err = %v, want ErrTooLarge", err)
	}
	if _, err := fs.LoadDeckSource(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a refused SaveDeckSource must write nothing: err = %v", err)
	}

	small := []byte(`{"sections":[]}`)
	if err := limited.SaveDeckSource(ctx, p.ID, small); err != nil {
		t.Fatalf("SaveDeckSource under the limit: %v", err)
	}
	// The deck source now counts against the page's budget.
	almost := "<p>" + strings.Repeat("y", limitBytes-len(small)-64) + "</p>"
	if _, err := limited.Update(ctx, p.ID, Patch{Content: &almost}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("update that fits only without its deck source: err = %v, want ErrTooLarge", err)
	}
}
