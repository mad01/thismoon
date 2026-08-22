package status

import (
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultPort is the port serve listens on when STATUS_PORT and --port are
// both unset.
const DefaultPort = 7426

// DefaultWorkdir is the uptime-history directory when STATUS_WORKDIR is
// unset. The leading ~ is expanded at runtime by the CLI, never at build
// time.
const DefaultWorkdir = "~/.local/share/status"

// Facts returns the mechanical facts rendered into OperatingDoc and error
// hints.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "status",
		Bin:       "status",
		Purpose:   "status page for t-man-managed local services with uptime history and stale-binary detection",
		BaseURL:   fmt.Sprintf("http://localhost:%d", DefaultPort),
		StorePath: DefaultWorkdir,
		HasDoctor: true,
	}
}
