package present

import (
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultWorkdir is the page store directory a fresh install uses when
// PRESENT_WORKDIR is unset. The leading ~ is expanded at runtime by the CLI,
// never at build time.
const DefaultWorkdir = "~/.local/state/present"

// LegacyWorkdir is where the page store lived before it moved to the XDG
// state directory. An install that already has this directory keeps using
// it — pages are not migrated, so a machine that has published pages must
// not be pointed somewhere else. Only a machine without it lands in
// DefaultWorkdir. The fleet recipe and the MCP's seatbelt profile both name
// this path explicitly, so a supervised process never depends on which
// branch the resolution takes.
const LegacyWorkdir = "~/.config/present"

// DefaultPort is the port `present serve` listens on and tool URLs point at
// when PRESENT_PORT is unset.
const DefaultPort = 7423

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	baseURL := fmt.Sprintf("http://localhost:%d", DefaultPort)
	return agentdoc.Facts{
		Name:          "present",
		Bin:           "present",
		Purpose:       "scrollable briefing pages agents publish and keep editing, served on localhost",
		BaseURL:       baseURL,
		StorePath:     DefaultWorkdir,
		HasDoctor:     true,
		MCPDoctorTool: "present_doctor",
		MCPNote:       "Tools read and write the page store directly; present serve (default " + baseURL + ") only serves the page URLs the tools return.",
	}
}
