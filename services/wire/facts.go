package wire

import (
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultPort is the port `wire serve` listens on when WIRE_PORT and --port
// are both unset. The server binds to loopback only.
const DefaultPort = 7432

// DefaultWorkdir is the store directory when WIRE_WORKDIR is unset. The
// leading ~ is expanded at runtime by the CLI, never at build time.
const DefaultWorkdir = "~/.local/share/wire"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "wire",
		Bin:       "wire",
		Purpose:   "message bus between agent sessions: named channels with blocking reads",
		BaseURL:   fmt.Sprintf("http://localhost:%d", DefaultPort),
		StorePath: DefaultWorkdir,
	}
}
