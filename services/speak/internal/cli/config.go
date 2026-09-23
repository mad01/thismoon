package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/config"
)

var configOutput string

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the provider config speak resolves",
	Long: `Show the provider config: which file was read, which provider block is
active, and every configured block with the problem that keeps it from being
used, if any. API keys are never printed.

Without a config file speak uses one implicit local provider (the Kokoro
engine on this machine). The file format is in 'speak docs' and config.md.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		if configOutput == "json" {
			return writeJSON(cmd.OutOrStdout(), cfg)
		}
		return writeConfigText(cmd.OutOrStdout(), cfg)
	},
}

var configActiveCmd = &cobra.Command{
	Use:   "active",
	Short: "Show the active provider",
	Long: `Show the active provider block: name, type, model, default voice, base
URL, whether text leaves this machine (remote), and its problem if any.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		active := cfg.ActiveProvider()
		if configOutput == "json" {
			return writeJSON(cmd.OutOrStdout(), active)
		}
		return writeProviders(cmd.OutOrStdout(), cfg.Active, []config.Provider{active})
	},
}

var configEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "List the environment variables the active provider reads",
	Long: `Print the names (never the values) of the environment variables the active
provider block reads, one per line: its API key variable and, for a proxy,
the variable holding its base URL. The spawn wrapper beside the speak recipe
(speak-env.sh) extracts exactly these from the secrets file, so nothing else
in that file reaches speak. Prints nothing for the local engine.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		for _, name := range cfg.ActiveProvider().Env {
			fmt.Fprintln(cmd.OutOrStdout(), name)
		}
		return nil
	},
}

func init() {
	configCmd.AddCommand(configEnvCmd)
	configCmd.PersistentFlags().
		StringVarP(&configOutput, "output", "o", "text", "Output format: text or json")
	configCmd.AddCommand(configActiveCmd)
	rootCmd.AddCommand(configCmd)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeConfigText(w io.Writer, cfg config.Config) error {
	fmt.Fprintf(w, "config: %s\nactive: %s\n\n", describeSource(cfg), cfg.Active)
	return writeProviders(w, cfg.Active, cfg.Providers)
}

// writeProviders renders one line per provider block, the active one marked.
func writeProviders(w io.Writer, active string, providers []config.Provider) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "\tNAME\tTYPE\tMODEL\tVOICE\tWHERE\tSTATUS")
	for _, p := range providers {
		mark := " "
		if p.Name == active {
			mark = "*"
		}
		where := "local"
		switch {
		case p.BaseURL == "":
			where = "?" // no base URL yet; its problem says where it should come from
		case p.Remote:
			where = "remote"
		}
		status := "ready"
		if p.Problem != "" {
			status = p.Problem
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			mark, p.Name, p.Type, p.Model, p.Voice, where, status)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("%s config: write: %w", speak.Component, err)
	}
	return nil
}
