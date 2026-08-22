package kof

import (
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultPort is the port `kof serve` listens on when KOF_PORT and --port are
// both unset. The server binds to loopback only.
const DefaultPort = 7431

// DefaultWorkdir is the store directory when KOF_WORKDIR is unset. The
// leading ~ is expanded at runtime by the CLI, never at build time.
const DefaultWorkdir = "~/.local/share/kof"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "keeper-of-facts",
		Bin:       "kof",
		Purpose:   "assertion store for evidence-pinned claims about how systems behave",
		BaseURL:   fmt.Sprintf("http://localhost:%d", DefaultPort),
		StorePath: DefaultWorkdir,
		HasDoctor: true,
	}
}
