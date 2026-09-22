package k8sstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// fixture is a store under test with the clock the test controls and the
// raw *Store for sweeper access.
type fixture struct {
	st  *Store
	now time.Time
}

// runConformance exercises the store.Store contract plus the sweeper
// against any Store; the fake client and the kind cluster both run it.
func runConformance(t *testing.T, newFixture func(t *testing.T) *fixture) {
	t.Helper()
	t.Run("CreateGetRoundTrip", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()
		exp := f.now.Add(time.Hour)
		p, err := f.st.Create(ctx, store.Draft{
			Title: "T", Content: "<p>x</p>", Graph: "cy.init();",
			References:  []store.Reference{{Title: "r", URL: "https://r"}},
			Doc:         []byte(`{"sections":[]}`),
			GraphSource: []byte(`{"nodes":[]}`),
			Author:      "abc", Ephemeral: true, ExpiresAt: &exp,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if len(p.ID) != 32 {
			t.Fatalf("id = %q, want a 32-hex shared id", p.ID)
		}
		got, err := f.st.Get(ctx, p.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Title != "T" || got.Content != "<p>x</p>" || got.Graph != "cy.init();" ||
			!got.HasGraph || !got.HasRefs || !got.HasDoc || got.Version != 1 ||
			got.Author != "abc" || !got.Ephemeral || got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) ||
			len(got.References) != 1 || got.References[0].URL != "https://r" {
			t.Fatalf("round-trip mismatch: %+v", got)
		}
		if !got.CreatedAt.Equal(f.now) || !got.UpdatedAt.Equal(f.now) {
			t.Fatalf("timestamps = %v/%v, want %v", got.CreatedAt, got.UpdatedAt, f.now)
		}
		if doc, err := f.st.LoadDoc(ctx, p.ID); err != nil || string(doc) != `{"sections":[]}` {
			t.Fatalf("LoadDoc = %q, %v", doc, err)
		}
		if src, err := f.st.LoadGraphSource(ctx, p.ID); err != nil || string(src) != `{"nodes":[]}` {
			t.Fatalf("LoadGraphSource = %q, %v", src, err)
		}
	})

	t.Run("CallerChosenID", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()
		id := store.NewSharedID()
		p, err := f.st.Create(ctx, store.Draft{ID: id, Title: "T", Content: "<p>x</p>"})
		if err != nil || p.ID != id {
			t.Fatalf("Create with id: %+v, %v", p, err)
		}
		if _, err := f.st.Create(ctx, store.Draft{ID: "../bad", Title: "T", Content: "x"}); err == nil {
			t.Fatal("Create with an invalid id must fail")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()
		for _, id := range []string{store.NewSharedID(), "not-an-id", "../x"} {
			if _, err := f.st.Get(ctx, id); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("Get(%q): %v, want ErrNotFound", id, err)
			}
			if err := f.st.Delete(ctx, id); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("Delete(%q): %v, want ErrNotFound", id, err)
			}
			title := "x"
			if _, err := f.st.Update(ctx, id, store.Patch{Title: &title}); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("Update(%q): %v, want ErrNotFound", id, err)
			}
			if err := f.st.SetShared(ctx, id, nil); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("SetShared(%q): %v, want ErrNotFound", id, err)
			}
			if f.st.HasDoc(ctx, id) {
				t.Errorf("HasDoc(%q) = true", id)
			}
		}
	})

	t.Run("UpdateBumpsVersionSourcesDoNot", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()
		p, _ := f.st.Create(ctx, store.Draft{Title: "T", Content: "<p>v1</p>"})
		f.now = f.now.Add(time.Minute)
		content := "<p>v2</p>"
		on := true
		exp := f.now.Add(time.Hour)
		got, err := f.st.Update(ctx, p.ID, store.Patch{Content: &content, Ephemeral: &on, ExpiresAt: &exp})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if got.Version != 2 || got.Content != content || !got.Ephemeral || got.ExpiresAt == nil || !got.UpdatedAt.Equal(f.now) {
			t.Fatalf("after update: %+v", got)
		}
		if err := f.st.SaveDoc(ctx, p.ID, []byte(`{}`)); err != nil {
			t.Fatalf("SaveDoc: %v", err)
		}
		if err := f.st.SaveGraphSource(ctx, p.ID, []byte(`{}`)); err != nil {
			t.Fatalf("SaveGraphSource: %v", err)
		}
		got, _ = f.st.Get(ctx, p.ID)
		if got.Version != 2 || !got.HasDoc || !f.st.HasGraphSource(ctx, p.ID) {
			t.Fatalf("after source saves: %+v", got)
		}
		if err := f.st.DeleteDoc(ctx, p.ID); err != nil {
			t.Fatalf("DeleteDoc: %v", err)
		}
		if err := f.st.DeleteGraphSource(ctx, p.ID); err != nil {
			t.Fatalf("DeleteGraphSource: %v", err)
		}
		if f.st.HasDoc(ctx, p.ID) || f.st.HasGraphSource(ctx, p.ID) {
			t.Fatal("sources must be gone after delete")
		}
		if _, err := f.st.LoadDoc(ctx, p.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("LoadDoc after delete: %v, want ErrNotFound", err)
		}
		off := false
		got, _ = f.st.Update(ctx, p.ID, store.Patch{Ephemeral: &off})
		if got.Ephemeral || got.ExpiresAt != nil || got.Version != 3 {
			t.Fatalf("ephemeral=false patch: %+v", got)
		}
	})

	t.Run("SetSharedIsMetaOnly", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()
		p, _ := f.st.Create(ctx, store.Draft{Title: "T", Content: "<p>x</p>"})
		exp := f.now.Add(time.Hour)
		info := &store.SharedInfo{ID: store.NewSharedID(), URL: "https://s/p/x", Ephemeral: true, ExpiresAt: &exp, SharedAt: f.now}
		if err := f.st.SetShared(ctx, p.ID, info); err != nil {
			t.Fatalf("SetShared: %v", err)
		}
		got, _ := f.st.Get(ctx, p.ID)
		if got.Shared == nil || got.Shared.ID != info.ID || got.Shared.URL != info.URL ||
			!got.Shared.Ephemeral || got.Shared.ExpiresAt == nil || !got.Shared.ExpiresAt.Equal(exp) ||
			!got.Shared.SharedAt.Equal(f.now) || got.Version != 1 {
			t.Fatalf("shared record: %+v version %d", got.Shared, got.Version)
		}
		if err := f.st.SetShared(ctx, p.ID, nil); err != nil {
			t.Fatalf("SetShared(nil): %v", err)
		}
		if got, _ := f.st.Get(ctx, p.ID); got.Shared != nil {
			t.Fatal("SetShared(nil) must clear")
		}
	})

	t.Run("DeleteAndList", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()
		a, _ := f.st.Create(ctx, store.Draft{Title: "A", Content: "<p>a</p>"})
		f.now = f.now.Add(time.Second)
		b, _ := f.st.Create(ctx, store.Draft{Title: "B", Content: "<p>b</p>"})
		pages, err := f.st.List(ctx)
		if err != nil || len(pages) != 2 || pages[0].ID != b.ID || pages[1].ID != a.ID {
			t.Fatalf("List = %+v, %v; want B then A", pages, err)
		}
		metas, err := f.st.ListMeta(ctx)
		if err != nil || len(metas) != 2 || metas[0].Content != "" || metas[0].Title != "B" {
			t.Fatalf("ListMeta = %+v, %v", metas, err)
		}
		if err := f.st.Delete(ctx, a.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := f.st.Get(ctx, a.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("Get after delete: %v", err)
		}
		if err := f.st.Ping(ctx); err != nil {
			t.Fatalf("Ping: %v", err)
		}
	})

	t.Run("TooLarge", func(t *testing.T) {
		f := newFixture(t)
		f.st.maxBytes = 512
		ctx := context.Background()
		big := make([]byte, 600)
		for i := range big {
			big[i] = 'x'
		}
		if _, err := f.st.Create(ctx, store.Draft{Title: "Big", Content: string(big)}); !errors.Is(err, store.ErrTooLarge) {
			t.Fatalf("Create over cap: %v, want ErrTooLarge", err)
		}
		p, err := f.st.Create(ctx, store.Draft{Title: "Small", Content: "<p>x</p>"})
		if err != nil {
			t.Fatal(err)
		}
		content := string(big)
		if _, err := f.st.Update(ctx, p.ID, store.Patch{Content: &content}); !errors.Is(err, store.ErrTooLarge) {
			t.Fatalf("Update over cap: %v, want ErrTooLarge", err)
		}
		if got, _ := f.st.Get(ctx, p.ID); got.Version != 1 {
			t.Fatal("a refused update must not change the page")
		}
	})

	t.Run("SweepDeletesOnlyExpiredEphemeral", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()
		past, future := f.now.Add(-time.Hour), f.now.Add(time.Hour)
		dead, _ := f.st.Create(ctx, store.Draft{Title: "Dead", Content: "x", Ephemeral: true, ExpiresAt: &past})
		live, _ := f.st.Create(ctx, store.Draft{Title: "Live", Content: "x", Ephemeral: true, ExpiresAt: &future})
		forever, _ := f.st.Create(ctx, store.Draft{Title: "Forever", Content: "x"})
		n, err := f.st.SweepExpired(ctx, f.now)
		if err != nil || n != 1 {
			t.Fatalf("SweepExpired = %d, %v; want 1", n, err)
		}
		if _, err := f.st.Get(ctx, dead.ID); !errors.Is(err, store.ErrNotFound) {
			t.Error("expired page survived the sweep")
		}
		for _, id := range []string{live.ID, forever.ID} {
			if _, err := f.st.Get(ctx, id); err != nil {
				t.Errorf("sweep removed a live page %s: %v", id, err)
			}
		}
		if n, err := f.st.SweepExpired(ctx, f.now); err != nil || n != 0 {
			t.Fatalf("second sweep = %d, %v; want 0", n, err)
		}
	})
}

// TestDeployCRDMatchesEmbedded keeps deploy/base/crd.yaml a byte-identical
// copy of the embedded definition, so the manifests can never install a
// schema the binary was not built against.
func TestDeployCRDMatchesEmbedded(t *testing.T) {
	deployed, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "base", "crd.yaml"))
	if err != nil {
		t.Fatalf("read deploy/base/crd.yaml: %v", err)
	}
	if string(deployed) != string(CRD) {
		t.Fatal("deploy/base/crd.yaml differs from internal/store/k8sstore/crd.yaml; copy the embedded file over")
	}
}
