package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/pr/internal/server"
)

var Version = "dev"

const defaultPort = 7427

var (
	flagPort    int
	flagConfig  string
	flagWorkdir string
)

var rootCmd = &cobra.Command{
	Use:   "pr",
	Short: "Local PR review dashboard",
	Long: `pr aggregates open pull requests from configured GitHub repos
(github.com and GitHub Enterprise hosts) and serves a triage
dashboard at http://pr.this/.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the PR dashboard HTTP server",
	RunE: func(_ *cobra.Command, _ []string) error {
		return server.Serve(server.Options{
			Port:       flagPort,
			ConfigPath: expandTilde(flagConfig),
			Workdir:    expandTilde(flagWorkdir),
			Version:    Version,
		})
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
	defaultConfig := "~/.config/pr/config.toml"
	if v := os.Getenv("PR_CONFIG"); v != "" {
		defaultConfig = v
	}
	defaultWorkdir := "~/.local/share/pr"
	if v := os.Getenv("PR_WORKDIR"); v != "" {
		defaultWorkdir = v
	}
	serveCmd.Flags().IntVar(&flagPort, "port", resolvedDefaultPort(),
		"port the HTTP server listens on (env PR_PORT)")
	serveCmd.Flags().StringVar(&flagConfig, "config", defaultConfig,
		"path to config file (env PR_CONFIG)")
	serveCmd.Flags().StringVar(&flagWorkdir, "workdir", defaultWorkdir,
		"directory for persistent state (env PR_WORKDIR)")
	versionCmd.Flags().StringP("output", "o", "", "output format (json)")
	rootCmd.AddCommand(serveCmd, versionCmd)
}

func Execute() error { return rootCmd.Execute() }

func resolvedDefaultPort() int {
	if v := os.Getenv("PR_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			return p
		}
	}
	return defaultPort
}

func expandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
