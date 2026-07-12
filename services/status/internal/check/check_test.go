package check

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/status/internal/discover"
)

func portOf(t *testing.T, ts *httptest.Server) int {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOverHTTP(t *testing.T) {
	tests := []struct {
		name   string
		code   int
		wantUp bool
	}{
		{"ok", http.StatusOK, true},
		{"not found still up", http.StatusNotFound, true},
		{"server error is down", http.StatusInternalServerError, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.code)
			}))
			defer ts.Close()
			res := Service(context.Background(), discover.Service{Label: "x", Port: portOf(t, ts)})
			if res.Up != tt.wantUp {
				t.Errorf("up = %v (%s), want %v", res.Up, res.Detail, tt.wantUp)
			}
		})
	}
}

func TestOverHTTPConnectionRefused(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	port := portOf(t, ts)
	ts.Close() // free the port; the probe must fail
	res := Service(context.Background(), discover.Service{Label: "x", Port: port})
	if res.Up {
		t.Errorf("expected down on connection refused, got up (%s)", res.Detail)
	}
}

func TestFromLaunchctl(t *testing.T) {
	running := "system/d-man = {\n\tactive count = 1\n\tstate = running\n\tpid = 1234\n}\n"
	if res := FromLaunchctl(running); !res.Up || res.Detail != "pid 1234" {
		t.Errorf("running output misclassified: %+v", res)
	}
	stopped := "gui/501/x = {\n\tstate = not running\n}\n"
	if res := FromLaunchctl(stopped); res.Up {
		t.Errorf("stopped output misclassified: %+v", res)
	}
	if res := FromLaunchctl(""); res.Up {
		t.Errorf("empty output (unknown service) must be down: %+v", res)
	}
}

func TestVersionToken(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"short sha", "224d220\n", "224d220"},
		{"dev fallback", "dev\n", "dev"},
		{"semver", "v1.2.3\n", "v1.2.3"},
		{"padded", "  abc1234  \n", "abc1234"},
		{"multi line is not a version", "usage: foo\nversion\n", ""},
		{"spaces are not a version", "foo version 1.2\n", ""},
		{"empty", "", ""},
		{"too long", strings.Repeat("a", 65), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VersionToken(tt.in); got != tt.want {
				t.Errorf("VersionToken(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBinaryVersion(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake")
	script := "#!/bin/sh\necho abc1234\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := BinaryVersion(context.Background(), bin); got != "abc1234" {
		t.Errorf("BinaryVersion = %q, want abc1234", got)
	}
	if got := BinaryVersion(context.Background(), filepath.Join(dir, "missing")); got != "" {
		t.Errorf("BinaryVersion(missing) = %q, want empty", got)
	}
	if got := BinaryVersion(context.Background(), ""); got != "" {
		t.Errorf("BinaryVersion(empty path) = %q, want empty", got)
	}
}
