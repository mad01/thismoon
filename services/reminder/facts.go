package reminder

import (
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultPort is the port `reminder serve` listens on when REMINDER_PORT and
// --port are both unset. The server binds to loopback only.
const DefaultPort = 7428

// DefaultWorkdir is the store directory when REMINDER_WORKDIR is unset. The
// leading ~ is expanded at runtime by the CLI, never at build time.
const DefaultWorkdir = "~/.local/share/reminder"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "reminder",
		Bin:       "reminder",
		Purpose:   "time-based reminders that fire a native macOS notification at their due time",
		BaseURL:   fmt.Sprintf("http://localhost:%d", DefaultPort),
		StorePath: DefaultWorkdir,
		HasDoctor: true,
	}
}
