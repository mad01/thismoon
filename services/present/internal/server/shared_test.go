package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/store"
)

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

// sharedFixture is a shared-mode server over a filesystem store with a
// controllable clock, so expiry can be driven from the test.
type sharedFixture struct {
	ts  *httptest.Server
	raw *store.FS
	now time.Time
}

func setupShared(t *testing.T) *sharedFixture {
	t.Helper()
	dir := t.TempDir()
	raw, err := store.NewFS(dir)
	if err != nil {
		t.Fatalf("store.NewFS: %v", err)
	}
	f := &sharedFixture{raw: raw, now: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)}
	clock := func() time.Time { return f.now }
	st := store.WithoutExpired(raw, clock)
	f.ts = httptest.NewServer(New(st, Options{
		Mode: ModeShared, Workdir: dir, Info: testInfo, Now: clock,
	}).Handler())
	t.Cleanup(f.ts.Close)
	return f
}

// call sends a JSON request with an optional bearer key and extra headers.
func (f *sharedFixture) call(
	t *testing.T, method, path, key string, body any, extra map[string]string,
) (*http.Response, []byte) {
	t.Helper()
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, f.ts.URL+path, payload)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp, out
}

func (f *sharedFixture) create(t *testing.T, key string, b map[string]any) sharedPage {
	t.Helper()
	resp, body := f.call(t, http.MethodPost, "/api/pages", key, b, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status %d body %s", resp.StatusCode, body)
	}
	var out sharedPage
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	return out
}

func page(title string) map[string]any {
	return map[string]any{"title": title, "content": "<p>" + title + "</p>"}
}

func TestSharedRootIsHowToNotIndex(t *testing.T) {
	f := setupShared(t)
	code, body := get(t, f.ts.URL+"/")
	if code != http.StatusOK {
		t.Fatalf("GET / = %d", code)
	}
	for _, want := range []string{"data-howto", "/mcp", "data-origin", "/webkit/webkit.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("how-to page missing %q", want)
		}
	}
	if strings.Contains(body, `id="app"`) || strings.Contains(body, "/index.js") {
		t.Error("shared root must not serve the index shell")
	}
}

func TestSharedHasNoListing(t *testing.T) {
	f := setupShared(t)
	out := f.create(t, author.NewKey(), page("Hidden"))
	// /api/pages exists for POST, so a GET is a method mismatch (405) rather
	// than a missing route; either way nothing lists the page.
	for _, path := range []string{"/api/pages", "/index.js"} {
		code, body := get(t, f.ts.URL+path)
		if code == http.StatusOK || strings.Contains(body, out.ID) {
			t.Errorf("GET %s = %d and body %q: shared mode must not list pages", path, code, body)
		}
	}
}

func TestSharedCreateNeedsKeyAndMintsCapabilityID(t *testing.T) {
	f := setupShared(t)
	resp, _ := f.call(t, http.MethodPost, "/api/pages", "", page("Anon"), nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("create without key = %d, want 401", resp.StatusCode)
	}
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("401 must carry WWW-Authenticate")
	}

	key := author.NewKey()
	out := f.create(t, key, page("Mine"))
	if !hex32.MatchString(out.ID) {
		t.Fatalf("id = %q, want 32 hex chars", out.ID)
	}
	if out.URL != f.ts.URL+"/p/"+out.ID {
		t.Fatalf("url = %q, want the request origin", out.URL)
	}
	if out.Version != 1 || out.Ephemeral || out.ExpiresAt != nil {
		t.Fatalf("unexpected create output: %+v", out)
	}
	p, err := f.raw.Get(t.Context(), out.ID)
	if err != nil {
		t.Fatalf("stored page: %v", err)
	}
	if p.Author != author.Hash(key) {
		t.Fatalf("author = %q, want hash of the key", p.Author)
	}
}

func TestSharedURLFollowsForwardedHeaders(t *testing.T) {
	f := setupShared(t)
	resp, body := f.call(t, http.MethodPost, "/api/pages", author.NewKey(), page("Fwd"),
		map[string]string{"X-Forwarded-Host": "present.example.com", "X-Forwarded-Proto": "https"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var out sharedPage
	_ = json.Unmarshal(body, &out)
	if want := "https://present.example.com/p/" + out.ID; out.URL != want {
		t.Fatalf("url = %q, want %q", out.URL, want)
	}
}

func TestSharedReadNeedsNoKey(t *testing.T) {
	f := setupShared(t)
	out := f.create(t, author.NewKey(), page("Public"))
	for _, path := range []string{"/p/" + out.ID, "/api/p/" + out.ID, "/p/" + out.ID + "/version"} {
		if code, _ := get(t, f.ts.URL+path); code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, code)
		}
	}
}

func TestSharedReplaceIsAuthorOnly(t *testing.T) {
	f := setupShared(t)
	owner, other := author.NewKey(), author.NewKey()
	out := f.create(t, owner, page("V1"))

	cases := []struct {
		name string
		key  string
		want int
	}{
		{"no key", "", http.StatusUnauthorized},
		{"other key", other, http.StatusForbidden},
		{"owner", owner, http.StatusOK},
	}
	for _, tc := range cases {
		resp, body := f.call(t, http.MethodPut, "/api/p/"+out.ID, tc.key, page("V2"), nil)
		if resp.StatusCode != tc.want {
			t.Errorf("%s: status %d, want %d (%s)", tc.name, resp.StatusCode, tc.want, body)
		}
	}
	p, _ := f.raw.Get(t.Context(), out.ID)
	if p.Version != 2 || p.Title != "V2" {
		t.Fatalf("after replace: version %d title %q", p.Version, p.Title)
	}

	resp, _ := f.call(t, http.MethodPut, "/api/p/"+store.NewSharedID(), owner, page("X"), nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown id: %d, want 404", resp.StatusCode)
	}
	resp, _ = f.call(t, http.MethodPut, "/api/p/not-an-id", owner, page("X"), nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("invalid id: %d, want 404", resp.StatusCode)
	}
}

