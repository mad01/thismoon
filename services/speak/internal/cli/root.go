package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/speak/internal/web"
)

// Version is injected at build time via -ldflags.
var Version = "dev"

const defaultPort = 7425

var (
	flagPort   int
	flagTTSURL string
)

var rootCmd = &cobra.Command{
	Use:   "speak",
	Short: "Read markdown aloud over localhost",
	Long: `speak serves a local page where you upload a markdown file, see it
rendered inline, and play it section by section. It also fronts the local
Kokoro TTS engine (mlx-audio) with a CORS-enabled OpenAI-compatible
/v1/audio/speech endpoint, so other local pages (present, csl) can fetch
speech from http://speak.this.`,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the local HTTP server",
	RunE: func(_ *cobra.Command, _ []string) error {
		return web.Serve(flagPort, flagTTSURL, Version)
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the build version",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if out, _ := cmd.Flags().GetString("output"); out == "json" {
			return json.NewEncoder(os.Stdout).Encode(map[string]string{"version": Version})
		}
		fmt.Println(Version)
		return nil
	},
}

func init() {
	serveCmd.Flags().IntVar(&flagPort, "port", resolvedDefaultPort(),
		"port the HTTP server listens on (env SPEAK_PORT)")
	serveCmd.Flags().StringVar(&flagTTSURL, "tts-url", resolvedTTSURL(),
		"base URL of the mlx-audio TTS server (env SPEAK_TTS_URL)")
	versionCmd.Flags().StringP("output", "o", "", "output format (json)")
	rootCmd.AddCommand(serveCmd, versionCmd)
}

func Execute() error {
	return rootCmd.Execute()
}

func resolvedDefaultPort() int {
	if v := os.Getenv("SPEAK_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			return p
		}
	}
	return defaultPort
}

func resolvedTTSURL() string {
	if v := os.Getenv("SPEAK_TTS_URL"); v != "" {
		return v
	}
	return "http://127.0.0.1:8765"
}
