package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/speak/internal/config"
	"github.com/mad01/thismoon/services/speak/internal/provider"
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
			p := provider.New(context.Background(), config.Provider{
				Name: "local", Type: config.TypeLocal, BaseURL: tc.url,
				Model: "kokoro", Voice: "af_heart", Voices: []string{"af_heart"}, Format: "wav",
			})
			report := doctor.Collect(context.Background(), []doctor.Check{synthesisWorks(p)})
			got := report.Checks[0]
			if got.Status != tc.wantStatus || !strings.Contains(got.Detail, tc.wantDetail) {
				t.Errorf("check = %+v, want status %s with detail containing %q",
					got, tc.wantStatus, tc.wantDetail)
			}
		})
	}
}

// TestProviderChecks pins how doctor treats blocks: the active one fails on
// a problem, an inactive one only notes it, since nothing uses it yet.
func TestProviderChecks(t *testing.T) {
	cfg := config.Config{Active: "gemini", Providers: []config.Provider{
		{Name: "gemini", Problem: "GEMINI_API_KEY is not set"},
		{Name: "local"},
		{Name: "openrouter", Problem: "OPENROUTER_API_KEY is not set"},
	}}
	report := doctor.Collect(context.Background(), providerChecks(cfg))
	want := map[string]string{
		"provider gemini (active)": doctor.StatusFail,
		"provider local":           doctor.StatusOK,
		"provider openrouter":      doctor.StatusSkipped,
	}
	for _, r := range report.Checks {
		if want[r.Name] != r.Status {
			t.Errorf("%s = %s (%s), want %s", r.Name, r.Status, r.Detail, want[r.Name])
		}
	}
	if report.OK {
		t.Error("report OK with the active provider unusable")
	}
}
