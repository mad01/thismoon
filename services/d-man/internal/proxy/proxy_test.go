package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mad01/thismoon/services/d-man/internal/config"
)

// alwaysUp is a Prober that reports every backend as live, for tests that
// aren't exercising the liveness filter itself.
func alwaysUp(string) bool { return true }

func TestRoutesByHost(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	addr := strings.TrimPrefix(backend.URL, "http://")
	h, err := New(map[string]string{"csl.this": addr}, nil, nil, "", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// All of these must resolve to the same route: plain, with port, with a
	// trailing FQDN dot, and uppercased.
	for _, host := range []string{"csl.this", "csl.this:80", "csl.this.", "CSL.this", "csl.this.:80"} {
		req := httptest.NewRequest("GET", "http://csl.this/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("host %q: code = %d, want 200", host, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "hello from backend") {
			t.Errorf("host %q: body = %q", host, rec.Body.String())
		}
	}
}

// SitesPath is answered by d-man itself on any configured host, instead of
// proxying to the backend — that's what makes the picker's fetch same-origin.
func TestServesSitesJSON(t *testing.T) {
	sites := []config.Site{
		{Name: "csl", Host: "csl.this", URL: "http://csl.this/", Backend: "127.0.0.1:9"},
	}
	h, err := New(map[string]string{"csl.this": "127.0.0.1:9"}, sites, nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	h.probe = alwaysUp // don't depend on a live backend for the JSON-shape check

	req := httptest.NewRequest("GET", "http://csl.this"+SitesPath, nil)
	req.Host = "csl.this"
	req.Header.Set("Origin", "http://localhost:7424")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("Content-Type = %q, want json", ct)
	}
	if acao := rec.Header().Get("Access-Control-Allow-Origin"); acao != "http://localhost:7424" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the origin reflected", acao)
	}
	if !strings.Contains(rec.Body.String(), `"csl.this"`) {
		t.Errorf("body = %q, want the site list", rec.Body.String())
	}
}

// A nil site list still yields valid JSON ("[]"), never an empty body.
func TestServesEmptySitesJSON(t *testing.T) {
	h, _ := New(map[string]string{"csl.this": "127.0.0.1:9"}, nil, nil, "", nil)
	req := httptest.NewRequest("GET", "http://csl.this"+SitesPath, nil)
	req.Host = "csl.this"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "[]" {
		t.Errorf("code=%d body=%q, want 200 []", rec.Code, rec.Body.String())
	}
}

