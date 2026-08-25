package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/mad01/thismoon/tools/t-man/internal/platform/launchd"
	"github.com/mad01/thismoon/tools/t-man/internal/procstat"
	"github.com/mad01/thismoon/tools/t-man/internal/service"
	"github.com/spf13/cobra"
)

var listResources bool

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all managed services",
	Long:    `List all services managed by t-man with their current status.`,
	RunE:    runList,
}

func init() {
	listCmd.Flags().
		BoolVar(&listResources, "resources", false, "Include PID, RSS, CPU%, and uptime columns")
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

	if listResources {
		return printListWithResources(getContext(), manager, services)
	}

	// Print table header
	fmt.Printf("%-30s %-15s %-50s\n", "NAME", "STATUS", "COMMAND")
	fmt.Println(strings.Repeat("-", 95))

	// Print each service
	for _, svc := range services {
		status := serviceStatus(manager, svc.Name)
		fmt.Printf("%-30s %-15s %-50s\n", svc.Name, status, formatCommand(svc, 50))
	}

	return nil
}

// printListWithResources prints the list table with PID, RSS, CPU%, and
// uptime columns resolved from one launchctl list call and one ps call.
func printListWithResources(
	ctx context.Context,
	manager *launchd.Manager,
	services []*service.Definition,
) error {
	stats, err := collectServiceStats(ctx, manager)
	if err != nil {
		return err
	}

	fmt.Printf(
		"%-30s %-12s %6s %8s %6s %8s  %-40s\n",
		"NAME", "STATUS", "PID", "RSS", "CPU%", "UPTIME", "COMMAND",
	)
	fmt.Println(strings.Repeat("-", 118))

	for _, svc := range services {
		status := serviceStatus(manager, svc.Name)
		pid, rss, cpu, uptime := "-", "-", "-", "-"
		if st, ok := stats[svc.Name]; ok {
			pid = fmt.Sprintf("%d", st.PID)
			rss = procstat.FormatRSS(st.RSSBytes)
			cpu = fmt.Sprintf("%.1f", st.CPUPercent)
			uptime = procstat.FormatUptime(st.Uptime)
		}
		fmt.Printf(
			"%-30s %-12s %6s %8s %6s %8s  %-40s\n",
			svc.Name, status, pid, rss, cpu, uptime, formatCommand(svc, 40),
		)
	}

	return nil
}

// collectServiceStats resolves each managed service's PID from launchctl and
// its resource usage from ps, keyed by service name. Services without a
// running process are absent from the result.
func collectServiceStats(
	ctx context.Context,
	manager *launchd.Manager,
) (map[string]procstat.Stats, error) {
	pids, err := manager.RunningPIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve service pids: %w", err)
	}

	pidList := make([]int, 0, len(pids))
	for _, pid := range pids {
		pidList = append(pidList, pid)
	}

	stats, err := procstat.NewCollector().Collect(ctx, pidList)
	if err != nil {
		return nil, fmt.Errorf("failed to collect process stats: %w", err)
	}

	byName := make(map[string]procstat.Stats, len(stats))
	for label, pid := range pids {
		if st, ok := stats[pid]; ok {
			byName[label] = st
		}
	}
	return byName, nil
}

// serviceStatus returns a service's status string, "unknown" when the lookup
// fails.
func serviceStatus(manager *launchd.Manager, name string) string {
	status, err := manager.Status(getContext(), name)
	if err != nil {
		return "unknown"
	}
	return status
}

// formatCommand joins a service's command and args, truncated to maxLen.
func formatCommand(svc *service.Definition, maxLen int) string {
	command := svc.Command
	if len(svc.Args) > 0 {
		command = fmt.Sprintf("%s %s", command, strings.Join(svc.Args, " "))
	}
	if len(command) > maxLen {
		command = command[:maxLen-3] + "..."
	}
	return command
}
