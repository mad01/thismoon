package tman

import (
	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/tools/t-man/internal/platform/launchd"
)

// UserLogsDir is the home-relative directory holding per-service log
// directories for user agents; `add` defaults a service's logs to
// <home>/<UserLogsDir>/<name> when --logs is not given.
const UserLogsDir = "Library/Logs"

// Facts returns the mechanical facts rendered into OperatingDoc and error
// hints. BaseURL stays empty: t-man is a CLI over launchd, with no backing
// service and no MCP server.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "t-man",
		Bin:       "t-man",
		Purpose:   "declarative manager for macOS launchd services",
		StorePath: "~/" + launchd.UserLaunchAgentsDir,
		LogPath:   "~/" + UserLogsDir + "/<name>",
	}
}
