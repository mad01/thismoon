package mcptest

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type noInput struct{}

type echoOutput struct {
	OK bool `json:"ok"`
}

func echo(context.Context, *mcp.CallToolRequest, noInput) (*mcp.CallToolResult, echoOutput, error) {
	return nil, echoOutput{OK: true}, nil
}

// TestVerifyToolAnnotations pins the helper's happy path: a read-only tool
// and an additive write, both with an explicit open-world hint, pass. The
// failure paths call t.Errorf and cannot be exercised without faking
// testing.TB, which its unexported method forbids.
func TestVerifyToolAnnotations(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "stub", Version: "test"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "stub_read",
		Description: "Read something.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, echo)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "stub_write",
		Description: "Append something.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false), OpenWorldHint: new(false)},
	}, echo)
	VerifyToolAnnotations(t, s)

	if got := len(ListTools(t, s)); got != 2 {
		t.Fatalf("ListTools: got %d tools, want 2", got)
	}
}
