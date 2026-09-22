package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/sharedclient"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// localFixture is a local-mode server whose Share button points at a
// shared-mode server, both over their own filesystem stores.
type localFixture struct {
	ts     *httptest.Server
	st     *store.FS
	dir    string
	shared *sharedFixture
}

// setupLocalSharing builds a shared server, then a local server whose Sharer
// signs with key against it.
func setupLocalSharing(t *testing.T, key string) *localFixture {
	t.Helper()
	shared := setupShared(t)
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatalf("store.NewFS: %v", err)
	}
	f := &localFixture{st: st, dir: dir, shared: shared}
	f.ts = newLocalServer(t, st, dir, sharedclient.New(shared.ts.URL, key))
	return f
}

func newLocalServer(
	t *testing.T,
	st store.Store,
	dir string,
	c *sharedclient.Client,
) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(New(st, Options{
		Mode: ModeLocal, Workdir: dir, Info: testInfo, Sharer: c,
	}).Handler())
	t.Cleanup(ts.Close)
	return ts
}

// createLocal stores a page the way the local MCP does: rendered content
// plus its Doc source.
func createLocal(t *testing.T, st store.Store, title string) store.Page {
	t.Helper()
	p, err := st.Create(t.Context(), store.Draft{
		Title:   title,
		Content: "<p>" + title + "</p>",
		Doc:     []byte(`{"sections":[]}`),
	})
	if err != nil {
		t.Fatalf("create local page: %v", err)
	}
	return p
}

// postShare hits the local share endpoint with a raw body (empty means no
// body at all) and returns the status and body.
func postShare(t *testing.T, ts *httptest.Server, id, body string) (int, []byte) {
	t.Helper()
	var payload io.Reader
	if body != "" {
		payload = strings.NewReader(body)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/p/"+id+"/share", payload)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST share: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

// shareOK posts a share and decodes the 200 reply.
func shareOK(t *testing.T, ts *httptest.Server, id, body string) apiShare {
	t.Helper()
	code, raw := postShare(t, ts, id, body)
	if code != http.StatusOK {
		t.Fatalf("POST share %q = %d, want 200 (%s)", body, code, raw)
	}
	var out apiShare
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode share reply: %v (%s)", err, raw)
	}
	return out
}

func apiPageOf(t *testing.T, url string) apiPage {
	t.Helper()
	code, body := get(t, url)
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", url, code)
	}
	var out apiPage
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return out
}

func TestShareRoundTripsToTheSharedInstance(t *testing.T) {
	f := setupLocalSharing(t, author.NewKey())
	p := createLocal(t, f.st, "Local")

	first := shareOK(t, f.ts, p.ID, `{"ephemeral":true}`)
	if !first.Enabled {
		t.Error("share reply must report sharing enabled")
	}
	prefix := f.shared.ts.URL + "/p/"
	if !strings.HasPrefix(first.URL, prefix) {
		t.Fatalf("url = %q, want prefix %q", first.URL, prefix)
	}
	sharedID := strings.TrimPrefix(first.URL, prefix)
	if !hex32.MatchString(sharedID) {
		t.Fatalf("shared id = %q, want 32 hex chars", sharedID)
	}
	if !first.Ephemeral || first.ExpiresAt == nil || first.SharedAt == nil {
		t.Fatalf("ephemeral share reply = %+v, want ephemeral with expiry and shared_at", first)
	}

	local := apiPageOf(t, f.ts.URL+"/api/p/"+p.ID)
	if !local.Share.Enabled || local.Share.URL != first.URL {
		t.Errorf("local page share = %+v, want enabled with url %q", local.Share, first.URL)
	}

	// The copy is readable on the shared instance by anyone with the link.
	copyPage := apiPageOf(t, f.shared.ts.URL+"/api/p/"+sharedID)
	if copyPage.Title != p.Title || copyPage.Content != p.Content {
		t.Errorf("shared copy = title %q content %q, want %q / %q",
			copyPage.Title, copyPage.Content, p.Title, p.Content)
	}

	// Sharing again keeps the link and can drop the expiry.
	second := shareOK(t, f.ts, p.ID, `{"ephemeral":false}`)
	if second.URL != first.URL {
		t.Errorf("re-share url = %q, want the same link %q", second.URL, first.URL)
	}
	if second.Ephemeral || second.ExpiresAt != nil {
		t.Errorf("re-share as permanent = %+v, want no expiry", second)
	}

	// No body at all means a permanent share.
	third := shareOK(t, f.ts, p.ID, "")
	if third.Ephemeral || third.URL != first.URL {
		t.Errorf("empty-body share = %+v, want permanent under %q", third, first.URL)
	}
}

func TestShareUnknownPageIs404(t *testing.T) {
	f := setupLocalSharing(t, author.NewKey())
	if code, _ := postShare(t, f.ts, store.NewID(), `{"ephemeral":true}`); code != http.StatusNotFound {
		t.Fatalf("share of unknown page = %d, want 404", code)
	}
}

func TestShareWithAnotherKeyIsBadGateway(t *testing.T) {
	f := setupLocalSharing(t, author.NewKey())
	p := createLocal(t, f.st, "Owned")
	shareOK(t, f.ts, p.ID, `{"ephemeral":false}`)

	// A second local process over the same store but a different key: the
	// shared instance refuses to replace the copy, and that is its failure.
	other, err := store.NewFS(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	ts := newLocalServer(t, other, f.dir, sharedclient.New(f.shared.ts.URL, author.NewKey()))
	code, body := postShare(t, ts, p.ID, "")
	if code != http.StatusBadGateway {
		t.Fatalf("share with other key = %d, want 502 (%s)", code, body)
	}
	if !strings.Contains(string(body), "share failed") || !strings.Contains(string(body), "403") {
		t.Errorf("502 body = %q, want the shared instance's 403 named", body)
	}
}

func TestShareIsOffWithoutASharer(t *testing.T) {
	ts, st := setup(t)
	p := createLocal(t, st, "Unshared")

	if got := apiPageOf(t, ts.URL+"/api/p/"+p.ID).Share; got.Enabled || got.URL != "" {
		t.Errorf("share state without a sharer = %+v, want disabled", got)
	}
	if code, _ := postShare(t, ts, p.ID, `{"ephemeral":true}`); code != http.StatusNotFound {
		t.Errorf("POST share without a sharer = %d, want 404 (route unregistered)", code)
	}
}

func TestSharedModeNeverOffersShare(t *testing.T) {
	f := setupShared(t)
	out := f.create(t, author.NewKey(), page("Hosted"))
	if got := apiPageOf(t, f.ts.URL+"/api/p/"+out.ID).Share; got.Enabled {
		t.Errorf("shared instance page share = %+v, want disabled", got)
	}
}
