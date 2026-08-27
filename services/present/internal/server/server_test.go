package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// testInfo is the build metadata the test server reports on /version.
var testInfo = buildinfo.Info{
	Version:   "test",
	Commit:    "0123456789abcdef0123456789abcdef01234567",
	Tag:       "present/v0.0.0",
	BuildTime: "2026-08-13T09:00:00Z",
}

func setup(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	ts := httptest.NewServer(New(st, dir, testInfo).Handler())
	t.Cleanup(ts.Close)
	return ts, st
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestPageServesShell(t *testing.T) {
	ts, st := setup(t)
	p, _ := st.Create("Demo", "<p data-fixation>unique-marker-text</p>", "", nil)

	code, body := get(t, ts.URL+"/p/"+p.ID)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	// The shell is chrome only; the body is fetched from /api/p/{id} and built
	// client-side by /app.js, so the page content is NOT in the served HTML.
	for _, want := range []string{`id="root"`, "/app.js", "/webkit/webkit.js", "wk-section li"} {
		if !contains(body, want) {
			t.Errorf("shell missing %q", want)
		}
	}
	if contains(body, "unique-marker-text") {
		t.Errorf("shell must not inline page content (the client renders it)")
	}
}

func TestAPIPageReturnsContentAndTitle(t *testing.T) {
	ts, st := setup(t)
	p, _ := st.Create("Demo", "<p data-fixation>unique-marker-text</p>", "", nil)

	code, body := get(t, ts.URL+"/api/p/"+p.ID)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	var got apiPage
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("api body not JSON: %v (body=%q)", err, body)
	}
	if got.Title != "Demo" {
		t.Errorf("title = %q, want Demo", got.Title)
	}
	if !contains(got.Content, "unique-marker-text") {
		t.Errorf("api content missing body fragment")
	}
	if got.Version != 1 {
		t.Errorf("version = %d, want 1", got.Version)
	}
}

func TestAppJSServed(t *testing.T) {
	ts, _ := setup(t)
	resp, err := http.Get(ts.URL + "/app.js")
	if err != nil {
		t.Fatalf("GET /app.js: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want javascript", ct)
	}
}

func TestPageSetsNoStoreCacheHeader(t *testing.T) {
	ts, st := setup(t)
	p, _ := st.Create("Cache", "<p>body</p>", "", nil)

	resp, err := http.Get(ts.URL + "/p/" + p.ID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q (stale cache causes reload loops)", got, "no-store")
	}
}

func TestUnknownPageIs404(t *testing.T) {
	ts, _ := setup(t)
	code, _ := get(t, ts.URL+"/p/doesnotexist")
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
}

func TestVersionEndpointReflectsUpdates(t *testing.T) {
	ts, st := setup(t)
	p, _ := st.Create("V", "<p>v1</p>", "", nil)

	code, body := get(t, ts.URL+"/p/"+p.ID+"/version")
	if code != http.StatusOK || body != "1" {
		t.Fatalf("version = %q (status %d), want \"1\"", body, code)
	}

	newContent := "<p>v2</p>"
	if _, err := st.Update(p.ID, store.Patch{Content: &newContent}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	_, body = get(t, ts.URL+"/p/"+p.ID+"/version")
	if body != "2" {
		t.Fatalf("version after update = %q, want \"2\"", body)
	}
}

func TestVersionUnknownIs404(t *testing.T) {
	ts, _ := setup(t)
	code, _ := get(t, ts.URL+"/p/nope/version")
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
}

// apiPagesBody is the decoded shape of GET /api/pages, mirroring the server's
// apiPages payload for assertions.
type apiPagesBody struct {
	Pages []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Version   int    `json:"version"`
		UpdatedAt string `json:"updated_at"`
	} `json:"pages"`
	Page       int  `json:"page"`
	Size       int  `json:"size"`
	Total      int  `json:"total"`
	TotalPages int  `json:"total_pages"`
	HasPrev    bool `json:"has_prev"`
	HasNext    bool `json:"has_next"`
	PrevPage   int  `json:"prev_page"`
	NextPage   int  `json:"next_page"`
}

func getAPIPages(t *testing.T, url string) apiPagesBody {
	t.Helper()
	code, body := get(t, url)
	if code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", url, code)
	}
	var got apiPagesBody
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("GET %s: body not JSON: %v (body=%q)", url, err, body)
	}
	return got
}

// hasID reports whether the page window contains the given id.
func hasID(pages apiPagesBody, id string) bool {
	for _, p := range pages.Pages {
		if p.ID == id {
			return true
		}
	}
	return false
}

