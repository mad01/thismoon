package mcpserver

import (
	"testing"

	"github.com/mad01/thismoon/kit/mcptest"
)

// TestToolAnnotations is the annotation gate: every registered tool must state
// whether it writes and whether it reaches outside this machine, so a client
// deciding what to auto-approve reads a hint rather than a tool name.
func TestToolAnnotations(t *testing.T) {
	mcptest.VerifyToolAnnotations(t, New("test"))
}
