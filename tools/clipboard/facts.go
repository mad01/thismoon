package clipboard

import "github.com/mad01/thismoon/kit/agentdoc"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints. BaseURL and StorePath stay empty:
// clipboard has no serve process and no store — every run execs the system
// pasteboard commands and exits, and the pasteboard itself is the only state.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:    "clipboard",
		Bin:     "clipboard",
		Purpose: "macOS system clipboard bridge: copy text to and paste text from the pasteboard",
		MCPNote: "Tools run pbcopy/pbpaste inside this stdio process (no backing service); macOS only.",
	}
}
