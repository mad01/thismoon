package present

import (
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultWorkdir is the page store directory when PRESENT_WORKDIR is unset.
// The leading ~ is expanded at runtime by the CLI, never at build time.
const DefaultWorkdir = "~/.config/present"

// DefaultPort is the port `present serve` listens on and tool URLs point at
// when PRESENT_PORT is unset.
const DefaultPort = 7423

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	baseURL := fmt.Sprintf("http://localhost:%d", DefaultPort)
	return agentdoc.Facts{
		Name:      "present",
		Bin:       "present",
		Purpose:   "scrollable briefing pages agents publish and keep editing, served on localhost",
		BaseURL:   baseURL,
		StorePath: DefaultWorkdir,
		HasDoctor: true,
		MCPNote:   "Tools read and write the page store directly; present serve (default " + baseURL + ") only serves the page URLs the tools return.",
	}
}
