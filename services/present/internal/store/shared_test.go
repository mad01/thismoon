package store

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"
)

func TestValidIDAcceptsOnlyMintedShapes(t *testing.T) {
	cases := map[string]bool{
		NewID():                            true,
		NewSharedID():                      true,
		"deadbeef00":                       true,
		"":                                 false,
		"nope":                             false,
		"../deadbeef00":                    false,
		"DEADBEEF00":                       false,
		"deadbeef0":                        false,
		"deadbeef000":                      false,
		"0123456789abcdef0123456789abcdef": true,
		"0123456789abcdef0123456789abcde":  false,
	}
	for id, want := range cases {
		if got := ValidID(id); got != want {
			t.Errorf("ValidID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestNewSharedIDIs32HexAndUnique(t *testing.T) {
	hex32 := regexp.MustCompile(`^[0-9a-f]{32}$`)
	seen := map[string]bool{}
	for range 100 {
		id := NewSharedID()
		if !hex32.MatchString(id) {
			t.Fatalf("NewSharedID() = %q, want 32 lowercase hex chars", id)
		}
		if seen[id] {
			t.Fatalf("NewSharedID() repeated %q", id)
		}
		seen[id] = true
	}
}

func TestInvalidIDIsNotFoundEverywhere(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	const bad = "../escape"
	if _, err := s.Get(ctx, bad); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get: err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, bad); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete: err = %v, want ErrNotFound", err)
	}
	if err := s.SaveDoc(ctx, bad, []byte(`{}`)); !errors.Is(err, ErrNotFound) {
		t.Errorf("SaveDoc: err = %v, want ErrNotFound", err)
	}
	if _, err := s.LoadDoc(ctx, bad); !errors.Is(err, ErrNotFound) {
		t.Errorf("LoadDoc: err = %v, want ErrNotFound", err)
	}
	if s.HasDoc(ctx, bad) {
		t.Error("HasDoc: true for an invalid id")
	}
}

func TestCreatePersistsSourcesFromDraft(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	doc := []byte(`{"sections":[]}`)
	graph := []byte(`{"nodes":[]}`)
	p, err := s.Create(ctx, Draft{
		Title: "T", Content: "<p>x</p>", Graph: "cy.init();",
		Doc: doc, GraphSource: graph,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !p.HasDoc {
		t.Error("Create returned HasDoc=false for a draft with a doc")
	}
	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.HasDoc {
		t.Error("Get: HasDoc=false after Create with a doc")
	}
	if b, err := s.LoadDoc(ctx, p.ID); err != nil || string(b) != string(doc) {
		t.Errorf("LoadDoc = %q, %v; want the draft doc", b, err)
	}
	if b, err := s.LoadGraphSource(ctx, p.ID); err != nil || string(b) != string(graph) {
		t.Errorf("LoadGraphSource = %q, %v; want the draft graph source", b, err)
	}
}

func TestCreateWithoutSourcesHasNone(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, err := s.Create(ctx, Draft{Title: "T", Content: "<p>x</p>"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.HasDoc || s.HasDoc(ctx, p.ID) || s.HasGraphSource(ctx, p.ID) {
		t.Error("a draft without sources must not persist any")
	}
}

func TestCreateKeepsAuthorAndExpiry(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	exp := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	p, err := s.Create(ctx, Draft{
		Title: "T", Content: "<p>x</p>", Author: "abc123", Ephemeral: true, ExpiresAt: &exp,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := s.Get(ctx, p.ID)
	if got.Author != "abc123" || !got.Ephemeral || got.ExpiresAt == nil ||
		!got.ExpiresAt.Equal(exp) {
		t.Errorf("round-trip lost author/expiry: %+v", got)
	}
}

func TestUpdateEphemeralPatchSetsAndClearsExpiry(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, _ := s.Create(ctx, Draft{Title: "T", Content: "<p>x</p>"})

	exp := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	on := true
	got, err := s.Update(ctx, p.ID, Patch{Ephemeral: &on, ExpiresAt: &exp})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !got.Ephemeral || got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) {
		t.Fatalf("ephemeral=true patch: %+v", got)
	}

	off := false
	got, err = s.Update(ctx, p.ID, Patch{Ephemeral: &off, ExpiresAt: &exp})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Ephemeral || got.ExpiresAt != nil {
		t.Fatalf("ephemeral=false patch must clear the expiry: %+v", got)
	}
	if got.Version != 3 {
		t.Fatalf("Version = %d, want 3 after two updates", got.Version)
	}
}

func TestSetSharedIsMetaOnly(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, _ := s.Create(ctx, Draft{Title: "T", Content: "<p>x</p>"})

	info := &SharedInfo{
		ID:       NewSharedID(),
		URL:      "https://example.test/p/x",
		SharedAt: time.Now().UTC(),
	}
	if err := s.SetShared(ctx, p.ID, info); err != nil {
		t.Fatalf("SetShared: %v", err)
	}
	got, _ := s.Get(ctx, p.ID)
	if got.Shared == nil || got.Shared.ID != info.ID || got.Shared.URL != info.URL {
		t.Fatalf("Shared not persisted: %+v", got.Shared)
	}
	if got.Version != p.Version {
		t.Fatalf("SetShared bumped the version: %d -> %d", p.Version, got.Version)
	}
	if got.Content != "<p>x</p>" {
		t.Fatalf("SetShared touched content: %q", got.Content)
	}

	if err := s.SetShared(ctx, p.ID, nil); err != nil {
		t.Fatalf("SetShared(nil): %v", err)
	}
	if got, _ := s.Get(ctx, p.ID); got.Shared != nil {
		t.Fatal("SetShared(nil) must clear the record")
	}
	if err := s.SetShared(ctx, "deadbeef00", info); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetShared on a missing page: err = %v, want ErrNotFound", err)
	}
}

func TestPing(t *testing.T) {
	s := newTestStore(t)
	if err := s.Ping(t.Context()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	broken := &FS{dir: t.TempDir() + "/missing", now: time.Now}
	if err := broken.Ping(t.Context()); err == nil {
		t.Fatal("Ping on a missing pages dir must fail")
	}
}

func TestPageExpired(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Second), now.Add(time.Second)
	if (Page{}).Expired(now) {
		t.Error("a page without expiry never expires")
	}
	if !(Page{ExpiresAt: &past}).Expired(now) {
		t.Error("past expiry must report expired")
	}
	if !(Page{ExpiresAt: &now}).Expired(now) {
		t.Error("expiry equal to now must report expired")
	}
	if (Page{ExpiresAt: &future}).Expired(now) {
		t.Error("future expiry must not report expired")
	}
}

func TestWithoutExpiredHidesExpiredPages(t *testing.T) {
	raw := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)

	live, _ := raw.Create(
		ctx,
		Draft{
			Title:     "Live",
			Content:   "<p>l</p>",
			Ephemeral: true,
			ExpiresAt: &future,
			Doc:       []byte(`{}`),
		},
	)
	dead, _ := raw.Create(
		ctx,
		Draft{
			Title:     "Dead",
			Content:   "<p>d</p>",
			Ephemeral: true,
			ExpiresAt: &past,
			Doc:       []byte(`{}`),
		},
	)
	forever, _ := raw.Create(ctx, Draft{Title: "Forever", Content: "<p>f</p>"})

	s := WithoutExpired(raw, func() time.Time { return now })

	if _, err := s.Get(ctx, live.ID); err != nil {
		t.Errorf("Get live: %v", err)
	}
	if _, err := s.Get(ctx, forever.ID); err != nil {
		t.Errorf("Get forever: %v", err)
	}
	if _, err := s.Get(ctx, dead.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get dead: err = %v, want ErrNotFound", err)
	}
	title := "x"
	if _, err := s.Update(ctx, dead.ID, Patch{Title: &title}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update dead: err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, dead.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete dead: err = %v, want ErrNotFound", err)
	}
	if s.HasDoc(ctx, dead.ID) {
		t.Error("HasDoc dead: true, want false")
	}
	if !s.HasDoc(ctx, live.ID) {
		t.Error("HasDoc live: false, want true")
	}
	if _, err := s.LoadDoc(ctx, dead.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("LoadDoc dead: err = %v, want ErrNotFound", err)
	}
	if err := s.SetShared(ctx, dead.ID, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetShared dead: err = %v, want ErrNotFound", err)
	}

	for name, list := range map[string]func(context.Context) ([]Page, error){
		"List": s.List, "ListMeta": s.ListMeta,
	} {
		pages, err := list(ctx)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(pages) != 2 {
			t.Fatalf("%s returned %d pages, want 2 (expired dropped)", name, len(pages))
		}
		for _, p := range pages {
			if p.ID == dead.ID {
				t.Fatalf("%s returned the expired page", name)
			}
		}
	}

	// The raw store still holds the expired page: that is the sweeper's job.
	if _, err := raw.Get(ctx, dead.ID); err != nil {
		t.Errorf("raw Get dead: %v (the wrapper must not delete)", err)
	}
}
