package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

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

	if anyScheduled(services) {
		return printListWithSchedule(getContext(), manager, services, time.Now())
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

// anyScheduled reports whether the list holds at least one scheduled job,
// which is when the schedule columns earn their width.
func anyScheduled(services []*service.Definition) bool {
	for _, svc := range services {
		if svc.Scheduled() {
			return true
		}
	}
	return false
}

// printListWithSchedule prints the list table with NEXT RUN, LAST RUN, and
// EXIT columns, so a scheduled job's idle state reads as waiting for its
// next run rather than as down. Long-lived services show "-" in them.
func printListWithSchedule(
	ctx context.Context,
	manager *launchd.Manager,
	services []*service.Definition,
	now time.Time,
) error {
	fmt.Printf(scheduleRowFormat, "NAME", "STATUS", "NEXT RUN", "LAST RUN", "EXIT", "COMMAND")
	fmt.Println(strings.Repeat("-", 124))

	for _, svc := range services {
		state, err := manager.RunState(ctx, svc)
		if err != nil {
			state = nil
		}
		fmt.Print(formatScheduleRow(svc, state, now))
	}

	return nil
}

// scheduleRowFormat lays out one line of the schedule-aware list table.
const scheduleRowFormat = "%-30s %-12s %-16s %-16s %4s  %-40s\n"

// formatScheduleRow renders one service's line of the schedule-aware list.
// state is nil when launchd could not be asked, which prints as unknown.
func formatScheduleRow(svc *service.Definition, state *launchd.RunState, now time.Time) string {
	status, exit := "unknown", "-"
	if state != nil {
		status = state.Status
		if svc.Scheduled() {
			exit = formatExitCode(state.LastExitCode)
		}
	}
	return fmt.Sprintf(
		scheduleRowFormat,
		svc.Name, status, formatNextRun(svc, now), formatLastRun(svc), exit,
		formatCommand(svc, 40),
	)
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
