package server

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/present/internal/images"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// storeImage writes a small PNG with the given pixel into dir's image
// store and returns its stored name.
func pngData(t *testing.T, shade uint8) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 2, 2))
	img.Pix[0] = shade
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func storeImage(t *testing.T, dir string, shade uint8) string {
	t.Helper()
	data := pngData(t, shade)
	name := images.Name(data, "png")
	if err := images.New(dir).Write(name, data); err != nil {
		t.Fatal(err)
	}
	// Written a while ago, as far as the sweep's grace period is concerned.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "images", name), old, old); err != nil {
		t.Fatal(err)
	}
	return name
}

func setupWithWorkdir(t *testing.T) (*httptest.Server, store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(st, Options{Workdir: dir, Info: testInfo}).Handler())
	t.Cleanup(ts.Close)
	return ts, st, dir
}

// A stored image is served under its name with its content type and the
// immutable cache header the vendored assets get, since its name is its
// content's hash.
func TestImageIsServedImmutable(t *testing.T) {
	ts, _, dir := setupWithWorkdir(t)
	name := storeImage(t, dir, 10)
	resp, err := http.Get(ts.URL + images.Served(name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != immutableCache {
		t.Errorf("Cache-Control = %q, want %q", cc, immutableCache)
	}
	if v := resp.Header.Get("X-Content-Type-Options"); v != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", v)
	}
	want, _ := os.ReadFile(filepath.Join(dir, "images", name))
	if !bytes.Equal(body, want) {
		t.Error("served bytes differ from the stored file")
	}
}

// Anything but a stored name is a 404: an unknown hash, a name of the
// wrong shape, a traversal, and the route in shared mode, which has no
// store at all.
func TestImageUnknownOrMalformedIs404(t *testing.T) {
	ts, _, dir := setupWithWorkdir(t)
	name := storeImage(t, dir, 10)
	// A file that is not a stored name must not be reachable through the
	// route either, however it is spelled.
	if err := os.WriteFile(filepath.Join(dir, "images", "secret.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		"/img/" + strings.Repeat("00", 32) + ".png",
		"/img/secret.png",
		"/img/" + strings.ToUpper(name),
		"/img/" + strings.TrimSuffix(name, ".png") + ".svg",
		"/img/../pages/x",
		"/img/..%2Fimages%2F" + name,
		"/img/",
	} {
		code, _ := get(t, ts.URL+p)
		if code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, code)
		}
	}

	shared := httptest.NewServer(
		New(mustStore(t, dir), Options{Mode: ModeShared, Workdir: dir, Info: testInfo}).Handler(),
	)
	t.Cleanup(shared.Close)
	if code, _ := get(t, shared.URL+images.Served(name)); code != http.StatusNotFound {
		t.Errorf("shared mode serves /img/: %d, want 404", code)
	}
}

func mustStore(t *testing.T, dir string) store.Store {
	t.Helper()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// Deleting a page removes the stored images no other page references and
// keeps the ones still in use, whichever rendition uses them.
func TestDeleteSweepsUnreferencedImages(t *testing.T) {
	ts, st, dir := setupWithWorkdir(t)
	shared := storeImage(t, dir, 10)
	only := storeImage(t, dir, 20)
	inDeck := storeImage(t, dir, 30)
	inSource := storeImage(t, dir, 40)
	tag := func(name string) string { return `<img src="` + images.Served(name) + `" alt="a">` }
	keeper, _ := st.Create(t.Context(), store.Draft{Title: "K", Content: tag(shared)})
	deckPage, _ := st.Create(
		t.Context(),
		store.Draft{Title: "D", Content: "<p>x</p>", Deck: tag(inDeck)},
	)
	// A reference held only by a Doc source keeps its file too.
	srcPage, _ := st.Create(t.Context(), store.Draft{
		Title: "S", Content: "<p>x</p>",
		Doc: []byte(
			`{"sections":[{"h":"S","blocks":[{"t":"image","src":"` + images.Served(
				inSource,
			) + `","alt":"a"}]}]}`,
		),
	})
	doomed, _ := st.Create(t.Context(), store.Draft{Title: "X", Content: tag(shared) + tag(only)})

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/p/"+doomed.ID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	for name, want := range map[string]bool{shared: true, inDeck: true, inSource: true, only: false} {
		_, err := os.Stat(filepath.Join(dir, "images", name))
		if exists := err == nil; exists != want {
			t.Errorf("%s exists = %v, want %v", name, exists, want)
		}
	}
	for _, id := range []string{keeper.ID, deckPage.ID, srcPage.ID} {
		if _, err := st.Get(t.Context(), id); err != nil {
			t.Errorf("page %s gone after the sweep: %v", id, err)
		}
	}
}

// SweepImages is what serve runs at startup: it removes the stale images no
// page references and leaves a fresh one alone, whoever wrote it.
func TestSweepImagesAtStartup(t *testing.T) {
	dir := t.TempDir()
	st := mustStore(t, dir)
	stale := storeImage(t, dir, 10)
	freshData := pngData(t, 50)
	fresh := images.Name(freshData, "png")
	if err := images.New(dir).Write(fresh, freshData); err != nil {
		t.Fatal(err)
	}
	New(st, Options{Workdir: dir, Info: testInfo}).SweepImages(t.Context())
	if _, err := os.Stat(filepath.Join(dir, "images", stale)); err == nil {
		t.Error("stale unreferenced image survived the startup sweep")
	}
	if _, err := os.Stat(filepath.Join(dir, "images", fresh)); err != nil {
		t.Errorf("fresh image removed by the startup sweep: %v", err)
	}
	// A shared server has no store and nothing to sweep.
	New(st, Options{Mode: ModeShared, Workdir: dir, Info: testInfo}).SweepImages(t.Context())
}
