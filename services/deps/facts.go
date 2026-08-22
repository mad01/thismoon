package deps

import (
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultPort is the port `deps serve` listens on when DEPS_PORT and --port
// are both unset. The server binds to loopback only.
const DefaultPort = 7429

// DefaultWorkdir is the scan-store directory when DEPS_WORKDIR is unset. The
// leading ~ is expanded at runtime by the CLI, never at build time.
const DefaultWorkdir = "~/.local/share/deps"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "deps",
		Bin:       "deps",
		Purpose:   "supply-chain scanner checking every catalog repo's pinned dependencies against OSV advisories",
		BaseURL:   fmt.Sprintf("http://localhost:%d", DefaultPort),
		StorePath: DefaultWorkdir,
	}
}
