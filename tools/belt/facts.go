package belt

import "github.com/mad01/thismoon/kit/agentdoc"

// DefaultConfigPath is the belt-owned config file. The leading ~ is expanded
// at runtime, never at build time; internal/config.DefaultPaths resolves the
// same location, and a test pins the two together.
const DefaultConfigPath = "~/.config/belt/config.yaml"

// Facts returns the mechanical facts rendered into OperatingDoc. belt is
// CLI-only: no backing service (BaseURL empty) and no log file (denies and
// hints emit best-effort to the events service via internal/notify).
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "belt",
		Bin:       "belt",
		Purpose:   "Claude Code guard and hint hooks that deny risky tool calls and add advisory context",
		StorePath: DefaultConfigPath,
		HasDoctor: true,
	}
}
