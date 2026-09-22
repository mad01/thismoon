package sharedclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// fakeShared is a minimal shared instance: create, replace, delete, whoami,
// with one accepted key, so client and orchestration can be tested without
// the server package.
type fakeShared struct {
	mu    sync.Mutex
	key   string
	pages map[string]Bundle
	seq   int
	ts    *httptest.Server
}

func newFakeShared(t *testing.T, key string) *fakeShared {
	t.Helper()
	f := &fakeShared{key: key, pages: map[string]Bundle{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/pages", f.create)
	mux.HandleFunc("PUT /api/p/{id}", f.replace)
	mux.HandleFunc("DELETE /p/{id}", f.remove)
	mux.HandleFunc("GET /api/whoami", f.whoami)
	f.ts = httptest.NewServer(mux)
	t.Cleanup(f.ts.Close)
	return f
}

func (f *fakeShared) authorized(w http.ResponseWriter, r *http.Request) bool {
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	switch {
	case got == "":
		http.Error(w, "authorization required", http.StatusUnauthorized)
		return false
	case got != f.key:
		http.Error(w, "page belongs to another author", http.StatusForbidden)
		return false
	}
	return true
}

func (f *fakeShared) reply(w http.ResponseWriter, status int, id string, b Bundle) {
	var exp *time.Time
	if b.Ephemeral {
		t := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
		exp = &t
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Result{ID: id, URL: f.ts.URL + "/p/" + id, Version: 1, Ephemeral: b.Ephemeral, ExpiresAt: exp})
}

func (f *fakeShared) create(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(w, r) {
		return
	}
	var b Bundle
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.seq++
	id := strings.Repeat("a", 31) + string(rune('0'+f.seq))
	f.pages[id] = b
	f.mu.Unlock()
	f.reply(w, http.StatusCreated, id, b)
}

func (f *fakeShared) replace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	f.mu.Lock()
	_, ok := f.pages[id]
	f.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !f.authorized(w, r) {
		return
	}
	var b Bundle
	_ = json.NewDecoder(r.Body).Decode(&b)
	f.mu.Lock()
	f.pages[id] = b
	f.mu.Unlock()
	f.reply(w, http.StatusOK, id, b)
}

func (f *fakeShared) remove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	f.mu.Lock()
	_, ok := f.pages[id]
	f.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !f.authorized(w, r) {
		return
	}
	f.mu.Lock()
	delete(f.pages, id)
	f.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeShared) whoami(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(w, r) {
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"author": "hash-of-" + f.key})
}

func TestStatusMapping(t *testing.T) {
	f := newFakeShared(t, "right")
	ctx := context.Background()
	b := Bundle{Title: "T", Content: "<p>x</p>"}

	if _, err := New(f.ts.URL, "").Create(ctx, b); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("no key: %v, want ErrUnauthorized", err)
	}
	res, err := New(f.ts.URL, "right").Create(ctx, b)
	if err != nil || res.ID == "" || !strings.HasSuffix(res.URL, "/p/"+res.ID) {
		t.Fatalf("create: %+v, %v", res, err)
	}
	if _, err := New(f.ts.URL, "wrong").Replace(ctx, res.ID, b); !errors.Is(err, ErrForbidden) {
		t.Errorf("other key: %v, want ErrForbidden", err)
	}
	if _, err := New(f.ts.URL, "right").Replace(ctx, strings.Repeat("f", 32), b); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v, want ErrNotFound", err)
	}
	if who, err := New(f.ts.URL, "right").WhoAmI(ctx); err != nil || who != "hash-of-right" {
		t.Errorf("whoami: %q, %v", who, err)
	}
	if err := New(f.ts.URL, "right").Delete(ctx, res.ID); err != nil {
		t.Errorf("delete: %v", err)
	}
	if err := New(f.ts.URL, "right").Delete(ctx, res.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete again: %v, want ErrNotFound", err)
	}
}

func TestShareCreatesThenReplacesAndRecovers(t *testing.T) {
	f := newFakeShared(t, "k")
	c := New(f.ts.URL, "k")
	st, err := store.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	p, _ := st.Create(ctx, store.Draft{Title: "Local", Content: "<p>l</p>", Doc: []byte(`{"sections":[]}`)})

	info, err := Share(ctx, st, c, p.ID, true, now)
	if err != nil {
		t.Fatalf("first share: %v", err)
	}
	if !info.Ephemeral || info.ExpiresAt == nil || !info.SharedAt.Equal(now) || info.URL == "" {
		t.Fatalf("first share info: %+v", info)
	}
	got, _ := st.Get(ctx, p.ID)
	if got.Shared == nil || got.Shared.ID != info.ID {
		t.Fatalf("local record not written: %+v", got.Shared)
	}
	if pushed := f.pages[info.ID]; string(pushed.Doc) != `{"sections":[]}` || pushed.Title != "Local" {
		t.Fatalf("pushed bundle lost content: %+v", pushed)
	}

	again, err := Share(ctx, st, c, p.ID, false, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("second share: %v", err)
	}
	if again.ID != info.ID || again.Ephemeral || again.ExpiresAt != nil {
		t.Fatalf("re-share must replace under the same id and honour the new flag: %+v", again)
	}

	// The instance purged the copy: sharing again creates a new one.
	delete(f.pages, info.ID)
	fresh, err := Share(ctx, st, c, p.ID, false, now)
	if err != nil {
		t.Fatalf("share after purge: %v", err)
	}
	if fresh.ID == info.ID {
		t.Fatal("share after purge must create a new copy")
	}

	if err := Unshare(ctx, st, c, p.ID); err != nil {
		t.Fatalf("unshare: %v", err)
	}
	if got, _ := st.Get(ctx, p.ID); got.Shared != nil {
		t.Fatal("unshare must clear the local record")
	}
	if _, ok := f.pages[fresh.ID]; ok {
		t.Fatal("unshare must delete the remote copy")
	}
	if err := Unshare(ctx, st, c, p.ID); err != nil {
		t.Fatalf("unshare of an unshared page must be a no-op: %v", err)
	}
	if _, err := Share(ctx, st, c, "deadbeef00", false, now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("share of a missing local page: %v, want ErrNotFound", err)
	}
}
