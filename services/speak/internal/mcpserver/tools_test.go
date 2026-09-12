package mcpserver

import (
	"context"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/mcptest"
)

// TestToolAnnotationsFollowTheContract builds the real MCP server and checks
// the tool list against the shared annotation gate, so a tool added without
// its read-only/destructive/open-world hints fails here rather than shipping
// with the SDK defaults. StateDir points at a temp directory: New builds a
// playback engine, which reaps stale audio under it on construction.
func TestToolAnnotationsFollowTheContract(t *testing.T) {
	s, err := New("test", Config{
		TTSURL:   "http://127.0.0.1:0",
		StateDir: t.TempDir(),
		Checks:   func(context.Context) []doctor.Check { return nil },
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	mcptest.VerifyToolAnnotations(t, s)
}
