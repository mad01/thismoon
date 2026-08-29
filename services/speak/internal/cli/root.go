package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/web"
)

// Resolved once on the root command, so serve, mcp, and doctor cannot end up
// pointing at different engines, ports, or state directories.
var (
	flagPort     int
	flagTTSURL   string
	flagStateDir string
)

var rootCmd = &cobra.Command{
	Use:   "speak",
	Short: "Read markdown aloud over localhost",
	Long: fmt.Sprintf(`speak serves a local page where you upload a markdown file, see it
rendered inline, and play it section by section, at http://localhost:%d
(speak.this with d-man). It also fronts the local Kokoro TTS engine
(mlx-audio) with an OpenAI-compatible /v1/audio/speech endpoint, so other
pages on this machine (present, csl) can fetch speech from it.

serve and mcp are two surfaces over the same engine and neither needs the
other: serve plays audio in the browser, mcp plays it on this machine's
speakers.`, speak.DefaultPort),
	// An error from a subcommand is a diagnosis, not a usage mistake; main
	// prints it once.
	SilenceUsage:  true,
	SilenceErrors: true,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the local HTTP server",
	RunE: func(_ *cobra.Command, _ []string) error {
		return web.Serve(flagPort, flagTTSURL, buildinfo.Get())
	},
}

func init() {
	rootCmd.PersistentFlags().IntVar(&flagPort, "port", envdefault.Int("SPEAK_PORT", speak.DefaultPort),
		"port the HTTP server listens on, and the one doctor probes (env SPEAK_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagTTSURL, "tts-url",
		envdefault.String("SPEAK_TTS_URL", speak.DefaultTTSURL),
		"base URL of the mlx-audio TTS server (env SPEAK_TTS_URL)")
	rootCmd.PersistentFlags().StringVar(&flagStateDir, "state-dir", defaultStateDir(),
		"directory holding playback audio and the playback lock (env SPEAK_STATE_DIR)")
	// Expand a leading ~ before any subcommand runs: SPEAK_STATE_DIR reaches
	// Go without shell expansion under launchd, and an unexpanded ~ would
	// create a literal "~" directory beside the working directory.
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		expanded, err := confdir.Expand(flagStateDir)
		if err != nil {
			return err
		}
		flagStateDir = expanded
		return nil
	}
	rootCmd.AddCommand(serveCmd)
}

func Execute() error {
	return rootCmd.Execute()
}

// serveBaseURL is where `speak serve` answers for the resolved --port: what
// doctor probes and what the MCP doctor tool reports on.
func serveBaseURL() string {
	return fmt.Sprintf("http://localhost:%d", flagPort)
}

// defaultStateDir resolves where playback state lives: SPEAK_STATE_DIR when
// set, otherwise the XDG state directory — except that an install which
// already has speak.DefaultStateDir on disk keeps using it, so an upgrade
// does not strand its lock and audio cache. A home directory that cannot be
// resolved leaves the ~-prefixed default here; PersistentPreRunE then fails
// with that as the cause instead of writing somewhere cwd-relative.
func defaultStateDir() string {
	if v := envdefault.String("SPEAK_STATE_DIR", ""); v != "" {
		return v
	}
	dir, err := confdir.StateDir(speak.Component, speak.DefaultStateDir)
	if err != nil {
		return speak.DefaultStateDir
	}
	return dir
}
