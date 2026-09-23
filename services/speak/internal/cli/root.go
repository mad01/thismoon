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
// pointing at different providers, ports, or state directories.
var (
	flagPort     int
	flagConfig   string
	flagProvider string
	flagTTSURL   string
	flagStateDir string
)

var rootCmd = &cobra.Command{
	Use:   "speak",
	Short: "Read markdown aloud over localhost",
	Long: fmt.Sprintf(`speak serves a local page where you upload a markdown file, see it
rendered inline, and play it section by section, at http://localhost:%d
(speak.this with d-man). It also serves an OpenAI-compatible
/v1/audio/speech endpoint, so other pages on this machine (present, csl) can
fetch speech from it.

Speech comes from the provider the config file selects: the local Kokoro
engine (mlx-audio) by default, or OpenRouter, OpenAI or a LiteLLM proxy
(see 'speak config' and 'speak docs'). serve and mcp are two surfaces over
the same provider and neither needs the other: serve plays audio in the
browser, mcp plays it on this machine's speakers.`, speak.DefaultPort),
	// An error from a subcommand is a diagnosis, not a usage mistake; main
	// prints it once.
	SilenceUsage:  true,
	SilenceErrors: true,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the local HTTP server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		p := activeProvider(cmd.Context())
		return web.Serve(flagPort, p, p.NewHealth(), buildinfo.Get())
	},
}

func init() {
	rootCmd.PersistentFlags().
		IntVar(&flagPort, "port", envdefault.Int("SPEAK_PORT", speak.DefaultPort),
			"port the HTTP server listens on, and the one doctor probes (env SPEAK_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config",
		envdefault.String(speak.ConfigEnv, defaultConfigPath()),
		"provider config file (env "+speak.ConfigEnv+")")
	rootCmd.PersistentFlags().StringVar(&flagProvider, "provider",
		envdefault.String(speak.ProviderEnv, ""),
		"provider block to use, overriding the config's provider line (env "+speak.ProviderEnv+")")
	rootCmd.PersistentFlags().StringVar(&flagTTSURL, "tts-url",
		envdefault.String("SPEAK_TTS_URL", ""),
		"base URL of the local mlx-audio engine, overriding local blocks' base_url (default "+
			speak.DefaultTTSURL+"; env SPEAK_TTS_URL)")
	rootCmd.PersistentFlags().StringVar(&flagStateDir, "state-dir", defaultStateDir(),
		"directory holding playback audio and the playback lock (env SPEAK_STATE_DIR)")
	// Expand a leading ~ before any subcommand runs: SPEAK_STATE_DIR reaches
	// Go without shell expansion under launchd, and an unexpanded ~ would
	// create a literal "~" directory beside the working directory.
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		for _, path := range []*string{&flagStateDir, &flagConfig} {
			expanded, err := confdir.Expand(*path)
			if err != nil {
				return err
			}
			*path = expanded
		}
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

// defaultConfigPath is the config file inside speak's config directory. A
// home directory that cannot be resolved leaves the ~-prefixed path, which
// PersistentPreRunE then fails on with that as the cause.
func defaultConfigPath() string {
	path, err := confdir.Path(speak.Component, speak.ConfigFileName)
	if err != nil {
		return "~/.config/" + speak.Component + "/" + speak.ConfigFileName
	}
	return path
}

// defaultStateDir resolves where playback state lives: SPEAK_STATE_DIR when
// set, otherwise the XDG state directory — except that an install which has
// played audio into speak.LegacyStateDir keeps using it, so an upgrade does
// not strand its lock and audio cache. The probe is what makes that check
// mean "state is here" rather than "some directory is here": the TTS
// engine's virtualenv shares the legacy path. A home directory that cannot
// be resolved leaves the ~-prefixed legacy path here; PersistentPreRunE
// then fails with that as the cause instead of writing somewhere
// cwd-relative.
func defaultStateDir() string {
	if v := envdefault.String("SPEAK_STATE_DIR", ""); v != "" {
		return v
	}
	dir, err := confdir.StateDir(speak.Component, confdir.LegacyDir{
		Dir:   speak.LegacyStateDir,
		Probe: speak.LegacyStateProbe,
	})
	if err != nil {
		return speak.LegacyStateDir
	}
	return dir
}