// idIndex returns the position of id within the page window, or -1.
func idIndex(pages apiPagesBody, id string) int {
	for i, p := range pages.Pages {
		if p.ID == id {
			return i
		}
	}
	return -1
}

func TestIndexServesShell(t *testing.T) {
	ts, st := setup(t)
	_, _ = st.Create("Alpha", "<p>unique-marker-text</p>", "", nil)

	code, body := get(t, ts.URL+"/")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	// The shell is chrome only; the page list is fetched from /api/pages and
	// built client-side by /index.js, so no page titles appear in the HTML.
	for _, want := range []string{`id="app"`, "/index.js", "/webkit/webkit.js", "delete-modal"} {
		if !contains(body, want) {
			t.Errorf("index shell missing %q", want)
		}
	}
	if contains(body, "Alpha") {
		t.Errorf("shell must not inline page titles (the client renders them)")
	}
}

func TestIndexJSServed(t *testing.T) {
	ts, _ := setup(t)
	resp, err := http.Get(ts.URL + "/index.js")
	if err != nil {
		t.Fatalf("GET /index.js: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want javascript", ct)
	}
}

func TestAPIPagesListsPages(t *testing.T) {
	ts, st := setup(t)
	a, _ := st.Create("Alpha", "<p>a</p>", "", nil)
	b, _ := st.Create("Beta", "<p>b</p>", "", nil)

	pages := getAPIPages(t, ts.URL+"/api/pages")
	if pages.Total != 2 {
		t.Errorf("total = %d, want 2", pages.Total)
	}
	if !hasID(pages, a.ID) || !hasID(pages, b.ID) {
		t.Errorf("api pages missing one of %q / %q", a.ID, b.ID)
	}
}

func TestIndexSetsNoCacheHeader(t *testing.T) {
	ts, st := setup(t)
	_, _ = st.Create("Only", "<p>x</p>", "", nil)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q (so a new page shows on reload)", got, "no-cache")
	}
}

// createOrdered creates n pages with stable, predictable titles and returns
// them in creation order (oldest first). Newest-first listing is the reverse.
func createOrdered(t *testing.T, st *store.Store, n int) []store.Page {
	t.Helper()
	pages := make([]store.Page, 0, n)
	for i := 0; i < n; i++ {
		p, err := st.Create(fmt.Sprintf("Page-%02d", i), "<p>x</p>", "", nil)
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		pages = append(pages, p)
	}
	return pages
}

func TestAPIPagesPaginatesFirstPage(t *testing.T) {
	ts, st := setup(t)
	created := createOrdered(t, st, 5) // newest is created[4]

	pages := getAPIPages(t, ts.URL+"/api/pages?page=1&size=2")
	// Page 1 (size 2) shows the two newest: created[4], created[3].
	for _, p := range created[3:] {
		if !hasID(pages, p.ID) {
			t.Errorf("page 1 missing newest page %q", p.Title)
		}
	}
	// Older pages must not appear on page 1.
	for _, p := range created[:3] {
		if hasID(pages, p.ID) {
			t.Errorf("page 1 unexpectedly contains older page %q", p.Title)
		}
	}
	// Total count reflects all pages, not the window.
	if pages.Total != 5 {
		t.Errorf("total = %d, want 5", pages.Total)
	}
	if pages.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", pages.TotalPages)
	}
	// Not the last page: a next link to page 2 must be available.
	if !pages.HasNext || pages.NextPage != 2 {
		t.Errorf("page 1 should have has_next with next_page=2, got has_next=%v next_page=%d", pages.HasNext, pages.NextPage)
	}
	if pages.HasPrev {
		t.Errorf("page 1 should not have has_prev")
	}
}

func TestAPIPagesPreservesOrderingAcrossPages(t *testing.T) {
	ts, st := setup(t)
	created := createOrdered(t, st, 5) // newest-first order: 4,3,2,1,0

	p1 := getAPIPages(t, ts.URL+"/api/pages?page=1&size=2")
	p2 := getAPIPages(t, ts.URL+"/api/pages?page=2&size=2")
	p3 := getAPIPages(t, ts.URL+"/api/pages?page=3&size=2")

	// Page 2 holds created[2], created[1]; page 3 holds the oldest created[0].
	for _, p := range []store.Page{created[2], created[1]} {
		if !hasID(p2, p.ID) {
			t.Errorf("page 2 missing %q", p.Title)
		}
	}
	if !hasID(p3, created[0].ID) {
		t.Errorf("page 3 missing oldest page %q", created[0].Title)
	}

	// Within page 1 the newest page must appear before the second-newest.
	if idxNewest, idxSecond := idIndex(p1, created[4].ID), idIndex(p1, created[3].ID); idxNewest > idxSecond {
		t.Errorf(
			"page 1 ordering wrong: newest (%d) should precede second (%d)",
			idxNewest,
			idxSecond,
		)
	}

	// Pages are disjoint: a page 1 entry must not reappear on page 2.
	if hasID(p2, created[4].ID) {
		t.Errorf("page 2 unexpectedly repeats a page-1 entry")
	}
}

