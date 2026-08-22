package csl

import "github.com/mad01/thismoon/kit/agentdoc"

// DefaultBaseURL is the web UI's base URL when web.base_url is unset in
// config.yaml: `csl web` binds loopback on its default port. A csl.this
// front is machine-private d-man wiring layered over this default.
const DefaultBaseURL = "http://127.0.0.1:7424"

// DefaultStateDir is where every piece of csl state lives: config.yaml, the
// lexical and semantic indexes, the search server's socket/pid/log, and the
// reindex queue. The leading ~ is expanded at runtime, never at build time.
const DefaultStateDir = "~/.config/csl"

// DefaultDaemonLog is the background search server's rotated log file.
const DefaultDaemonLog = DefaultStateDir + "/search-daemon.log"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "csl",
		Bin:       "csl",
		Purpose:   "local code search over your git checkouts (zoekt lexical, semantic, hybrid)",
		BaseURL:   DefaultBaseURL,
		StorePath: DefaultStateDir,
		LogPath:   DefaultDaemonLog,
		HasDoctor: true,
		MCPNote:   "Tools search the local index via an auto-started daemon with in-process fallback; nothing must be running. The web UI (default " + DefaultBaseURL + ") is separate.",
	}
}
