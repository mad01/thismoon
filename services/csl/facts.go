package csl

import "github.com/mad01/thismoon/kit/agentdoc"

// parallelCallsNote asks clients to batch independent calls into one turn,
// or independent searches into one csl_search_multi call, which clients that
// defer tools would not otherwise find. It names the two tools that write:
// every other csl tool sets ReadOnlyHint, and clients run read-only calls
// from one turn concurrently.
const parallelCallsNote = "Every tool but csl_repo_pull and csl_repo_reindex is read-only and safe to run concurrently: " +
	"send independent searches and reads as parallel calls in one turn, " +
	"or batch several searches, or a search plus a read of its top file, into one csl_search_multi call; " +
	"only a call that needs an earlier result belongs in the next turn."

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
		MCPNote:       "Tools search the local index via an auto-started daemon with in-process fallback; nothing must be running. The web UI (default " + baseURL + ") is separate. " + parallelCallsNote,
		MCPDoctorTool: "csl_doctor",
	}
}
