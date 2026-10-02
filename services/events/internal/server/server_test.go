package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/events/internal/store"
)

// testInfo is the build metadata the test server reports on /version.
var testInfo = buildinfo.Info{
	Version:   "test-sha",
	Commit:    "0123456789abcdef0123456789abcdef01234567",
	Tag:       "events/v0.0.0",
	BuildTime: "2026-08-13T09:00:00Z",
}

// testHandler builds the routes over an empty temp store with the default caps
// and clock; the page route under test never reads the store.
func testHandler(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.New(t.TempDir(), 0, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	return New(st, testInfo).Handler()
}

// TestPageServesShell asserts the timeline page is the webkit-chromed static
// shell and links the themes page webkit serves, so a reader can pick a
// palette family from events too.
func TestPageServesShell(t *testing.T) {
	rec := httptest.NewRecorder()
	testHandler(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET / Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"<wk-header", "/webkit/webkit.js", `<a data-nav href="/webkit/themes">Themes</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("shell missing %q", want)
		}
	}
}
