package cli

import (
	"bytes"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/events/internal/event"
	"github.com/mad01/thismoon/services/events/internal/server"
	"github.com/mad01/thismoon/services/events/internal/store"
)

// TestListWindowFlags runs `events list` against a real serve handler: one
// event two days old and one an hour old, so --since and --after-id each
// have something to drop, and a bad value surfaces serve's 400 message.
func TestListWindowFlags(t *testing.T) {
	clock := time.Now().UTC().Add(-48 * time.Hour)
	st, err := store.New(t.TempDir(), 0, 0, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	old, err := st.Append(event.Event{Source: "deps", Title: "old scan"})
	if err != nil {
		t.Fatal(err)
	}
	clock = time.Now().UTC().Add(-time.Hour)
	recent, err := st.Append(event.Event{Source: "deps", Title: "recent scan"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.New(st, buildinfo.Info{}).Handler())
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		args    []string
		want    []string // substrings the output must carry
		notWant []string
		wantErr []string // substrings the error must carry
	}{
		{
			name:    "since span",
			args:    []string{"--since", "24h"},
			want:    []string{"recent scan"},
			notWant: []string{"old scan"},
		},
		{
			name:    "after id",
			args:    []string{"--after-id", old.ID},
			want:    []string{"recent scan"},
			notWant: []string{"old scan"},
		},
		{
			name: "after newest id",
			args: []string{"--after-id", recent.ID},
			want: []string{"no events"},
		},
		{
			name:    "bad since",
			args:    []string{"--since", "yesterday"},
			wantErr: []string{"since must be", "after_id"},
		},
		{
			name:    "bad after id",
			args:    []string{"--after-id", "24h"},
			wantErr: []string{"after_id must be", "since=24h"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// cobra keeps flag values between Execute calls; reset the two
			// under test so one case's value never leaks into the next.
			listSince, listAfterID = "", ""
			var out bytes.Buffer
			rootCmd.SetOut(&out)
			rootCmd.SetArgs(append([]string{"list", "--port", u.Port()}, tc.args...))
			err := rootCmd.Execute()
			if len(tc.wantErr) > 0 {
				if err == nil {
					t.Fatalf("list %v succeeded with %q; want an error", tc.args, out.String())
				}
				for _, w := range tc.wantErr {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("error %q does not mention %q", err, w)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("list %v: %v", tc.args, err)
			}
			for _, w := range tc.want {
				if !strings.Contains(out.String(), w) {
					t.Errorf("list %v output %q lacks %q", tc.args, out.String(), w)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(out.String(), w) {
					t.Errorf("list %v output %q carries %q", tc.args, out.String(), w)
				}
			}
		})
	}
}
