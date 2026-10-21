package cli

import (
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	agentsync "github.com/mad01/thismoon/tools/agent-sync"
	"github.com/mad01/thismoon/tools/agent-sync/internal/apply"
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
	"github.com/mad01/thismoon/tools/agent-sync/internal/settings"
)

func (a *app) settingsCmd() *cobra.Command {
	return a.sectionCommand(section{
		name:  config.SectionSettings,
		short: "Merge the template settings.json and the local overlay into Claude Code's settings",
		long: `Merge the template settings.json the config names into Claude Code's live
settings file, then merge the local overlay (settings.local.json) on top.

The template wins on conflicts: objects merge key by key, arrays and scalars
are replaced, and keys only the live file has are kept, except the retired
key paths settings.prune lists, which are deleted first. The overlay merges
last, with permissions.allow, deny, ask and additionalDirectories unioned
instead of replaced. A symlinked live file becomes a regular file.

check is the drift gate for 'ralph doctor': its first line is a one-line
summary, and it exits 1 when sync would change the live file.`,
		run: func(file *config.File, mode apply.Mode, out apply.Streams) error {
			cfg, err := settings.ParseConfig(file.Settings)
			if err != nil {
				return agentdoc.Hint(err, agentsync.Facts())
			}
			return settings.Run(cfg, mode, out)
		},
	})
}
