package prs

import (
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultPort is the port `prs serve` listens on when PRS_PORT and --port are
// both unset. The server binds to loopback only.
const DefaultPort = 7427

// DefaultWorkdir is the cache directory when PRS_WORKDIR is unset. The
// leading ~ is expanded at runtime by the CLI, never at build time.
const DefaultWorkdir = "~/.local/share/prs"

// DefaultConfigPath is where serve looks for the YAML config when PRS_CONFIG
// and --config are both unset.
const DefaultConfigPath = "~/.config/prs/config.yaml"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "prs",
		Bin:       "prs",
		Purpose:   "open pull requests across every locally checked-out repo, polled from their GitHub hosts",
		BaseURL:   fmt.Sprintf("http://localhost:%d", DefaultPort),
		StorePath: DefaultWorkdir,
		HasDoctor: true,
	}
}
