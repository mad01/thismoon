package agentsync

import "github.com/mad01/thismoon/kit/agentdoc"

// Defaults shared by every section. A section's own defaults (the live
// settings file, say) live as constants in its package.
const (
	// Name is the component name and the config directory under
	// $XDG_CONFIG_HOME or ~/.config.
	Name = "agent-sync"
	// ConfigFileName is the config file inside that directory.
	ConfigFileName = "config.yaml"
	// ConfigEnv relocates the config file; --config overrides it.
	ConfigEnv = "AGENT_SYNC_CONFIG"
)

// Facts returns the mechanical facts rendered into OperatingDoc and the
// error hints. BaseURL and StorePath stay empty: agent-sync has no serve
// process and no store of its own. Each run reads the config, merges data
// into the live files it names, and exits.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      Name,
		Bin:       Name,
		Purpose:   "syncs coding agent config (Claude Code settings, mods, MCP servers) from a data repo into the live files",
		HasDoctor: true,
	}
}
