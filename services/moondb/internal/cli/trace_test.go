package cli

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"

	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/server/servertest"
)

// traceClient is a client of the real server handler over the fixture
// store, whose older session has one traced turn.
func traceClient(t *testing.T) *client.Client {
	t.Helper()
	st := servertest.NewStore()
	ts := httptest.NewServer(server.New(st, servertest.NewQuerier(st), buildinfo.Info{}).Handler())
	t.Cleanup(ts.Close)
	return client.New(ts.URL)
}

func TestSessionTraceResolvesShortIDs(t *testing.T) {
	c := traceClient(t)
	tests := []struct {
		name    string
		id      string
		want    string
		wantErr string
	}{
		{name: "full id", id: servertest.OldSession, want: servertest.OldSession},
		{name: "unique prefix", id: "sess-o", want: servertest.OldSession},
		{name: "ambiguous prefix", id: "sess-", wantErr: `"sess-" matches 2 sessions`},
		{name: "no match", id: "nope", wantErr: `no session "nope"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := sessionTrace(context.Background(), c, tt.id)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if st.SessionID != tt.want {
				t.Errorf("session = %q, want %q", st.SessionID, tt.want)
			}
		})
	}
}

func TestPrintTrace(t *testing.T) {
	st, err := sessionTrace(context.Background(), traceClient(t), servertest.OldSession)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	printTrace(&out, st)
	// The fixture turn runs two minutes: the Edit call took one, and the
	// two model calls on claude-opus-5-5 cost $0.01444 between them. The
	// reviewer's two calls under it cost $0.0114.
	for _, want := range []string{
		"session sess-old, 1 turn:",
		"wall    2m00s",
		"cost    $0.026  main $0.014, agents $0.011",
		"calls   6       main: model 2, tool 1 (0 failed); agents: model 2, tool 1",
		"AGENTS",
		"main      -                1      2m00s     3      1      1.5k    $0.014",
		"reviewer  general-purpose  1      2m00s     3      0      2.2k    $0.011",
		"LONGEST TOOL CALLS",
		"1     main      Edit  1m00s     ok",
		"1     reviewer  Read  30.0s     ok",
		"MOST EXPENSIVE MODEL CALLS",
		"1     main      claude-opus-5-5  $0.014",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}
