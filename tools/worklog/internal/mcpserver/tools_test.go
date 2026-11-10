package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/tools/worklog/internal/store"
)

func TestBuildZeroHintEmptyStore(t *testing.T) {
	s := store.New(t.TempDir())

	hint := buildZeroHint(s, "the status/repo filter matched none")
	if hint == nil {
		t.Fatal("want a hint for an empty store")
	}
	if hint.ItemsStored != 0 {
		t.Errorf("items_stored = %d, want 0", hint.ItemsStored)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "holds no items") {
		t.Errorf("notes = %v, want the empty-store note", hint.Notes)
	}
}

func TestBuildZeroHintFilterMiss(t *testing.T) {
	s := store.New(t.TempDir())
	if _, err := s.Checkpoint("ABC-1234", store.CheckpointInput{
		Where: "state", Note: "n", Repo: "a",
	}); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	hint := buildZeroHint(s, `no key or content contains "zebra"`)
	if hint == nil {
		t.Fatal("want a hint")
	}
	if hint.ItemsStored != 1 {
		t.Errorf("items_stored = %d, want 1", hint.ItemsStored)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "1 items stored") {
		t.Errorf("notes = %v, want the filter-miss note with the store count", hint.Notes)
	}
}

// connect serves the tools over an in-memory transport against a temp store
// and a config path with no file behind it, so no remote is ever pushed to.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	t.Setenv("WORKLOG_DIR", t.TempDir())
	s := New("test", filepath.Join(t.TempDir(), "config.yaml"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// checkpoint calls worklog_checkpoint with args and returns the result.
func checkpoint(t *testing.T, cs *mcp.ClientSession, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "worklog_checkpoint", Arguments: args,
	})
	if err != nil {
		t.Fatalf("worklog_checkpoint: %v", err)
	}
	return res
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// TestCheckpointTakesStatus is the reported failure: the input schema
// refused a status argument, so a closing checkpoint could not mark done.
func TestCheckpointTakesStatus(t *testing.T) {
	cs := connect(t)

	res := checkpoint(t, cs, map[string]any{"key": "MAD-1", "note": "opened"})
	var v itemView
	decode(t, res, &v)
	if v.Status != store.StatusActive {
		t.Fatalf("new item status = %q, want active", v.Status)
	}

	res = checkpoint(t, cs, map[string]any{"key": "MAD-1", "note": "shipped", "status": "done"})
	decode(t, res, &v)
	if v.Status != store.StatusDone {
		t.Errorf("status after a done checkpoint = %q, want done", v.Status)
	}

	res = checkpoint(t, cs, map[string]any{"key": "MAD-1", "note": "again"})
	decode(t, res, &v)
	if v.Status != store.StatusDone {
		t.Errorf("status after a checkpoint without status = %q, want it kept as done", v.Status)
	}
}

func TestCheckpointRejectsUnknownStatus(t *testing.T) {
	cs := connect(t)
	res := checkpoint(t, cs, map[string]any{"key": "MAD-2", "note": "x", "status": "blocked"})
	if !res.IsError || !strings.Contains(text(res), "active, paused, or done") ||
		!strings.Contains(text(res), `"blocked"`) {
		t.Errorf("result = %s, want a tool error naming the valid statuses", text(res))
	}
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "worklog_show", Arguments: map[string]any{"key": "MAD-2"},
	})
	if err != nil || !res.IsError {
		t.Errorf("a rejected checkpoint created the item: %s, %v", text(res), err)
	}
}

func decode(t *testing.T, res *mcp.CallToolResult, out any) {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool error: %s", text(res))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
}

// TestToolAnnotationContract holds every registered tool to the repo-wide
// annotation rules: a spec-legal name, a description, an explicit open-world
// hint, and a destructive hint on anything that writes. New only registers
// tools, so an empty config path never gets resolved here.
func TestToolAnnotationContract(t *testing.T) {
	mcptest.VerifyToolAnnotations(t, New("test", ""))
}
