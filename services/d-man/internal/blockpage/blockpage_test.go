package blockpage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesPageAndGame(t *testing.T) {
	h := Handler()

	cases := []struct {
		path        string
		wantType    string
		wantContain string
	}{
		{"/", "text/html", "is blocked"},
		{"/some/blocked/path", "text/html", "is blocked"}, // catch-all still shows the page
		{"/dino.js", "application/javascript", "requestAnimationFrame"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tc.wantType) {
				t.Errorf("Content-Type = %q, want prefix %q", ct, tc.wantType)
			}
			if body := rec.Body.String(); !strings.Contains(body, tc.wantContain) {
				t.Errorf("body for %q does not contain %q", tc.path, tc.wantContain)
			}
		})
	}
}
