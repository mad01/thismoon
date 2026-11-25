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

	"github.com/mad01/thismoon/kit/firstlaunchtest"
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

// The lines are the ones `launchctl print` gives for a t-man job started
// with --every 1m, between two runs and during one.
func TestScheduledFromLaunchctl(t *testing.T) {
	const idle = "gui/501/poll = {\n\ttype = LaunchAgent\n\tstate = not running\n" +
		"\truns = 965\n\tlast exit code = %s\n\trun interval = 60 seconds\n}\n"
	tests := []struct {
		name       string
		out        string
		wantUp     bool
		wantDetail string
	}{
		{"idle after a clean run", strings.Replace(idle, "%s", "0", 1), true, "idle, last run ok"},
		{"idle before the first run", strings.Replace(idle, "%s", "(never exited)", 1), true, "idle, no run yet"},
		{"last run failed", strings.Replace(idle, "%s", "1", 1), false, "last run exited 1"},
		{"last run failed with a named code", strings.Replace(idle, "%s", "78: EX_CONFIG", 1), false, "last run exited 78: EX_CONFIG"},
		{"mid run", "gui/501/poll = {\n\tstate = running\n\tpid = 4321\n\tlast exit code = 1\n}\n", true, "pid 4321"},
		{"unknown to launchd", "", false, "not loaded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := ScheduledFromLaunchctl(tt.out)
			if res.Up != tt.wantUp || res.Detail != tt.wantDetail {
				t.Errorf("got %+v, want up=%v detail=%q", res, tt.wantUp, tt.wantDetail)
			}
		})
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
	// Under a full make test the fake's first launch can outlast the 3 s
	// probe timeout, so take it here, untimed.
	firstlaunchtest.Warm(t, bin, []string{"version"}, nil)
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
