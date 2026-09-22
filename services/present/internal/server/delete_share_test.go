package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/sharedclient"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// deletePage issues the web index's delete for a page and returns the
// status and body.
func deletePage(t *testing.T, ts *httptest.Server, id string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/p/"+id, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE page: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

// sharedIDOf extracts the shared instance's page id from a share reply.
func sharedIDOf(t *testing.T, f *localFixture, url string) string {
	t.Helper()
	prefix := f.shared.ts.URL + "/p/"
	if !strings.HasPrefix(url, prefix) {
		t.Fatalf("share url = %q, want prefix %q", url, prefix)
	}
	return strings.TrimPrefix(url, prefix)
}

func TestDeleteRemovesTheSharedCopyFirst(t *testing.T) {
	f := setupLocalSharing(t, author.NewKey())
	p := createLocal(t, f.st, "Local")
	sharedID := sharedIDOf(t, f, shareOK(t, f.ts, p.ID, `{"ephemeral":false}`).URL)

	if code, body := deletePage(t, f.ts, p.ID); code != http.StatusNoContent {
		t.Fatalf("delete = %d (%s), want 204", code, body)
	}
	if code, _ := get(t, f.shared.ts.URL+"/api/p/"+sharedID); code != http.StatusNotFound {
		t.Errorf("shared copy after delete = %d, want 404", code)
	}
	if code, _ := get(t, f.ts.URL+"/api/p/"+p.ID); code != http.StatusNotFound {
		t.Errorf("local page after delete = %d, want 404", code)
	}
}

func TestDeleteKeepsTheLocalPageWhenTheSharedInstanceRefuses(t *testing.T) {
	f := setupLocalSharing(t, author.NewKey())
	p := createLocal(t, f.st, "Owned")
	sharedID := sharedIDOf(t, f, shareOK(t, f.ts, p.ID, `{"ephemeral":false}`).URL)

	// A second local process over the same store but a different key: the
	// shared instance refuses to delete a page it did not author, and the
	// local page must survive so its copy stays findable.
	other, err := store.NewFS(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	ts := newLocalServer(t, other, f.dir, sharedclient.New(f.shared.ts.URL, author.NewKey()))
	code, body := deletePage(t, ts, p.ID)
	if code != http.StatusBadGateway {
		t.Fatalf("delete with another key = %d (%s), want 502", code, body)
	}
	if !strings.Contains(string(body), "unshare failed") ||
		!strings.Contains(string(body), "403") {
		t.Errorf("502 body = %q, want the shared instance's 403 named", body)
	}
	if code, _ := get(t, f.ts.URL+"/api/p/"+p.ID); code != http.StatusOK {
		t.Errorf("local page after a refused unshare = %d, want 200", code)
	}
	if code, _ := get(t, f.shared.ts.URL+"/api/p/"+sharedID); code != http.StatusOK {
		t.Errorf("shared copy after a refused unshare = %d, want 200", code)
	}
}

func TestDeleteWithoutASharerLeavesTheSharedCopy(t *testing.T) {
	ts, st := setup(t)
	p := createLocal(t, st, "Recorded")
	info := store.SharedInfo{
		ID:       store.NewSharedID(),
		URL:      "https://present.example.com/p/" + store.NewSharedID(),
		SharedAt: time.Now().UTC(),
	}
	if err := st.SetShared(t.Context(), p.ID, &info); err != nil {
		t.Fatal(err)
	}

	// Nothing here can reach the instance the page was pushed to, so the
	// local delete goes ahead and the copy stays where it is.
	if code, body := deletePage(t, ts, p.ID); code != http.StatusNoContent {
		t.Fatalf("delete without a sharer = %d (%s), want 204", code, body)
	}
	if code, _ := get(t, ts.URL+"/api/p/"+p.ID); code != http.StatusNotFound {
		t.Errorf("local page after delete = %d, want 404", code)
	}
}
