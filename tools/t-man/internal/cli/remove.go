package cli

import (
	"fmt"

	"github.com/mad01/thismoon/tools/t-man/internal/platform/launchd"
	"github.com/spf13/cobra"
)

// removeCmd represents the remove command
var removeCmd = &cobra.Command{
	Use:     "remove <service-name>",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a service",
	Long:    `Remove a service by name. This will unload and delete the service.`,
	Args:    cobra.ExactArgs(1),
	RunE:    runRemove,
}

func init() {
	rootCmd.AddCommand(removeCmd)
}

func runRemove(cmd *cobra.Command, args []string) error {
	// Check for sudo if needed
	if err := checkSudo(); err != nil {
		return err
	}

	serviceName := args[0]

	if dryRun {
		fmt.Printf("Dry run mode - would remove service: %s\n", serviceName)
		return nil
	}

	// Create manager
	manager := launchd.NewManager(GetVersion(), !daemonMode)

	// Delete the service
	if err := manager.Delete(getContext(), serviceName); err != nil {
		return fmt.Errorf("failed to remove service: %w", err)
	}

	fmt.Printf("✓ Service '%s' removed\n", serviceName)
	return nil
}
