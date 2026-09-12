// Package mcptest holds the shared test gate every MCP-bearing component
// runs over its tool list, so the per-component contract test is one call
// instead of a hand-rolled copy.
package mcptest

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolName is the spec's tool-name rule: 1 to 128 characters drawn from
// letters, digits, underscore, hyphen, and dot.
var toolName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// ListTools connects an in-memory client to s and returns every tool the
// server advertises, in the order tools/list delivers them.
func ListTools(t testing.TB, s *mcp.Server) []*mcp.Tool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := s.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "mcptest", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	var tools []*mcp.Tool
	for tool, err := range clientSession.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		tools = append(tools, tool)
	}
	return tools
}

// VerifyToolAnnotations asserts every tool s advertises carries the
// annotation contract this repo requires: a spec-legal name, a
// description, and annotations that make the read-only and open-world
// decisions explicit. A read-only tool must not claim a destructive or
// idempotent hint (the spec defines both only for writes); a writing tool
// must state its destructive hint rather than inherit the default.
func VerifyToolAnnotations(t testing.TB, s *mcp.Server) {
	t.Helper()
	tools := ListTools(t, s)
	if len(tools) == 0 {
		t.Fatal("server advertises no tools")
	}
	for _, tool := range tools {
		verifyTool(t, tool)
	}
}

func verifyTool(t testing.TB, tool *mcp.Tool) {
	t.Helper()
	if !toolName.MatchString(tool.Name) {
		t.Errorf("tool %q: name must match %s", tool.Name, toolName)
	}
	if tool.Description == "" {
		t.Errorf("tool %q: empty description", tool.Name)
	}
	a := tool.Annotations
	if a == nil {
		t.Errorf("tool %q: no annotations", tool.Name)
		return
	}
	if a.OpenWorldHint == nil {
		t.Errorf("tool %q: openWorldHint must be set explicitly", tool.Name)
	}
	if a.ReadOnlyHint {
		if a.DestructiveHint != nil {
			t.Errorf("tool %q: read-only tool must not set destructiveHint", tool.Name)
		}
		if a.IdempotentHint {
			t.Errorf("tool %q: read-only tool must not set idempotentHint", tool.Name)
		}
		return
	}
	if a.DestructiveHint == nil {
		t.Errorf("tool %q: writing tool must set destructiveHint explicitly", tool.Name)
	}
}
