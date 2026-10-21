// Package cli wires the agent-sync commands: one command family per config
// section (settings, mods, mcp), each with sync, plan and check, plus
// doctor, docs and version.
package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/envdefault"
	agentsync "github.com/mad01/thismoon/tools/agent-sync"
	"github.com/mad01/thismoon/tools/agent-sync/internal/apply"
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
)

// Main runs agent-sync with args and returns the process exit code. Errors
// print once to stderr as "error: <message>"; a command that already
// reported (check on drift) exits with its own code and prints nothing more.
func Main(args []string, stdout, stderr io.Writer) int {
	c := root()
	c.SetArgs(args)
	c.SetOut(stdout)
	c.SetErr(stderr)
	return apply.Main(stderr, c.Execute())
}

// app carries what the root command resolves once for every subcommand:
// where the config file lives.
type app struct{ configPath string }

func root() *cobra.Command {
	a := &app{}
	c := &cobra.Command{
		Use:           agentsync.Name,
		Short:         "Sync coding agent config from a data repo into the live files",
		Long:          rootLong(),
		Version:       buildinfo.Get().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.PersistentFlags().StringVar(&a.configPath, "config",
		envdefault.String(agentsync.ConfigEnv, ""),
		"config file (default "+defaultConfigPath()+")")
	c.AddCommand(
		versionCmd(),
		agentcli.DocsCommand(agentsync.OperatingDoc, agentsync.Facts()),
		a.doctorCmd(),
		a.settingsCmd(),
		a.modsCmd(),
		a.mcpCmd(),
	)
	return c
}

func rootLong() string {
	return fmt.Sprintf(`agent-sync merges the coding agent config a data repo carries (a template
settings.json, the mods to install, the MCP servers to register) into the
live files on this machine. Each section of the config has its own command
with three modes: sync applies, plan prints what sync would change, check
exits 1 when sync would change anything.

  config  %s (override with --config or $%s)

'agent-sync doctor' checks the config and the paths it names; 'agent-sync
docs' prints the operating doc.`, defaultConfigPath(), agentsync.ConfigEnv)
}

// defaultConfigPath is the compiled default for help text. A home directory
// that will not resolve is a real error everywhere it matters, but help text
// is not one of those places.
func defaultConfigPath() string {
	p, err := config.Path("")
	if err != nil {
		return "~/.config/" + agentsync.Name + "/" + agentsync.ConfigFileName
	}
	return p
}

// load reads the config file this invocation points at. Errors carry the
// doctor hint: a missing or broken config is what doctor explains.
func (a *app) load() (*config.File, error) {
	path, err := config.Path(a.configPath)
	if err != nil {
		return nil, agentdoc.Hint(err, agentsync.Facts())
	}
	f, err := config.Load(path)
	if errors.Is(err, config.ErrNotFound) {
		return nil, fmt.Errorf("%w (create it, or point --config or $%s at one; "+
			"'%s docs' shows an example)", err, agentsync.ConfigEnv, agentsync.Name)
	}
	if err != nil {
		return nil, agentdoc.Hint(err, agentsync.Facts())
	}
	return f, nil
}
