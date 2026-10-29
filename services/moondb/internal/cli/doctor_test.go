package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"

	"github.com/mad01/thismoon/services/moondb/internal/client"
)

// TestBriefsCheck: a count warns with the import that fills it, no count
// passes, and a serve without the route or without an answer steps aside
// for the checks that own those failures.
func TestBriefsCheck(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantNote string // "" means a plain pass
	}{
		{"none missing", http.StatusOK, `{"agents_to_fill":0}`, ""},
		{
			"some missing", http.StatusOK, `{"agents_to_fill":3}`,
			"3 agents without a spawn brief or later messages; run moondb import --since 30d --force",
		},
		{
			"a serve with only the old key", http.StatusOK, `{"agents_without_brief":2}`,
			"2 agents without a spawn brief or later messages; run moondb import --since 30d --force",
		},
		{"older serve", http.StatusNotFound, `{"error":"moondb: no API route"}`, "version-skew"},
		{
			"serve fails",
			http.StatusInternalServerError,
			`{"error":"moondb: boom"}`,
			"service-reachable",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/doctor" {
					t.Errorf("path = %s, want /api/doctor", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer ts.Close()
			err := briefsCheck(client.New(ts.URL))(context.Background())
			report := doctor.Collect(
				context.Background(),
				[]doctor.Check{
					{Name: "agent-briefs", Run: func(context.Context) error { return err }},
				},
			)
			got := report.Checks[0]
			if !report.OK {
				t.Fatalf("check failed: %+v", got)
			}
			if c.wantNote == "" {
				if got.Status != doctor.StatusOK {
					t.Errorf("status = %s (%s), want ok", got.Status, got.Detail)
				}
				return
			}
			if got.Status != doctor.StatusSkipped || !strings.Contains(got.Detail, c.wantNote) {
				t.Errorf("got %s %q, want skipped with %q", got.Status, got.Detail, c.wantNote)
			}
		})
	}
}
