package opener

import "github.com/mad01/thismoon/kit/agentdoc"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints. BaseURL and StorePath stay empty:
// opener has no serve process and no store — every run execs the system
// open command and exits. The binary is named opener, not open, so it can
// never shadow /usr/bin/open on a PATH that puts ~/code/bin first.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:    "opener",
		Bin:     "opener",
		Purpose: "macOS open bridge: URLs, files, and apps to their handlers, plus Finder reveal",
		MCPNote: "Tools exec /usr/bin/open inside this stdio process (no backing service); macOS only.",
	}
}
