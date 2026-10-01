package cli

import (
	"fmt"
	"time"

	"github.com/mad01/thismoon/tools/t-man/internal/platform/launchd"
	"github.com/spf13/cobra"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start <service-name>",
	Short: "Start a service",
	Args:  cobra.ExactArgs(1),
	RunE:  hintRunE(runStart),
}

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop <service-name>",
	Short: "Stop a service",
	Args:  cobra.ExactArgs(1),
	RunE:  hintRunE(runStop),
}

// restartCmd represents the restart command
var restartCmd = &cobra.Command{
	Use:   "restart <service-name>",
	Short: "Restart a service",
	Args:  cobra.ExactArgs(1),
	RunE:  hintRunE(runRestart),
}

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run <service-name>",
	Short: "Run a scheduled job once, now",
	Long: `Run a scheduled job once, outside its schedule, for testing.

Only jobs added with --schedule, --calendar, or --every qualify; a long-lived
service is controlled with start, stop, and restart instead.`,
	Args: cobra.ExactArgs(1),
	RunE: hintRunE(runRun),
}

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status <service-name>",
	Short: "Get status of a service",
	Args:  cobra.ExactArgs(1),
	RunE:  hintRunE(runStatus),
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(statusCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	if err := checkSudo(); err != nil {
		return err
	}

	serviceName := args[0]

	if dryRun {
		fmt.Printf("Dry run mode - would start service: %s\n", serviceName)
		return nil
	}

	manager := launchd.NewManager(GetVersion(), !daemonMode)

	if err := manager.Start(getContext(), serviceName); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	fmt.Printf("✓ Service '%s' started\n", serviceName)
	return nil
}

func runStop(cmd *cobra.Command, args []string) error {
	if err := checkSudo(); err != nil {
		return err
	}

	serviceName := args[0]

	if dryRun {
		fmt.Printf("Dry run mode - would stop service: %s\n", serviceName)
		return nil
	}

	manager := launchd.NewManager(GetVersion(), !daemonMode)

	if err := manager.Stop(getContext(), serviceName); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	fmt.Printf("✓ Service '%s' stopped\n", serviceName)
	return nil
}

func runRestart(cmd *cobra.Command, args []string) error {
	if err := checkSudo(); err != nil {
		return err
	}

	serviceName := args[0]

	if dryRun {
		fmt.Printf("Dry run mode - would restart service: %s\n", serviceName)
		return nil
	}

	manager := launchd.NewManager(GetVersion(), !daemonMode)

	if err := manager.Restart(getContext(), serviceName); err != nil {
		return fmt.Errorf("failed to restart service: %w", err)
	}

	fmt.Printf("✓ Service '%s' restarted\n", serviceName)
	return nil
}

// runRun fires a scheduled job once. launchctl start runs a loaded job
// immediately whatever its triggers say, and a job without KeepAlive exits
// when it is done, so this is the one-shot the schedule would have fired.
func runRun(cmd *cobra.Command, args []string) error {
	if err := checkSudo(); err != nil {
		return err
	}

	serviceName := args[0]
	manager := launchd.NewManager(GetVersion(), !daemonMode)

	svc, err := manager.Get(getContext(), serviceName)
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}
	if !svc.Scheduled() {
		return fmt.Errorf(
			"service '%s' is not a scheduled job; use 't-man start %s' for a long-lived service",
			serviceName, serviceName,
		)
	}

	if dryRun {
		fmt.Printf("Dry run mode - would run job once: %s\n", serviceName)
		return nil
	}

	if err := manager.Start(getContext(), serviceName); err != nil {
		return fmt.Errorf("failed to run job: %w", err)
	}

	fmt.Printf("✓ Job '%s' started; follow it with 't-man logs %s -f'\n", serviceName, serviceName)
	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	if err := checkSudo(); err != nil {
		return err
	}

	serviceName := args[0]

	manager := launchd.NewManager(GetVersion(), !daemonMode)

	// Get service details
	svc, err := manager.Get(getContext(), serviceName)
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	// Get status
	status := "unknown"
	state, err := manager.RunState(getContext(), svc)
	if err == nil {
		status = state.Status
	}

	// Print service information
	fmt.Printf("Service: %s\n", svc.Name)
	fmt.Printf("Status: %s\n", status)
	fmt.Printf("Command: %s", svc.Command)
	if len(svc.Args) > 0 {
		fmt.Printf(" %s", svc.Args)
	}
	fmt.Println()

	if svc.WorkingDir != "" {
		fmt.Printf("Working Directory: %s\n", svc.WorkingDir)
	}

	if len(svc.Environment) > 0 {
		fmt.Println("Environment:")
		for k, v := range svc.Environment {
			fmt.Printf("  %s=%s\n", k, v)
		}
	}

	if svc.StandardOutPath != "" {
		fmt.Printf("Stdout: %s\n", svc.StandardOutPath)
	}
	if svc.StandardErrPath != "" {
		fmt.Printf("Stderr: %s\n", svc.StandardErrPath)
	}

	if len(svc.ExtraLogs) > 0 {
		fmt.Println("Extra logs:")
		for _, source := range sortedKeys(svc.ExtraLogs) {
			fmt.Printf("  %s: %s\n", source, svc.ExtraLogs[source])
		}
	}

	for _, line := range scheduleStatusLines(svc, state, time.Now()) {
		fmt.Println(line)
	}

	fmt.Printf("Run at load: %v\n", svc.RunAtLoad)
	fmt.Printf("Keep alive: %v\n", svc.KeepAlive)

	return nil
}
