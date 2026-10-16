package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// inboxServer answers every events query with one item addressed to s1.
func inboxServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"id": "0001", "title": "rebase done", "message": "feat/x is ready",
			"tags": map[string]string{"to": "s1", "from": "orchestrator"},
		}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func watchOn() (config.Config, error) {
	return config.Config{Watch: config.Watch{Enabled: true, PollIntervalMS: 100, MaxWaitS: 1}}, nil
}

// TestRunWatch pins the entrypoint's contract: an item addressed to the
// session is written to stderr and reported as a wake; every other path
// stays silent and reports none.
func TestRunWatch(t *testing.T) {
	tests := []struct {
		name     string
		load     Load
		payload  string
		wantWake bool
	}{
		{"item for the session wakes it", watchOn, `{"session_id":"s1"}`, true},
		{"watch disabled by default", defaults, `{"session_id":"s1"}`, false},
		{"unreadable config", failing, `{"session_id":"s1"}`, false},
		{"payload without a session id", watchOn, `{"cwd":"/x"}`, false},
		{"malformed payload", watchOn, "not json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("EVENTS_BASE_URL", inboxServer(t).URL)
			var stderr bytes.Buffer

			woke := RunWatch(context.Background(), tt.load, strings.NewReader(tt.payload), &stderr)

			if woke != tt.wantWake {
				t.Errorf("RunWatch() woke = %v, want %v", woke, tt.wantWake)
			}
			want := ""
			if tt.wantWake {
				want = "belt[watch]: from orchestrator: rebase done\nfeat/x is ready\n"
			}
			if stderr.String() != want {
				t.Errorf("stderr = %q, want %q", stderr.String(), want)
			}
		})
	}
}