func TestSharedReplaceSyncsSources(t *testing.T) {
	f := setupShared(t)
	key := author.NewKey()
	withDoc := page("Doc")
	withDoc["doc"] = json.RawMessage(`{"sections":[]}`)
	out := f.create(t, key, withDoc)
	if !f.raw.HasDoc(t.Context(), out.ID) {
		t.Fatal("create with doc must persist doc.json")
	}
	resp, _ := f.call(t, http.MethodPut, "/api/p/"+out.ID, key, page("NoDoc"), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("replace: %d", resp.StatusCode)
	}
	if f.raw.HasDoc(t.Context(), out.ID) {
		t.Fatal("replace without doc must remove the stale doc.json")
	}
}

func TestSharedDeleteIsAuthorOnly(t *testing.T) {
	f := setupShared(t)
	owner, other := author.NewKey(), author.NewKey()
	out := f.create(t, owner, page("Doomed"))

	if resp, _ := f.call(t, http.MethodDelete, "/p/"+out.ID, "", nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("delete without key = %d, want 401", resp.StatusCode)
	}
	if resp, _ := f.call(t, http.MethodDelete, "/p/"+out.ID, other, nil, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("delete with other key = %d, want 403", resp.StatusCode)
	}
	if resp, _ := f.call(t, http.MethodDelete, "/p/"+out.ID, owner, nil, nil); resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete by owner = %d, want 204", resp.StatusCode)
	}
	if code, _ := get(t, f.ts.URL+"/p/"+out.ID); code != http.StatusNotFound {
		t.Errorf("after delete: %d, want 404", code)
	}
}

func TestSharedEphemeralExpiresAndReplaceResets(t *testing.T) {
	f := setupShared(t)
	key := author.NewKey()
	b := page("Short-lived")
	b["ephemeral"] = true
	out := f.create(t, key, b)
	if !out.Ephemeral || out.ExpiresAt == nil {
		t.Fatalf("ephemeral create: %+v", out)
	}
	if want := f.now.Add(present.SharedTTL); !out.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", out.ExpiresAt, want)
	}

	// Ten days in, a replace starts the clock over.
	f.now = f.now.Add(10 * 24 * time.Hour)
	resp, body := f.call(t, http.MethodPut, "/api/p/"+out.ID, key, b, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("replace: %d %s", resp.StatusCode, body)
	}
	var replaced sharedPage
	_ = json.Unmarshal(body, &replaced)
	if want := f.now.Add(present.SharedTTL); replaced.ExpiresAt == nil ||
		!replaced.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at after replace = %v, want %v", replaced.ExpiresAt, want)
	}

	// Past the new expiry the page is gone from every surface, even though
	// the raw store still holds it until the sweeper runs.
	f.now = f.now.Add(present.SharedTTL)
	for _, path := range []string{"/p/" + out.ID, "/api/p/" + out.ID, "/p/" + out.ID + "/version"} {
		if code, _ := get(t, f.ts.URL+path); code != http.StatusNotFound {
			t.Errorf("expired GET %s = %d, want 404", path, code)
		}
	}
	if resp, _ := f.call(t, http.MethodPut, "/api/p/"+out.ID, key, b, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("expired PUT = %d, want 404", resp.StatusCode)
	}
	if _, err := f.raw.Get(t.Context(), out.ID); err != nil {
		t.Errorf("raw store lost the page before the sweeper ran: %v", err)
	}
}

func TestSharedRejectsOversizedAndInvalidBodies(t *testing.T) {
	f := setupShared(t)
	key := author.NewKey()
	huge := page("Huge")
	huge["content"] = strings.Repeat("x", maxBundleBytes+1)
	if resp, _ := f.call(t, http.MethodPost, "/api/pages", key, huge, nil); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized = %d, want 413", resp.StatusCode)
	}
	if resp, _ := f.call(t, http.MethodPost, "/api/pages", key, map[string]any{"content": "<p>no title</p>"}, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing title = %d, want 400", resp.StatusCode)
	}
	req, _ := http.NewRequest(
		http.MethodPost,
		f.ts.URL+"/api/pages",
		strings.NewReader("{not json"),
	)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed json = %d, want 400", resp.StatusCode)
	}
}

func TestSharedWhoAmI(t *testing.T) {
	f := setupShared(t)
	if resp, _ := f.call(t, http.MethodGet, "/api/whoami", "", nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("whoami without key = %d, want 401", resp.StatusCode)
	}
	key := author.NewKey()
	resp, body := f.call(t, http.MethodGet, "/api/whoami", key, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("whoami = %d", resp.StatusCode)
	}
	var out map[string]string
	_ = json.Unmarshal(body, &out)
	if out["author"] != author.Hash(key) {
		t.Fatalf("author = %q, want hash of the key", out["author"])
	}
}

func TestLocalModeHasNoSharedSurface(t *testing.T) {
	ts, _ := setup(t)
	req, _ := http.NewRequest(
		http.MethodPost,
		ts.URL+"/api/pages",
		strings.NewReader(`{"title":"x","content":"y"}`),
	)
	req.Header.Set("Authorization", "Bearer "+author.NewKey())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusCreated {
		t.Fatal("local mode must not accept pushed pages")
	}
	if code, _ := get(t, ts.URL+"/api/whoami"); code != http.StatusNotFound {
		t.Errorf("local /api/whoami = %d, want 404", code)
	}
	if code, _ := get(t, ts.URL+"/mcp"); code != http.StatusNotFound {
		t.Errorf("local /mcp = %d, want 404", code)
	}
}
