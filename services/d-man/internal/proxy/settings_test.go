package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mad01/thismoon/services/d-man/internal/config"
)

// settingsFixture is a proxy with one configured host backed by a counting
// backend, one blocked host, and a stub settings handler.
type settingsFixture struct {
	h            *Handler
	backendHits  atomic.Int32
	settingsHits atomic.Int32
}

func newSettingsFixture(t *testing.T, withSettings bool) *settingsFixture {
	t.Helper()
	f := &settingsFixture{}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.backendHits.Add(1)
		_, _ = w.Write([]byte("from backend"))
	}))
	t.Cleanup(backend.Close)
	addr := strings.TrimPrefix(backend.URL, "http://")

	var settings http.Handler
	if withSettings {
		settings = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			f.settingsHits.Add(1)
			_, _ = w.Write([]byte("from settings"))
		})
	}
	sites := []config.Site{{Name: "csl", Host: "csl.this", URL: "http://csl.this/", Backend: addr}}
	h, err := New(map[string]string{"csl.this": addr}, sites, []string{"reddit.com"}, "", settings)
	if err != nil {
		t.Fatal(err)
	}
	h.probe = alwaysUp
	f.h = h
	return f
}

func (f *settingsFixture) get(host, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "http://"+host+path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

func TestSettingsPathOnConfiguredHost(t *testing.T) {
	f := newSettingsFixture(t, true)
	for _, host := range []string{"csl.this", "CSL.this:80", "csl.this."} {
		rec := f.get(host, SettingsPath)
		if rec.Code != http.StatusOK || rec.Body.String() != "from settings" {
			t.Errorf("host %q: %d %q, want the settings handler", host, rec.Code, rec.Body.String())
		}
	}
	if n := f.backendHits.Load(); n != 0 {
		t.Errorf("backend hit %d times, want 0", n)
	}
	// Any other path on the same host still reaches the backend.
	if rec := f.get("csl.this", "/__this/settings.json/x"); rec.Body.String() != "from backend" {
		t.Errorf("neighbour path body = %q, want it proxied", rec.Body.String())
	}
}

func TestSettingsPathOnUnknownHostIs502(t *testing.T) {
	f := newSettingsFixture(t, true)
	if rec := f.get("nope.this", SettingsPath); rec.Code != http.StatusBadGateway {
		t.Errorf("code = %d, want 502", rec.Code)
	}
	if n := f.settingsHits.Load(); n != 0 {
		t.Errorf("settings handler hit %d times, want 0", n)
	}
}

func TestSettingsPathOnBlockedHostIsBlockPage(t *testing.T) {
	f := newSettingsFixture(t, true)
	rec := f.get("reddit.com", SettingsPath)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "is blocked") {
		t.Errorf("code = %d, want the block page", rec.Code)
	}
	if n := f.settingsHits.Load(); n != 0 {
		t.Errorf("settings handler hit %d times, want 0", n)
	}
}

func TestNilSettingsHandlerProxiesThePath(t *testing.T) {
	f := newSettingsFixture(t, false)
	rec := f.get("csl.this", SettingsPath)
	if rec.Body.String() != "from backend" || f.backendHits.Load() != 1 {
		t.Errorf("body = %q, backend hits = %d; want the path proxied",
			rec.Body.String(), f.backendHits.Load())
	}
}
