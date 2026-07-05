package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/status/internal/discover"
	"github.com/mad01/thismoon/services/status/internal/history"
)

func testMonitor() *Monitor {
	m := newMonitor(Options{HistoryDays: 30}, nil)
	m.snap = Snapshot{
		GeneratedAt: time.Date(2026, 6, 10, 12, 0, 0, 0, time.Local),
		WindowDays:  30,
		Down:        1,
		Services: []ServiceStatus{
			{
				Service: discover.Service{
					Label: "speak-web",
					Port:  7425,
					Link:  "http://speak.this/",
				},
				Up: true, Known: true, Detail: "HTTP 200",
				CheckedAt: time.Date(2026, 6, 10, 12, 0, 0, 0, time.Local),
				Uptime:    99.93, HasUptime: true, Version: "80d09e8",
				Days: []history.Day{
					{Date: "2026-06-10", OK: 100, Fail: 0, HasData: true, Pct: 100},
				},
			},
			{
				Service: discover.Service{Label: "dark-notify"},
				Up:      false, Known: true, Detail: "not running",
				CheckedAt: time.Date(2026, 6, 10, 12, 0, 0, 0, time.Local),
				Days:      []history.Day{{Date: "2026-06-10"}},
			},
		},
	}
	return m
}

func TestPageServesShell(t *testing.T) {
	mux := NewMux(testMonitor(), "test-sha")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != 200 {
		t.Fatalf("GET / = %d", rec.Code)
	}
	body := rec.Body.String()
	// The page is a static shell: chrome + an empty mount point + the client
	// scripts. The body is rendered in the browser, so no service data inlined.
	for _, want := range []string{
		`id="app"`, "/app.js", "/webkit/webkit.js",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("shell missing %q", want)
		}
	}
	for _, absent := range []string{"speak-web", "dark-notify", "Operational"} {
		if strings.Contains(body, absent) {
			t.Errorf("shell should not inline service data, found %q", absent)
		}
	}
}

func TestAppJS(t *testing.T) {
	mux := NewMux(testMonitor(), "test-sha")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/app.js", nil))

	if rec.Code != 200 {
		t.Fatalf("GET /app.js = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("Content-Type = %q, want application/javascript", ct)
	}
	if !strings.Contains(rec.Body.String(), "/api/status") {
		t.Error("app.js should fetch /api/status")
	}
}

func TestAPIStatus(t *testing.T) {
	mux := NewMux(testMonitor(), "test-sha")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/status", nil))

	if rec.Code != 200 {
		t.Fatalf("GET /api/status = %d", rec.Code)
	}
	var snap Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Services) != 2 || snap.Down != 1 {
		t.Errorf("snapshot = %d services / %d down, want 2 / 1", len(snap.Services), snap.Down)
	}

	// app.js renders from these field names, so the JSON shape is the contract.
	body := rec.Body.String()
	for _, want := range []string{
		`"generated_at"`, `"window_days"`, `"down"`, `"services"`,
		`"label"`, `"link"`, `"up"`, `"known"`, `"detail"`, `"checked_at"`,
		`"version"`, `"uptime_pct"`, `"has_uptime"`, `"days"`, `"port"`,
		`"date"`, `"has_data"`, `"pct"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("api/status JSON missing field %s", want)
		}
	}
}

func TestVersionAndHealthz(t *testing.T) {
	mux := NewMux(testMonitor(), "test-sha")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/version", nil))
	var v map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v["version"] != "test-sha" {
		t.Errorf("version = %q", v["version"])
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 204 {
		t.Errorf("GET /healthz = %d, want 204", rec.Code)
	}
}

// The banner / dayClass / shortenWebkit presentation logic moved from Go
// (internal/server/view.go) into the client renderer (app.js) as part of the
// client-side-rendering migration (ADR-0017), so it is no longer tested here.
