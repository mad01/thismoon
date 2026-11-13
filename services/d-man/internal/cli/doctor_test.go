package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/d-man/internal/proxy"
)

func TestProbeSettings(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "current daemon", status: http.StatusOK, body: `{"v":1,"settings":{}}`},
		{
			name:    "old daemon proxies to the backend",
			status:  http.StatusNotFound,
			body:    "404 page not found",
			wantErr: "returned 404",
		},
		{
			name:    "backend answers 200 with something else",
			status:  http.StatusOK,
			body:    "<html></html>",
			wantErr: "not a v1 settings document",
		},
		{
			name:    "other version",
			status:  http.StatusOK,
			body:    `{"v":2,"settings":{}}`,
			wantErr: "not a v1 settings document",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotHost, gotPath string
			srv := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					gotHost, gotPath = r.Host, r.URL.Path
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(tt.body))
				}),
			)
			defer srv.Close()

			err := probeSettings(context.Background(), srv.URL, "csl.this")
			if gotHost != "csl.this" || gotPath != proxy.SettingsPath {
				t.Errorf("probe sent Host %q path %q, want csl.this %s",
					gotHost, gotPath, proxy.SettingsPath)
			}
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("err = %v, want nil", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("err = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

// probeProxy runs probeSettings as csl.this against a real proxy built from
// routeMap, so the 502 bodies are the ones the daemon sends.
func probeProxy(t *testing.T, routeMap map[string]string, settingsHandler http.Handler) error {
	t.Helper()
	h, err := proxy.New(routeMap, nil, nil, "", settingsHandler)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	return probeSettings(context.Background(), srv.URL, "csl.this")
}

// A daemon that loaded another routes file has no route for the host doctor
// picked; that is no reason to suspect an old binary.
func TestProbeSettingsNoRoute(t *testing.T) {
	err := probeProxy(t, map[string]string{}, http.NotFoundHandler())
	if err == nil || !strings.Contains(err.Error(), "no route for csl.this") {
		t.Errorf("err = %v, want the missing route named", err)
	}
	if err != nil && strings.Contains(err.Error(), "predate") {
		t.Errorf("err = %v, want no stale-binary hint", err)
	}
}

// A serve built before the endpoint proxies the path; with the backend down
// that is a bare 502, which still points at the old binary.
func TestProbeSettingsOldDaemonBackendDown(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	backend := strings.TrimPrefix(dead.URL, "http://")
	dead.Close()
	err := probeProxy(t, map[string]string{"csl.this": backend}, nil)
	if err == nil || !strings.Contains(err.Error(), "returned 502") ||
		!strings.Contains(err.Error(), "predate") {
		t.Errorf("err = %v, want a 502 with the stale-binary hint", err)
	}
}

// The proxy answers the sites list only on a configured host, so proxy-answers
// must send one: probing a real proxy as a route host passes, and a daemon
// without that route is named as running another routes file.
func TestProbeSites(t *testing.T) {
	h, err := proxy.New(map[string]string{"csl.this": "127.0.0.1:9"}, nil, nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	if err := probeSites(context.Background(), srv.URL, "csl.this"); err != nil {
		t.Errorf("configured host: err = %v, want nil", err)
	}
	err = probeSites(context.Background(), srv.URL, "present.this")
	if err == nil || !strings.Contains(err.Error(), "no route for present.this") {
		t.Errorf("unrouted host: err = %v, want the missing route named", err)
	}
}

func TestProbeSitesOtherServer(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	err := probeSites(context.Background(), srv.URL, "csl.this")
	if err == nil || !strings.Contains(err.Error(), "something other than d-man") {
		t.Errorf("err = %v, want another process named", err)
	}
}

func TestProbeSitesNotRunning(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	err := probeSites(context.Background(), url, "csl.this")
	if err == nil || !strings.Contains(err.Error(), "not answering") {
		t.Errorf("err = %v, want the daemon reported down", err)
	}
}

// probeHost names the first route's host, is empty for a routes file with no
// route (only a blocklist), and skips when the file does not parse, because
// config-loads already fails for that.
func TestProbeHost(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name       string
		file       string
		body       string
		wantHost   string
		wantStatus string
	}{
		{
			name:       "no routes",
			file:       "blocklist.toml",
			body:       "blocklist = [\"ads.example.com\"]\n",
			wantStatus: doctor.StatusOK,
		},
		{
			name:       "one route",
			file:       "routes.toml",
			body:       "[[route]]\n  name = \"csl\"\n  port = 7420\n",
			wantHost:   "csl.this",
			wantStatus: doctor.StatusOK,
		},
		{
			name:       "does not parse",
			file:       "broken.toml",
			body:       "suffix = \n",
			wantStatus: doctor.StatusSkipped,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := write(t, dir, tt.file, tt.body)
			var host string
			report := doctor.Collect(context.Background(), []doctor.Check{{
				Name: "probe-host",
				Run: func(context.Context) error {
					var err error
					host, err = probeHost(path)
					return err
				},
			}})
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if got := report.Checks[0]; got.Status != tt.wantStatus {
				t.Errorf("status = %q (%s), want %q", got.Status, got.Detail, tt.wantStatus)
			}
		})
	}
}

// With no route configured, proxy-answers takes d-man's own no-route 502 as
// proof that d-man holds the port, and still flags a down daemon and any other
// server, including one that answers 502 itself.
func TestProbeUnrouted(t *testing.T) {
	h, err := proxy.New(map[string]string{}, nil, []string{"ads.example.com"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	dman := httptest.NewServer(h)
	defer dman.Close()
	other := httptest.NewServer(http.NotFoundHandler())
	defer other.Close()
	gateway := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}),
	)
	defer gateway.Close()
	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()

	tests := []struct {
		name    string
		baseURL string
		wantErr string
	}{
		{name: "d-man holds the port", baseURL: dman.URL},
		{name: "another server", baseURL: other.URL, wantErr: "something other than d-man"},
		{
			name:    "another server answering 502",
			baseURL: gateway.URL,
			wantErr: "something other than d-man",
		},
		{name: "nothing listening", baseURL: downURL, wantErr: "not answering"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := probeUnrouted(context.Background(), tt.baseURL)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("err = %v, want nil", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("err = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestProbeSettingsNotRunning(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	err := probeSettings(context.Background(), url, "csl.this")
	if err == nil || !strings.Contains(err.Error(), "not answering") {
		t.Errorf("err = %v, want the daemon reported down", err)
	}
}
