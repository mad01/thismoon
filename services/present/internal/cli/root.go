package cli

import (
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	present "github.com/mad01/thismoon/services/present"
)

var (
	flagWorkdir   string
	flagPort      int
	flagBaseURL   string
	flagSharedURL string
	flagAuthorKey string
)

var rootCmd = &cobra.Command{
	Use:   "present",
	Short: "Serve and manage scrollable briefing pages over localhost",
	Long: `present manages single-page HTML briefing pages (create / read / update /
list; delete is available only in the web index). Pages are authored as
structured JSON and compiled to an HTML fragment at authoring time; the server
serves that fragment and the browser assembles the full page client-side. The
chrome (header, theme, controls) comes from the shared webkit package.

Subcommands:
  serve   Run the local HTTP server that serves pages.
  mcp     Run the MCP stdio server exposing present_* tools to Claude Code.
  share   Push a local page to the shared instance (--shared-url, --author-key).
  key     Mint an author key for the shared instance.`,
	// Let main print the error once; cobra stays quiet on both usage and errors.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir", defaultWorkdir(),
		"directory holding pages/ (env PRESENT_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port",
		envdefault.Int("PRESENT_PORT", present.DefaultPort),
		"port the HTTP server listens on / URLs point at (env PRESENT_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url",
		envdefault.String("PRESENT_BASE_URL", ""),
		"base URL for page links (env PRESENT_BASE_URL); defaults to http://localhost:<port>")
	rootCmd.PersistentFlags().StringVar(&flagSharedURL, "shared-url",
		envdefault.String("PRESENT_SHARED_URL", ""),
		"shared present instance pages can be pushed to, e.g. https://present.example.com (env PRESENT_SHARED_URL)")
	rootCmd.PersistentFlags().StringVar(&flagAuthorKey, "author-key",
		envdefault.String("PRESENT_AUTHOR_KEY", ""),
		"author key sent to the shared instance as a bearer token; prefer the env var (env PRESENT_AUTHOR_KEY)")
	// Expand a leading ~ in the workdir before any subcommand runs. A literal
	// "~/..." reaches Go from PRESENT_WORKDIR or --workdir without shell
	// expansion; left unexpanded the MCP and serve processes resolve different
	// directories and updates land where nothing serves them.
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		workdir, err := confdir.Expand(flagWorkdir)
		if err != nil {
			return err
		}
		flagWorkdir = workdir
		return nil
	}
}

// Execute runs the root command and returns any error for main to print.
func Execute() error {
	return rootCmd.Execute()
}

// defaultWorkdir resolves the pages directory: PRESENT_WORKDIR when set,
// else present.LegacyWorkdir while it still holds a pages/ directory, else
// the XDG state directory ($XDG_STATE_HOME/present, or
// present.DefaultWorkdir). The probe matters because provisioning creates
// the legacy directory for the template and assets alone.
// Pages are never migrated, so an install that has published pages keeps
// reading the directory they are in. An unresolvable home leaves the legacy
// path in place so --help still names one; PersistentPreRunE then reports
// the failure as an error rather than serving an empty store.
func defaultWorkdir() string {
	path, err := confdir.StateDir("present", confdir.LegacyDir{
		Dir:   present.LegacyWorkdir,
		Probe: present.LegacyWorkdirProbe,
	})
	if err != nil {
		path = present.LegacyWorkdir
	}
	return envdefault.String("PRESENT_WORKDIR", path)
}
