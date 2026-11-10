package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestParseWindow(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	const id = "01762776000000000000-ab12"
	tests := []struct {
		name, since, afterID string
		wantSince            time.Time
		wantAfter            string
		wantErr              []string // substrings the error must carry
	}{
		{name: "empty"},
		{name: "span", since: "24h", wantSince: now.Add(-24 * time.Hour)},
		{name: "days", since: "7d", wantSince: now.Add(-7 * 24 * time.Hour)},
		{
			name:      "rfc3339",
			since:     "2026-11-09T08:00:00Z",
			wantSince: time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC),
		},
		{name: "after_id", afterID: id, wantAfter: id},
		{
			name: "both", since: "1h", afterID: id,
			wantSince: now.Add(-time.Hour), wantAfter: id,
		},
		{name: "legacy since id", since: id, wantAfter: id},
		{name: "bad since", since: "yesterday", wantErr: []string{"since must be", "after_id"}},
		{name: "bad after_id", afterID: "24h", wantErr: []string{"after_id must be", "since=24h"}},
		{name: "id twice", since: id, afterID: id, wantErr: []string{"once, as after_id"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			since, after, err := parseWindow(tc.since, tc.afterID, now)
			if len(tc.wantErr) > 0 {
				if err == nil {
					t.Fatalf("parseWindow = %v, %q; want an error", since, after)
				}
				for _, w := range tc.wantErr {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("error %q does not mention %q", err, w)
					}
				}
				return
			}
			if err != nil || !since.Equal(tc.wantSince) || after != tc.wantAfter {
				t.Errorf("parseWindow = %v, %q, %v; want %v, %q",
					since, after, err, tc.wantSince, tc.wantAfter)
			}
		})
	}
}

// TestQueryRejectsBadWindow holds the API to a 400 for a since it cannot
// read, where it used to return an empty success.
func TestQueryRejectsBadWindow(t *testing.T) {
	for _, path := range []string{
		"/api/events?since=yesterday",
		"/api/events?after_id=24h",
	} {
		rec := httptest.NewRecorder()
		testHandler(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "must be") {
			t.Errorf("GET %s body = %s, want the must-be message", path, rec.Body)
		}
	}
}

// TestPageTailsWithAfterID keeps the timeline's live tail on the cursor
// parameter, so a since span never reads the newest id.
func TestPageTailsWithAfterID(t *testing.T) {
	if !strings.Contains(string(indexHTML), "fetchMerged('after_id', newestId)") {
		t.Error("timeline page no longer tails with after_id")
	}
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
