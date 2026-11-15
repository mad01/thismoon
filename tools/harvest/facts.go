package harvest

import "github.com/mad01/thismoon/kit/agentdoc"

// Defaults the settings fall back to and the docs render.
const (
	// Name is the component and binary name.
	Name = "harvest"
	// DefaultStateDir holds the log and the per-session marks, unless
	// HARVEST_STATE_DIR names another directory.
	DefaultStateDir = "~/.local/state/harvest"
	// LogFile is the log inside the state dir.
	LogFile = "harvest.log"
)

// Facts returns the mechanical facts rendered into OperatingDoc and the
// error hints. BaseURL stays empty: harvest has no serve process. Each call
// is a hook, a hint or one background run, and exits.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      Name,
		Bin:       Name,
		Purpose:   "saves what a Claude Code session learned to the brain vault inbox as raw captures",
		StorePath: DefaultStateDir,
		LogPath:   DefaultStateDir + "/" + LogFile,
		HasDoctor: true,
	}
}
