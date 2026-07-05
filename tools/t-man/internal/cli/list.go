package cli

import (
	"fmt"
	"strings"

	"github.com/mad01/thismoon/tools/t-man/internal/platform/launchd"
	"github.com/spf13/cobra"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all managed services",
	Long:    `List all services managed by t-man with their current status.`,
	RunE:    runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	// Check for sudo if needed
	if err := checkSudo(); err != nil {
		return err
	}

	// Create manager
	manager := launchd.NewManager(GetVersion(), !daemonMode)

	// Get all services
	services, err := manager.List(getContext())
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}

	if len(services) == 0 {
		fmt.Println("No services found")
		return nil
	}

	// Print table header
	fmt.Printf("%-30s %-15s %-50s\n", "NAME", "STATUS", "COMMAND")
	fmt.Println(strings.Repeat("-", 95))

	// Print each service
	for _, svc := range services {
		// Get status
		status, err := manager.Status(getContext(), svc.Name)
		if err != nil {
			status = "unknown"
		}

		// Format command with args
		command := svc.Command
		if len(svc.Args) > 0 {
			command = fmt.Sprintf("%s %s", command, strings.Join(svc.Args, " "))
		}

		// Truncate long commands
		if len(command) > 50 {
			command = command[:47] + "..."
		}

		fmt.Printf("%-30s %-15s %-50s\n", svc.Name, status, command)
	}

	return nil
}
