package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
)

// TestSynthesisWorks pins what tts-synthesis adds over the reachability
// ping: an engine that answers but cannot speak fails with its own reason,
// and an unreachable one skips because tts-engine-reachable reports it.
func TestSynthesisWorks(t *testing.T) {
	live := func(status int, body string) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		return srv.URL
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()

	cases := []struct {
		name       string
		url        string
		wantStatus string
		wantDetail string
	}{
		{"speaks", live(http.StatusOK, "RIFFfake"), doctor.StatusOK, ""},
		{
			"engine up, model missing",
			live(http.StatusInternalServerError, `{"detail":"LocalEntryNotFoundError"}`),
			doctor.StatusFail,
			"local synthesis failed (upstream): TTS engine returned 500: LocalEntryNotFoundError",
		},
		{"engine stopped", dead.URL, doctor.StatusSkipped, "see tts-engine-reachable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := doctor.Collect(context.Background(), []doctor.Check{synthesisWorks(tc.url)})
			got := report.Checks[0]
			if got.Status != tc.wantStatus || !strings.Contains(got.Detail, tc.wantDetail) {
				t.Errorf("check = %+v, want status %s with detail containing %q",
					got, tc.wantStatus, tc.wantDetail)
			}
		})
	}
}
