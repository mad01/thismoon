package worklog

import "github.com/mad01/thismoon/kit/agentdoc"

// DefaultRoot is the store directory when WORKLOG_DIR is unset. The leading ~
// is expanded at runtime by the store, never at build time.
const DefaultRoot = "~/code/worklog"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints. BaseURL stays empty: worklog is CLI +
// MCP only, with no backing service.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "worklog",
		Bin:       "worklog",
		Purpose:   "resumable cross-session work state keyed by ticket or topic, not working directory",
		StorePath: DefaultRoot,
	}
}
