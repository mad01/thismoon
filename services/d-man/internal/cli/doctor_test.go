package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestProbeSettingsNotRunning(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	err := probeSettings(context.Background(), url, "csl.this")
	if err == nil || !strings.Contains(err.Error(), "not answering") {
		t.Errorf("err = %v, want the daemon reported down", err)
	}
}
