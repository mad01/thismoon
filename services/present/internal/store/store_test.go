package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *FS {
	t.Helper()
	s, err := NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestCreatePersistsAndReturnsID(t *testing.T) {
	s := newTestStore(t)
	p, err := s.Create(t.Context(), Draft{Title: "My Brief", Content: "<p>body</p>"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == "" {
		t.Fatal("Create returned empty id")
	}
	if p.Version != 1 {
		t.Fatalf("Version = %d, want 1", p.Version)
	}
	// meta.json and content.html must exist on disk.
	dir := s.pageDir(p.ID)
	for _, f := range []string{metaFile, contentFile} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("expected %s on disk: %v", f, err)
		}
	}
	// No graph -> no graph.js file.
	if _, err := os.Stat(filepath.Join(dir, graphFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("graph.js should not exist when no graph provided")
	}
}

func TestDeleteRemovesPage(t *testing.T) {
	s := newTestStore(t)
	p, err := s.Create(t.Context(), Draft{Title: "Doomed", Content: "<p>body</p>"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Delete(t.Context(), p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(s.pageDir(p.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("page dir still exists after delete: %v", err)
	}
	if _, err := s.Get(t.Context(), p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.Delete(t.Context(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete = %v, want ErrNotFound", err)
	}
}

func TestGetRoundTrips(t *testing.T) {
	s := newTestStore(t)
	created, err := s.Create(
		t.Context(),
		Draft{Title: "Title", Content: "<p>hello</p>", Graph: "cy.init();"},
	)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "Title" || got.Content != "<p>hello</p>" || got.Graph != "cy.init();" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if !got.HasGraph {
		t.Fatal("HasGraph = false, want true")
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Get(t.Context(), "deadbeef00"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get unknown id: err = %v, want ErrNotFound", err)
	}
}

func TestUpdatePatchesAndBumpsVersion(t *testing.T) {
	s := newTestStore(t)
	created, _ := s.Create(t.Context(), Draft{Title: "Original", Content: "<p>v1</p>"})

	newContent := "<p>v2</p>"
	got, err := s.Update(t.Context(), created.ID, Patch{Content: &newContent})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Content != newContent {
		t.Fatalf("Content = %q, want %q", got.Content, newContent)
	}
	if got.Title != "Original" {
		t.Fatalf("Title = %q, want unchanged Original", got.Title)
	}
	if got.Version != 2 {
		t.Fatalf("Version = %d, want 2", got.Version)
	}
	if !got.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("CreatedAt changed: %v -> %v", created.CreatedAt, got.CreatedAt)
	}
	// Persisted, not just returned.
	reloaded, _ := s.Get(t.Context(), created.ID)
	if reloaded.Version != 2 || reloaded.Content != newContent {
		t.Fatalf("update not persisted: %+v", reloaded)
	}
}

func TestUpdateClearsGraphWithEmptyString(t *testing.T) {
	s := newTestStore(t)
	created, _ := s.Create(t.Context(), Draft{Title: "T", Content: "<p>x</p>", Graph: "cy.init();"})
	empty := ""
	got, err := s.Update(t.Context(), created.ID, Patch{Graph: &empty})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.HasGraph || got.Graph != "" {
		t.Fatalf("graph not cleared: HasGraph=%v Graph=%q", got.HasGraph, got.Graph)
	}
}

func TestUpdateNotFound(t *testing.T) {
	s := newTestStore(t)
	title := "x"
	if _, err := s.Update(t.Context(), "nope000000", Patch{Title: &title}); !errors.Is(
		err,
		ErrNotFound,
	) {
		t.Fatalf("Update unknown id: err = %v, want ErrNotFound", err)
	}
}

func TestListSortedByUpdatedDesc(t *testing.T) {
	s := newTestStore(t)
	// Inject a controllable clock for deterministic ordering.
	base := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	tick := 0
	s.now = func() time.Time { tick++; return base.Add(time.Duration(tick) * time.Minute) }

	a, _ := s.Create(t.Context(), Draft{Title: "A", Content: "<p>a</p>"}) // t+1
	b, _ := s.Create(t.Context(), Draft{Title: "B", Content: "<p>b</p>"}) // t+2
	c, _ := s.Create(t.Context(), Draft{Title: "C", Content: "<p>c</p>"}) // t+3

	pages, err := s.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pages) != 3 {
		t.Fatalf("List returned %d pages, want 3", len(pages))
	}
	wantOrder := []string{c.ID, b.ID, a.ID}
	for i, want := range wantOrder {
		if pages[i].ID != want {
			t.Fatalf("List[%d].ID = %q, want %q (order: %v)", i, pages[i].ID, want, wantOrder)
		}
	}
}

func TestCreateWithReferences(t *testing.T) {
	s := newTestStore(t)
	refs := []Reference{
		{Title: "dotfiles repo", URL: "https://github.com/mad01/dotfiles"},
		{Title: "Go docs", URL: "https://pkg.go.dev"},
	}
	p, err := s.Create(
		t.Context(),
		Draft{Title: "With Refs", Content: "<p>body</p>", References: refs},
	)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !p.HasRefs || len(p.References) != 2 {
		t.Fatalf("HasRefs=%v Refs=%d, want true/2", p.HasRefs, len(p.References))
	}
	dir := s.pageDir(p.ID)
	if _, err := os.Stat(filepath.Join(dir, refsFile)); err != nil {
		t.Fatalf("refs.json should exist: %v", err)
	}
	got, _ := s.Get(t.Context(), p.ID)
	if len(got.References) != 2 || got.References[0].Title != "dotfiles repo" {
		t.Fatalf("round-trip refs mismatch: %+v", got.References)
	}
}

func TestUpdatePatchesReferences(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.Create(t.Context(), Draft{Title: "T", Content: "<p>x</p>"})
	if p.HasRefs {
		t.Fatal("HasRefs should be false initially")
	}
	refs := []Reference{{Title: "PR #1", URL: "https://github.com/mad01/dotfiles/pull/1"}}
	got, err := s.Update(t.Context(), p.ID, Patch{References: &refs})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !got.HasRefs || len(got.References) != 1 {
		t.Fatalf("refs not patched: HasRefs=%v len=%d", got.HasRefs, len(got.References))
	}
	empty := []Reference{}
	got2, _ := s.Update(t.Context(), p.ID, Patch{References: &empty})
	if got2.HasRefs || len(got2.References) != 0 {
		t.Fatalf("refs not cleared: HasRefs=%v len=%d", got2.HasRefs, len(got2.References))
	}
}

func TestListEmpty(t *testing.T) {
	s := newTestStore(t)
	pages, err := s.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pages) != 0 {
		t.Fatalf("List on empty store = %d, want 0", len(pages))
	}
}
