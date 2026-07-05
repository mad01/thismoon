package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Version = "dev"

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
	rootCmd.MarkFlagsMutuallyExclusive("agent", "daemon")
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

// getContext returns a context for the current command execution
func getContext() context.Context {
	return context.Background()
}

// GetVersion returns the current version
func GetVersion() string {
	return Version
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