func TestAPIPagesLastPageHasNoNext(t *testing.T) {
	ts, st := setup(t)
	createOrdered(t, st, 5)

	pages := getAPIPages(t, ts.URL+"/api/pages?page=3&size=2") // last page (ceil(5/2)=3)
	if pages.HasNext {
		t.Errorf("last page should not have has_next")
	}
	if !pages.HasPrev || pages.PrevPage != 2 {
		t.Errorf("last page should have has_prev with prev_page=2, got has_prev=%v prev_page=%d", pages.HasPrev, pages.PrevPage)
	}
}

func TestAPIPagesOutOfRangePageClamps(t *testing.T) {
	ts, st := setup(t)
	created := createOrdered(t, st, 5)

	// page=99 is out of range; it must clamp to the last page (3) without error.
	pages := getAPIPages(t, ts.URL+"/api/pages?page=99&size=2")
	if !hasID(pages, created[0].ID) {
		t.Errorf("out-of-range page should clamp to last page (oldest entry)")
	}
	if pages.Page != 3 || pages.TotalPages != 3 {
		t.Errorf("out-of-range page should clamp to page 3 of 3, got page=%d total_pages=%d", pages.Page, pages.TotalPages)
	}

	// page=0 and garbage clamp to page 1 (newest entry).
	for _, q := range []string{"?page=0&size=2", "?page=abc&size=2"} {
		pages := getAPIPages(t, ts.URL+"/api/pages"+q)
		if !hasID(pages, created[4].ID) {
			t.Errorf("GET %s should clamp to page 1 (newest entry)", q)
		}
		if pages.Page != 1 {
			t.Errorf("GET %s should clamp to page 1, got page=%d", q, pages.Page)
		}
	}
}

func TestDeletePageRemovesIt(t *testing.T) {
	ts, st := setup(t)
	p, _ := st.Create("Doomed", "<p>x</p>", "", nil)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/p/"+p.ID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	code, _ := get(t, ts.URL+"/p/"+p.ID)
	if code != http.StatusNotFound {
		t.Fatalf("GET after delete = %d, want 404", code)
	}
}

func TestDeleteUnknownPageIs404(t *testing.T) {
	ts, _ := setup(t)
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/p/doesnotexist", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestIndexHasDeleteAffordance(t *testing.T) {
	ts, st := setup(t)
	p, _ := st.Create("Alpha", "<p>a</p>", "", nil)

	// The delete affordance is built client-side from the page id (which carries
	// the data-id) plus the static delete-modal in the shell.
	_, shell := get(t, ts.URL+"/")
	if !contains(shell, "delete-modal") {
		t.Errorf("index shell missing delete-modal")
	}
	pages := getAPIPages(t, ts.URL+"/api/pages")
	if !hasID(pages, p.ID) {
		t.Errorf("api pages missing %q (no data-id source for the delete button)", p.ID)
	}
}

func TestAPIPageReturnsReferences(t *testing.T) {
	ts, st := setup(t)
	refs := []store.Reference{
		{Title: "dotfiles repo", URL: "https://github.com/mad01/dotfiles"},
	}
	p, _ := st.Create("Refs", "<p>body</p>", "", refs)
	code, body := get(t, ts.URL+"/api/p/"+p.ID)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	var got apiPage
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("api body not JSON: %v (body=%q)", err, body)
	}
	if len(got.References) != 1 {
		t.Fatalf("references = %d, want 1", len(got.References))
	}
	if got.References[0].Title != "dotfiles repo" || got.References[0].URL != "https://github.com/mad01/dotfiles" {
		t.Errorf("reference = %+v, want the dotfiles ref", got.References[0])
	}
}

// The shared webkit chrome must be served at /webkit/ so the brief template can
// link /webkit/webkit.css and /webkit/webkit.js.
func TestWebkitAssetsServed(t *testing.T) {
	ts, _ := setup(t)
	for _, path := range []string{"/webkit/webkit.css", "/webkit/webkit.js"} {
		code, body := get(t, ts.URL+path)
		if code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", path, code)
		}
		if body == "" {
			t.Errorf("GET %s: empty body", path)
		}
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
