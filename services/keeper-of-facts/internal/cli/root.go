package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	kof "github.com/mad01/thismoon/services/keeper-of-facts"
)

var (
	flagWorkdir string
	flagPort    int
	flagBaseURL string
)

var rootCmd = &cobra.Command{
	Use:   "kof",
	Short: "Store assertions about how systems behave, pinned to evidence",
	Long: fmt.Sprintf(`kof (keeper-of-facts) is a local assertion store with evidence
pins, served at http://localhost:%d (kof.this with d-man). An agent session
records a one-sentence assertion and pins it to a line range in a repo working
tree, hashed the moment it is recorded; kof check re-hashes those pins and
flips an assertion stale when the code changed.

serve owns the store and resolves pins; the MCP and CLI mutations are thin HTTP
clients to it, so serve stays the single writer.

Subcommands:
  serve   Run the HTTP server (web UI + JSON API) that owns the store.
  mcp     Run the MCP stdio server exposing kof_* tools to Claude Code.`, kof.DefaultPort),
	// A failed call is a diagnosis ("assertion not found"), not a usage
	// mistake; main prints the error once and nothing dumps the help text.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	// The KEEP_* names are the tool's pre-rename spelling, honored so a
	// half-updated machine keeps working through the keep → keeper-of-facts
	// cutover (MAD-266). Nesting them gives the precedence KOF_*, then
	// KEEP_*, then the compiled-in default.
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir",
		envdefault.String("KOF_WORKDIR", envdefault.String("KEEP_WORKDIR", kof.DefaultWorkdir)),
		"directory holding the assertion log (env KOF_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port",
		envdefault.Int("KOF_PORT", envdefault.Int("KEEP_PORT", kof.DefaultPort)),
		"port the HTTP server listens on / the MCP talks to (env KOF_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url",
		envdefault.String("KOF_BASE_URL", envdefault.String("KEEP_BASE_URL", "")),
		"human-facing base URL the MCP links to (env KOF_BASE_URL); defaults to http://localhost:<port>")
	// Expand a leading ~ in the workdir before any subcommand runs: KOF_WORKDIR
	// reaches Go without shell expansion. serve auto-migrates a pre-rename
	// ~/.local/share/keep store to the new path on start (MAD-269).
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		workdir, err := confdir.Expand(flagWorkdir)
		if err != nil {
			return err
		}
		flagWorkdir = workdir
		return nil
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
