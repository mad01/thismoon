package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/status/internal/discover"
	"github.com/mad01/thismoon/services/status/internal/server"
)

// serveSnapshot stands in for status serve, answering GET /api/status with
// snap.
func serveSnapshot(t *testing.T, snap server.Snapshot) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func svc(label string, up bool, detail string) server.ServiceStatus {
	return server.ServiceStatus{
		Service: discover.Service{Label: label, Port: 7400},
		Up:      up, Known: true, Detail: detail,
	}
}

var checkedAt = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func TestListAllUpExitsZero(t *testing.T) {
	up := svc("events", true, "HTTP 200")
	up.Drift = true
	url := serveSnapshot(t, server.Snapshot{
		GeneratedAt: checkedAt,
		Services:    []server.ServiceStatus{up, svc("deps", true, "HTTP 200")},
	})
	var out bytes.Buffer
	if err := runList(&out, url, outputText); err != nil {
		t.Fatalf("runList with every service up: %v", err)
	}
	for _, want := range []string{"events", "up, stale binary", "deps", "0 of 2 down"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

// TestListDownExitsNonzero is the gap list closes: a down service showed only
// through curl, and nothing exited nonzero for it.
func TestListDownExitsNonzero(t *testing.T) {
	url := serveSnapshot(t, server.Snapshot{
		GeneratedAt: checkedAt,
		Down:        1,
		Services: []server.ServiceStatus{
			svc("events", true, "HTTP 200"),
			svc("speak", false, "connection refused"),
		},
	})
	var out bytes.Buffer
	err := runList(&out, url, outputJSON)
	if err == nil {
		t.Fatal("runList with a down service returned nil, want the exit-1 error")
	}
	if !strings.Contains(err.Error(), "1 of 2 services down: speak") {
		t.Errorf("error = %q, want it to name the down service", err)
	}

	var got listing
	if jerr := json.Unmarshal(out.Bytes(), &got); jerr != nil {
		t.Fatalf("-o json output is not JSON: %v\n%s", jerr, out.String())
	}
	if got.Down != 1 || len(got.Services) != 2 || got.Services[1].Label != "speak" ||
		got.Services[1].Up || got.Services[1].Detail != "connection refused" {
		t.Errorf("listing = %+v", got)
	}
	if strings.Contains(out.String(), `"days"`) {
		t.Error("the JSON listing carries the uptime days, want them left out")
	}
}

func TestListErrors(t *testing.T) {
	notChecked := serveSnapshot(t, server.Snapshot{})
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()

	tests := []struct {
		name, url, output, want string
	}{
		{"bad output", notChecked, "yaml", `--output must be text or json, got "yaml"`},
		{"first check pending", notChecked, outputText, "has not finished its first check"},
		{"serve down", dead.URL, outputText, "status doctor"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runList(&out, tc.url, tc.output)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
			if out.Len() != 0 {
				t.Errorf("printed %q before failing, want nothing", out.String())
			}
		})
	}
}
