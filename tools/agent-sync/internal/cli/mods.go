package cli

import (
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	agentsync "github.com/mad01/thismoon/tools/agent-sync"
	"github.com/mad01/thismoon/tools/agent-sync/internal/apply"
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
	"github.com/mad01/thismoon/tools/agent-sync/internal/mods"
)

func (a *app) modsCmd() *cobra.Command {
	return a.sectionCommand(section{
		name:  config.SectionMods,
		short: "Install, enable, update, disable and retire Claude Code mods",
		long: `Reconcile the Claude Code mods the mods file lists, then each drop-in in
the dropins directory, with Claude Code's plugin state, through the claude
plugin CLI at user scope. Each file names one directory marketplace and its
mods. Nothing the files do not name is touched.

A mod set "enabled": true is installed and enabled, and updated when its
install record's version differs from the plugin.json version in its
marketplace folder. "enabled": false disables a mod that is enabled.
"installed": false retires a mod: it is uninstalled when it has an install
record, even after the marketplace stops listing it. "profiles" and "hosts"
gate a mod to some machines; a gated-off mod is left alone.

check is the drift gate for 'ralph doctor': its first line is a one-line
summary, and it exits 1 when sync would run any claude command. A failed
claude command exits 2.`,
		run: func(file *config.File, mode apply.Mode, out apply.Streams) error {
			cfg, err := mods.ParseConfig(file.Mods)
			if err != nil {
				return agentdoc.Hint(err, agentsync.Facts())
			}
			return mods.Run(cfg, mode, out)
		},
	})
}