// The site list names this machine's local services, so d-man answers it only
// on a configured route host. A page that rebinds its own name to 127.0.0.1
// reaches d-man with that name as its Host: it gets the 502 and no CORS
// header, and a blocked host gets the block page, never the list.
func TestSitesOnlyOnConfiguredHosts(t *testing.T) {
	var backendHits, settingsHits atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		backendHits.Add(1)
	}))
	defer backend.Close()
	addr := strings.TrimPrefix(backend.URL, "http://")
	settings := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { settingsHits.Add(1) })

	sites := []config.Site{
		{Name: "csl", Host: "csl.this", URL: "http://csl.this/", Backend: addr},
		{Name: "present", Host: "present.this", URL: "http://present.this/", Backend: addr},
	}
	routes := map[string]string{"csl.this": addr, "present.this": addr}
	h, err := New(routes, sites, []string{"reddit.com"}, "", settings)
	if err != nil {
		t.Fatal(err)
	}
	h.probe = alwaysUp

	const origin = "http://localhost:7424"
	get := func(host string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://csl.this"+SitesPath, nil)
		req.Host = host
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for _, host := range []string{"csl.this", "present.this", "PRESENT.this:80", "present.this."} {
		rec := get(host)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"present.this"`) {
			t.Errorf("host %q: %d %q, want the site list", host, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("host %q: ACAO = %q, want the origin reflected", host, got)
		}
	}

	for _, host := range []string{"evil.example", "evil.example:80", "127.0.0.1", "localhost", ""} {
		rec := get(host)
		if rec.Code != http.StatusBadGateway {
			t.Errorf("host %q: code = %d, want 502", host, rec.Code)
		}
		if strings.Contains(rec.Body.String(), `"csl.this"`) {
			t.Errorf("host %q: body = %q, want no site list", host, rec.Body.String())
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("host %q: ACAO = %q, want no CORS header", host, got)
		}
	}

	rec := get("reddit.com")
	if body := rec.Body.String(); !strings.Contains(body, "is blocked") ||
		strings.Contains(body, `"csl.this"`) {
		t.Errorf("blocked host: %d %q, want the block page and no site list", rec.Code, body)
	}

	if backendHits.Load() != 0 || settingsHits.Load() != 0 {
		t.Errorf("backend hits %d, settings hits %d; want sites answered by the proxy alone",
			backendHits.Load(), settingsHits.Load())
	}
}

// A blocked host is served the local block-page minigame on every path, never
// proxied and never a 502 — even when normalization is needed (case, port).
func TestServesBlockPageForBlockedHost(t *testing.T) {
	h, err := New(
		map[string]string{"csl.this": "127.0.0.1:9"},
		nil,
		[]string{"reddit.com"},
		"",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"reddit.com", "Reddit.com", "reddit.com:443"} {
		req := httptest.NewRequest("GET", "http://reddit.com/r/all", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("host %q: code = %d, want 200", host, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "is blocked") {
			t.Errorf("host %q: body missing block page: %q", host, rec.Body.String())
		}
	}
}

func TestUnknownHostIs502(t *testing.T) {
	h, err := New(map[string]string{"csl.this": "127.0.0.1:9"}, nil, nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "http://nope.this/", nil)
	req.Host = "nope.this"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("code = %d, want 502", rec.Code)
	}
}

// A backend that redirects to its own absolute URL (e.g. adding a trailing
// slash) must not leak its 127.0.0.1:port host to the client — the proxy
// rewrites Location back to the hostname the client used.
func TestRewritesBackendSelfRedirect(t *testing.T) {
	var backendURL string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, backendURL+"/app/", http.StatusFound)
	}))
	defer backend.Close()
	backendURL = backend.URL

	addr := strings.TrimPrefix(backend.URL, "http://")
	h, _ := New(map[string]string{"csl.this": addr}, nil, nil, "", nil)

	req := httptest.NewRequest("GET", "http://csl.this/app", nil)
	req.Host = "csl.this"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("code = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "csl.this") || strings.Contains(loc, "127.0.0.1") {
		t.Errorf("Location = %q, want host rewritten to csl.this", loc)
	}
}

// sitesBody fetches SitesPath as host, which must be a configured route.
func sitesBody(t *testing.T, h *Handler, host string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "http://"+host+SitesPath, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Body.String()
}

// The picker must show only sites whose backend is actually up on this host:
// routes.toml lists every possible site, but a service gated off (or just not
// running) on a given machine should not appear.
func TestSitesFilteredByLiveness(t *testing.T) {
	sites := []config.Site{
		{Name: "up", Host: "up.this", URL: "http://up.this/", Backend: "127.0.0.1:1"},
		{Name: "down", Host: "down.this", URL: "http://down.this/", Backend: "127.0.0.1:2"},
	}
	h, err := New(
		map[string]string{"up.this": "127.0.0.1:1", "down.this": "127.0.0.1:2"},
		sites,
		nil,
		"",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	h.ttl = 0 // re-probe every request
	h.probe = func(backend string) bool { return backend == "127.0.0.1:1" }

	body := sitesBody(t, h, "up.this")
	if !strings.Contains(body, `"up.this"`) {
		t.Errorf("body = %q, want the live site listed", body)
	}
	if strings.Contains(body, `"down.this"`) {
		t.Errorf("body = %q, want the dead site filtered out", body)
	}
}

// A burst of picker fetches inside the TTL window must trigger one probe round,
// not one per request — the result is cached.
func TestSitesProbeCached(t *testing.T) {
	sites := []config.Site{
		{Name: "csl", Host: "csl.this", URL: "http://csl.this/", Backend: "127.0.0.1:1"},
	}
	h, err := New(map[string]string{"csl.this": "127.0.0.1:1"}, sites, nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var probes atomic.Int32
	h.probe = func(string) bool { probes.Add(1); return true }

	for range 3 {
		sitesBody(t, h, "csl.this")
	}
	if got := probes.Load(); got != 1 {
		t.Errorf("probe calls = %d, want 1 (cached within TTL)", got)
	}
}
