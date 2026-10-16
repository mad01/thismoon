package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApplyEditKeepsOnlyTheEditableFields(t *testing.T) {
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	now := created.Add(time.Hour)
	exp := now.Add(time.Hour)
	shared := &SharedInfo{ID: "s", URL: "https://s"}
	cur := Snapshot{
		Page: Page{
			ID: "abcdef0123", Title: "T", Content: "<p>v1</p>", Version: 3, Author: "a",
			CreatedAt: created, UpdatedAt: created, Shared: shared,
		},
		Sources: Sources{Doc: []byte(`{}`)},
	}
	next, err := ApplyEdit(cur, func(s Snapshot) (Snapshot, error) {
		s.Page = Page{
			ID: "other", Title: "T2", Deck: "<wk-section>d</wk-section>", Graph: "cy();",
			References: []Reference{{Title: "r", URL: "https://r"}},
			Version:    99, Author: "b", Ephemeral: true, ExpiresAt: &exp,
		}
		s.Sources = Sources{Deck: []byte(`{"d":1}`)}
		return s, nil
	}, now)
	if err != nil {
		t.Fatalf("ApplyEdit: %v", err)
	}
	p := next.Page
	if p.ID != "abcdef0123" || p.Version != 4 || p.Author != "a" || p.Shared != shared ||
		!p.CreatedAt.Equal(created) || !p.UpdatedAt.Equal(now) {
		t.Errorf("store-owned fields moved: %+v", p)
	}
	if p.Title != "T2" || p.Content != "" || p.HasBrief || !p.HasDeck || !p.HasGraph ||
		!p.HasRefs || !p.Ephemeral || p.ExpiresAt != &exp {
		t.Errorf("editable fields not taken: %+v", p)
	}
	if p.HasDoc || string(next.Sources.Deck) != `{"d":1}` {
		t.Errorf("sources not taken: has_doc %v, deck %s", p.HasDoc, next.Sources.Deck)
	}
}

// TestEditLandsSourcesBeforeTheVersion breaks the final meta.json write
// from inside the edit, the way a crash before it would, and finds the
// sources and renditions already on disk under the old version. The next
// edit then builds on them.
func TestEditLandsSourcesBeforeTheVersion(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, err := s.Create(ctx, Draft{Title: "T", Content: "<p>v1</p>", Doc: []byte(`{"v":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(s.pageDir(p.ID), metaFile)
	oldMeta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Edit(ctx, p.ID, func(cur Snapshot) (Snapshot, error) {
		// The page is already read; a directory in meta.json's place makes
		// the rename that would replace it fail.
		if err := os.Remove(metaPath); err != nil {
			return Snapshot{}, err
		}
		if err := os.Mkdir(metaPath, 0o755); err != nil {
			return Snapshot{}, err
		}
		cur.Page.Content, cur.Sources.Doc = "<p>v2</p>", []byte(`{"v":2}`)
		return cur, nil
	})
	if err == nil {
		t.Fatal("Edit succeeded without writing meta.json")
	}
	if err := os.Remove(metaPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, oldMeta, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := s.LoadDoc(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.Content != "<p>v2</p>" || string(doc) != `{"v":2}` {
		t.Fatalf("after the broken write: version %d content %q doc %s; "+
			"want version 1 over the new content and doc", got.Version, got.Content, doc)
	}
	next, err := s.Edit(ctx, p.ID, PatchEdit(Patch{}))
	if err != nil || next.Version != 2 || next.Content != "<p>v2</p>" {
		t.Fatalf("next edit = %+v, %v; want version 2 over the new content", next, err)
	}
}

// TestWritesNeverRecreateADeletedPage pins the page lock's refusal: a write
// that arrives after Delete finds no page and leaves no directory behind.
func TestWritesNeverRecreateADeletedPage(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, err := s.Create(ctx, Draft{Title: "T", Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	title := "x"
	writes := map[string]error{
		"Edit": func() error {
			_, err := s.Edit(ctx, p.ID, PatchEdit(Patch{Title: &title}))
			return err
		}(),
		"SaveDoc":   s.SaveDoc(ctx, p.ID, []byte(`{}`)),
		"DeleteDoc": s.DeleteDoc(ctx, p.ID),
		"SetShared": s.SetShared(ctx, p.ID, nil),
	}
	for name, err := range writes {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s after Delete: err = %v, want ErrNotFound", name, err)
		}
	}
	if _, err := os.Stat(s.pageDir(p.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("page dir after writes to a deleted page: err = %v, want it gone", err)
	}
}

// TestSizeLimitMeasuresAnEdit pins the wrapper's Edit: an edit growing the
// content or a source past the cap is refused with the page untouched.
func TestSizeLimitMeasuresAnEdit(t *testing.T) {
	limited, fs := limitedFS(t)
	ctx := t.Context()
	p, err := limited.Create(ctx, Draft{Title: "T", Content: "<p>v1</p>"})
	if err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", limitBytes)
	for name, grow := range map[string]EditFunc{
		"content": func(cur Snapshot) (Snapshot, error) {
			cur.Page.Content = big
			return cur, nil
		},
		"source": func(cur Snapshot) (Snapshot, error) {
			cur.Sources.Doc = []byte(`"` + big + `"`)
			return cur, nil
		},
	} {
		if _, err := limited.Edit(ctx, p.ID, grow); !errors.Is(err, ErrTooLarge) {
			t.Errorf("edit growing the %s: err = %v, want ErrTooLarge", name, err)
		}
	}
	got, err := fs.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.Content != "<p>v1</p>" || got.HasDoc {
		t.Fatalf("page after refused edits = %+v, want the original", got)
	}
	if _, err := limited.Edit(ctx, p.ID, PatchEdit(Patch{})); err != nil {
		t.Fatalf("edit under the limit: %v", err)
	}
}

// TestWithoutExpiredRefusesAnEdit pins the wrapper's Edit: an expired page
// is not found, the edit never runs, and the raw page keeps its version.
func TestWithoutExpiredRefusesAnEdit(t *testing.T) {
	raw := newTestStore(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	dead, err := raw.Create(
		ctx, Draft{Title: "Dead", Content: "<p>d</p>", Ephemeral: true, ExpiresAt: &past},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := WithoutExpired(raw, func() time.Time { return now })
	ran := false
	_, err = s.Edit(ctx, dead.ID, func(cur Snapshot) (Snapshot, error) {
		ran = true
		return cur, nil
	})
	if !errors.Is(err, ErrNotFound) || ran {
		t.Fatalf("Edit expired: err = %v, ran %v; want ErrNotFound without running", err, ran)
	}
	if got, err := raw.Get(ctx, dead.ID); err != nil || got.Version != 1 {
		t.Fatalf("raw page after the refused edit = v%d, %v; want v1", got.Version, err)
	}
}
