package mcpserver

import (
	"testing"

	"github.com/mad01/thismoon/kit/mcptest"
)

// TestToolAnnotationsFollowTheContract builds the real MCP server and checks
// its tool list against the shared annotation gate, so a tool added without
// its read-only/destructive/open-world hints fails here rather than shipping
// with the SDK defaults. New needs nothing beyond a version string: every
// handler constructs its own sysopen.Opener, and none of them runs here.
func TestToolAnnotationsFollowTheContract(t *testing.T) {
	mcptest.VerifyToolAnnotations(t, New("test"))
}

// TestEveryOpenVerbIsRegistered pins the advertised tool set, so dropping or
// renaming a verb breaks the MCP contract here instead of at an agent's call
// site.
func TestEveryOpenVerbIsRegistered(t *testing.T) {
	want := map[string]bool{
		"open_url":         true,
		"open_file":        true,
		"open_app":         true,
		"open_with":        true,
		"reveal_in_finder": true,
	}
	got := map[string]bool{}
	for _, tool := range mcptest.ListTools(t, New("test")) {
		got[tool.Name] = true
	}
	for name := range want {
		if !got[name] {
			t.Errorf("tool %q is not advertised", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("tool %q is advertised but not in the pinned set", name)
		}
	}
}
