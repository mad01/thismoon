package csl

import "github.com/mad01/thismoon/kit/agentdoc"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints. The paths and the base URL are
// resolved at call time, not compiled in: where state lives depends on
// whether this install predates the config/state split, and where the web
// UI listens depends on CSL_PORT, so a doc built from constants would name
// a location the binary does not use.
func Facts() agentdoc.Facts {
	state := stateDirForDocs()
	baseURL := BaseURLForPort(ResolvedPort())
	return agentdoc.Facts{
		Name:          "csl",
		Bin:           "csl",
		Purpose:       "local code search over your git checkouts (zoekt lexical, semantic, hybrid)",
		BaseURL:       baseURL,
		StorePath:     state,
		LogPath:       state + "/" + DaemonLogFile,
		HasDoctor:     true,
		MCPNote:       "Tools search the local index via an auto-started daemon with in-process fallback; nothing must be running. The web UI (default " + baseURL + ") is separate.",
		MCPDoctorTool: "csl_doctor",
	}
}
