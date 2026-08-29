package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	prs "github.com/mad01/thismoon/services/prs"
)

var (
	flagWorkdir string
	flagPort    int
	flagBaseURL string
	flagConfig  string
)

var rootCmd = &cobra.Command{
	Use:   "prs",
	Short: "See every open PR across your locally checked-out repos",
	Long: fmt.Sprintf(`prs is a local open-PR dashboard, served at http://localhost:%d
(prs.this with d-man). It discovers every git repo under the configured
directories (the same discovery csl-style tools use), polls each repo's GitHub
host for open pull requests — github.com and GitHub Enterprise alike,
authenticated through your existing gh logins — and answers "is there anything
I need to act on?" in one place.

serve owns the cache and the polling; the MCP and CLI commands are thin HTTP
clients to it, so serve stays the single writer.

Subcommands:
  serve   Run the HTTP server (web UI + JSON API) that owns the cache and polls.
  mcp     Run the MCP stdio server exposing prs_* tools to Claude Code.`, prs.DefaultPort),
	// An error from a subcommand is a diagnosis, not a usage mistake; main
	// prints it once.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir",
		envdefault.String("PRS_WORKDIR", prs.DefaultWorkdir),
		"directory holding the PR cache (env PRS_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port",
		envdefault.Int("PRS_PORT", prs.DefaultPort),
		"port the HTTP server listens on / the MCP talks to (env PRS_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url",
		envdefault.String("PRS_BASE_URL", ""),
		"human-facing base URL the MCP links to (env PRS_BASE_URL); defaults to http://localhost:<port>")
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", defaultConfigPath(),
		"path to the YAML config (env PRS_CONFIG)")
	// Expand a leading ~ before any subcommand runs: PRS_WORKDIR and
	// PRS_CONFIG reach Go without shell expansion under launchd. Both are
	// assigned only once both expand, so a failure leaves neither half-resolved.
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		workdir, err := confdir.Expand(flagWorkdir)
		if err != nil {
			return err
		}
		config, err := confdir.Expand(flagConfig)
		if err != nil {
			return err
		}
		flagWorkdir, flagConfig = workdir, config
		return nil
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// defaultConfigPath resolves the config path: PRS_CONFIG when set, else the
// config file inside the XDG config directory ($XDG_CONFIG_HOME/prs, else
// ~/.config/prs). The file name comes from prs.DefaultConfigPath so the
// documented default and the resolved one cannot name different files. An
// unresolvable home leaves that documented default in place so --help still
// shows a path; PersistentPreRunE then reports the failure as an error
// instead of reading a config nobody wrote.
func defaultConfigPath() string {
	path, err := confdir.Path("prs", filepath.Base(prs.DefaultConfigPath))
	if err != nil {
		path = prs.DefaultConfigPath
	}
	return envdefault.String("PRS_CONFIG", path)
}
