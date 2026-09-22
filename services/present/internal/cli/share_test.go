package cli

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
)

// setSharingFlags points the package-level sharing flags at url and key for
// the test's lifetime and restores them afterwards.
func setSharingFlags(t *testing.T, url, key string) {
	t.Helper()
	prevURL, prevKey := flagSharedURL, flagAuthorKey
	flagSharedURL, flagAuthorKey = url, key
	t.Cleanup(func() { flagSharedURL, flagAuthorKey = prevURL, prevKey })
}

// quietLog silences the half-configuration warning sharer prints.
func quietLog(t *testing.T) {
	t.Helper()
	prev := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(prev) })
}

func TestSharerNeedsBothURLAndKey(t *testing.T) {
	quietLog(t)
	cases := []struct {
		name    string
		url     string
		key     string
		wantURL string
	}{
		{"both set", "https://present.example.com/", "k", "https://present.example.com"},
		{"only url", "https://present.example.com", "", ""},
		{"only key", "", "k", ""},
		{"neither", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setSharingFlags(t, tc.url, tc.key)
			c := sharer()
			if tc.wantURL == "" {
				if c != nil {
					t.Fatalf("sharer() = %+v, want nil", c)
				}
				return
			}
			if c == nil {
				t.Fatal("sharer() = nil, want a client")
			}
			if c.BaseURL != tc.wantURL || c.Key != tc.key {
				t.Errorf("sharer() = {%q %q}, want {%q %q}", c.BaseURL, c.Key, tc.wantURL, tc.key)
			}
		})
	}
}

// whoamiServer answers GET /api/whoami with status and body.
func whoamiServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/whoami" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestSharedInstanceCheck(t *testing.T) {
	quietLog(t)
	ok := whoamiServer(t, http.StatusOK, `{"author":"x"}`)
	refused := whoamiServer(t, http.StatusUnauthorized, `unauthorized`)

	cases := []struct {
		name       string
		url        string
		key        string
		wantStatus string
		wantOK     bool
		wantDetail string
	}{
		{"nothing configured", "", "", doctor.StatusSkipped, true, ""},
		{"reachable and key accepted", ok.URL, "k", doctor.StatusOK, true, ""},
		{"key refused", refused.URL, "k", doctor.StatusFail, false, refused.URL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setSharingFlags(t, tc.url, tc.key)
			report := doctor.Collect(t.Context(), []doctor.Check{sharedInstanceCheck()})
			if report.OK != tc.wantOK || len(report.Checks) != 1 {
				t.Fatalf("report = %+v, want ok=%v with one check", report, tc.wantOK)
			}
			got := report.Checks[0]
			if got.Name != "shared-instance" || got.Status != tc.wantStatus {
				t.Errorf("check = %+v, want shared-instance %s", got, tc.wantStatus)
			}
			if tc.wantDetail != "" && !strings.Contains(got.Detail, tc.wantDetail) {
				t.Errorf("detail = %q, want it to name %q", got.Detail, tc.wantDetail)
			}
		})
	}
}

func TestKeyNewPrintsAFreshKey(t *testing.T) {
	var buf bytes.Buffer
	keyNewCmd.SetOut(&buf)
	t.Cleanup(func() { keyNewCmd.SetOut(nil) })

	if err := keyNewCmd.RunE(keyNewCmd, nil); err != nil {
		t.Fatalf("key new: %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}\n$`).MatchString(buf.String()) {
		t.Errorf("key new printed %q, want 64 lowercase hex and a newline", buf.String())
	}
}
