package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/mcptest"

	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/server/servertest"
)

// noChecks satisfies Config.Checks where the doctor tool is not under test.
func noChecks(context.Context) []doctor.Check { return nil }

// TestToolAnnotationsFollowTheContract builds the real MCP server and checks
// the tool list against the shared annotation gate, so a tool added without
// its read-only and open-world hints fails here rather than shipping with
// the SDK defaults.
func TestToolAnnotationsFollowTheContract(t *testing.T) {
	s, err := New("test", Config{Port: 1, Checks: noChecks})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	mcptest.VerifyToolAnnotations(t, s)
	for _, tool := range mcptest.ListTools(t, s) {
		if !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q is not read-only; moondb records through hooks, never MCP", tool.Name)
		}
	}
}

func TestNewRequiresChecks(t *testing.T) {
	if _, err := New("test", Config{Port: 1}); err == nil {
		t.Fatal("want an error when no doctor checks are configured")
	}
}

// session runs the MCP server over a client pointed at the real HTTP handler
// on the fixture store, and returns a connected MCP client session.
func session(t *testing.T, checks func(context.Context) []doctor.Check) *mcp.ClientSession {
	t.Helper()
	st := servertest.NewStore()
	ts := httptest.NewServer(server.New(st, servertest.NewQuerier(st), buildinfo.Info{}).Handler())
	t.Cleanup(ts.Close)
	s := newServer(
		"test",
		&handlers{client: client.New(ts.URL), webURL: "http://moondb.test", checks: checks},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// call invokes a tool and decodes its structured content into out. It also
// checks the text content carries the same JSON, which is what a client that
// ignores structured content reads.
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any, out any) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s returned a tool error: %s", name, toolText(res))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("%s: marshal structured content: %v", name, err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("%s: decode structured content: %v (%s)", name, err, raw)
	}
	if !strings.Contains(toolText(res), `"summary"`) && name != "moondb_doctor" {
		t.Errorf("%s text content lacks the summary: %s", name, toolText(res))
	}
}

func toolText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestWhyRoundTrip(t *testing.T) {
	cs := session(t, noChecks)
	var out whyOutput
	call(t, cs, "moondb_why", map[string]any{
		"worktree": servertest.Worktree, "path": servertest.PathA, "line": 1,
	}, &out)
	if out.Result.Edit == nil || out.Result.Edit.ID != servertest.EditA {
		t.Fatalf("result = %+v", out.Result)
	}
	if !strings.Contains(out.Summary, servertest.NewTurn) ||
		!strings.Contains(out.Summary, "content") {
		t.Errorf("summary = %q, want the turn id and method", out.Summary)
	}
	if out.URL != "http://moondb.test/?session="+servertest.NewSession {
		t.Errorf("url = %q", out.URL)
	}
}

func TestWhyUnexplainedLine(t *testing.T) {
	cs := session(t, noChecks)
	var out whyOutput
	call(
		t,
		cs,
		"moondb_why",
		map[string]any{"repo": servertest.Repo, "path": "nope.go", "line": 9},
		&out,
	)
	if out.Result.Edit != nil ||
		!strings.HasPrefix(out.Summary, "no recorded edit explains line 9") {
		t.Errorf("out = %+v", out)
	}
}

func TestHistoryRoundTrip(t *testing.T) {
	cs := session(t, noChecks)
	var out editsOutput
	call(
		t,
		cs,
		"moondb_history",
		map[string]any{"repo": servertest.Repo, "path": servertest.PathB},
		&out,
	)
	if len(out.Edits) != 1 || out.Edits[0].ID != servertest.EditB {
		t.Fatalf("edits = %+v", out.Edits)
	}
	if !strings.HasPrefix(out.Summary, "1 edits of b.go") {
		t.Errorf("summary = %q", out.Summary)
	}
}

func TestTurnRoundTripCarriesPrompt(t *testing.T) {
	cs := session(t, noChecks)
	var out turnOutput
	call(t, cs, "moondb_turn", map[string]any{"id": servertest.NewTurn}, &out)
	if out.Turn.ID != servertest.NewTurn || len(out.Turn.Edits) != 2 {
		t.Fatalf("turn = %+v", out.Turn)
	}
	if out.Prompt != servertest.Prompt {
		t.Errorf("prompt = %q, want the chunk's text", out.Prompt)
	}
	if !strings.Contains(out.Summary, `prompt: "add the server"`) {
		t.Errorf("summary = %q, want the prompt's first line", out.Summary)
	}
}

func TestTurnNotFoundIsToolError(t *testing.T) {
	cs := session(t, noChecks)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "moondb_turn", Arguments: map[string]any{"id": "nope"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError || !strings.Contains(toolText(res), "not found") {
		t.Errorf("result = %+v, want a not-found tool error", res)
	}
}

func TestSessionsRoundTrip(t *testing.T) {
	cs := session(t, noChecks)
	var out sessionsOutput
	call(t, cs, "moondb_sessions", map[string]any{"repo": servertest.Repo, "limit": 10}, &out)
	if len(out.Sessions) != 1 || out.Sessions[0].ID != servertest.NewSession {
		t.Fatalf("sessions = %+v", out.Sessions)
	}
	if out.Sessions[0].EditCount != 2 || !strings.HasPrefix(out.Summary, "1 sessions") {
		t.Errorf("out = %+v", out)
	}
}

func TestDoctorRoundTripReportsFailuresAsResults(t *testing.T) {
	checks := func(context.Context) []doctor.Check {
		return []doctor.Check{
			{Name: "service-reachable", Run: func(context.Context) error { return nil }},
			{
				Name: "store-ping",
				Run:  func(context.Context) error { return errors.New("disk gone") },
			},
		}
	}
	cs := session(t, checks)
	var out doctor.Report
	call(t, cs, "moondb_doctor", nil, &out)
	if out.OK || len(out.Checks) != 2 || out.Checks[1].Detail != "disk gone" {
		t.Errorf("report = %+v, want one failing check", out)
	}
}

func TestTruncateKeepsUTF8(t *testing.T) {
	got := truncate("héllo", 2)
	if got != "h…" {
		t.Errorf("truncate = %q, want the split rune dropped", got)
	}
	if truncate("short", 10) != "short" {
		t.Error("truncate must leave a short string alone")
	}
}
