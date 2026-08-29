package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
)

var (
	// Global flags
	agentMode  bool
	daemonMode bool
	dryRun     bool
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "t-man",
	Short: "t-man - Service manager for macOS launchd",
	Long: `t-man is a service manager for macOS that provides a simple CLI
for managing launchd services with declarative configuration.

Compatible with serviceman CLI for easy migration.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	// Global flags available to all commands
	rootCmd.PersistentFlags().BoolVar(&agentMode, "agent", true, "Run as user agent (LaunchAgent)")
	rootCmd.PersistentFlags().
		BoolVar(&daemonMode, "daemon", false, "Run as system daemon (LaunchDaemon) - requires sudo")
	rootCmd.PersistentFlags().
		BoolVar(&dryRun, "dryrun", false, "Dry run mode - show what would be done without applying changes")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		daemon, err := modeFlags{
			agent:     agentMode,
			daemon:    daemonMode,
			agentSet:  cmd.Flags().Changed("agent"),
			daemonSet: cmd.Flags().Changed("daemon"),
		}.daemonSelected()
		if err != nil {
			return err
		}
		daemonMode, agentMode = daemon, !daemon
		return nil
	}
}

// modeFlags is the --agent/--daemon pair as parsed, carrying whether each was
// spelled out on the command line.
type modeFlags struct {
	agent, daemon       bool
	agentSet, daemonSet bool
}

// daemonSelected collapses the pair into the single value every command reads.
// --agent and --daemon are one choice spelled two ways — t-man keeps --agent
// for serviceman compatibility — so --agent=false selects daemon mode, and a
// pair that agrees on the same value contradicts itself.
func (m modeFlags) daemonSelected() (bool, error) {
	switch {
	case m.agentSet && m.daemonSet && m.agent == m.daemon:
		return false, fmt.Errorf(
			"--agent=%t and --daemon=%t contradict each other: pass one of them",
			m.agent, m.daemon,
		)
	case m.agentSet && !m.daemonSet:
		return !m.agent, nil
	default:
		return m.daemon, nil
	}
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

// getContext returns a context for the current command execution
func getContext() context.Context {
	return context.Background()
}

// GetVersion returns the build's version token, the value stamped into a
// managed plist's TManMetadata.
func GetVersion() string {
	return buildinfo.Get().Version
}

// IsDryRun returns whether dry run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// IsUserMode returns whether user mode is enabled
func IsUserMode() bool {
	return !daemonMode
}

// checkSudo verifies that the command is running with appropriate permissions
func checkSudo() error {
	if daemonMode && os.Geteuid() != 0 {
		return fmt.Errorf("system daemon mode requires sudo/root privileges")
	}
	return nil
}
