package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tman "github.com/mad01/thismoon/tools/t-man"
	"github.com/mad01/thismoon/tools/t-man/internal/platform/launchd"
	"github.com/mad01/thismoon/tools/t-man/internal/reconcile"
	"github.com/mad01/thismoon/tools/t-man/internal/service"
	"github.com/spf13/cobra"
)

var (
	addName           string
	addDesc           string
	addWorkdir        string
	addPath           string
	addEnv            []string
	addLogDir         string
	addSandboxProfile string
	addExtraLogs      []string
	addSchedule       scheduleFlags
)

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add [flags] -- <command> [args...]",
	Short: "Add or update a service",
	Long: `Add or update a service with the specified configuration.

This command is compatible with serviceman CLI syntax:
  t-man add --name myservice -- /path/to/executable arg1 arg2

The command and its arguments must come after the -- separator. A command path
may start with ~/, which expands to your home directory.

Without a schedule the service is long-lived: launchd starts it at load and
restarts it whenever it exits. With --schedule, --calendar, or --every it is a
scheduled job instead: launchd runs it at those times and lets it exit, and
"scheduled" is its normal status between runs.`,
	Example: `  # Add a simple service
  t-man add --name myapp -- /usr/local/bin/myapp

  # Add with working directory and environment
  t-man add --name webapp --workdir /var/www --path /usr/local/bin -- node server.js

  # Add with custom log directory
  t-man add --name myservice --logs /var/log/myservice -- /usr/bin/myapp

  # Run a job every morning at 07:30, and on Fridays at 17:00 too
  t-man add --name digest --schedule 07:30,fri@17:00 -- /usr/local/bin/digest

  # Office days only: a weekday set, or a range (ranges may wrap: fri-mon)
  t-man add --name standup --schedule weekdays@08:50 -- /usr/local/bin/standup
  t-man add --name standup --schedule mon-fri@08:50 -- /usr/local/bin/standup

  # Run a job on the first of every month using launchd's own fields
  t-man add --name rollup --calendar day=1,hour=6,minute=0 -- /usr/local/bin/rollup

  # Every hour on the hour during office hours (a range expands to 45 entries)
  t-man add --name poll --calendar hour=9-17,weekday=1-5,minute=0 -- /usr/local/bin/poll

  # Run a job every hour
  t-man add --name audit --every 1h -- /usr/local/bin/audit`,
	RunE: runAdd,
}

func init() {
	rootCmd.AddCommand(addCmd)

	addCmd.Flags().StringVar(&addName, "name", "", "Service name (required)")
	addCmd.Flags().StringVar(&addDesc, "desc", "", "Service description")
	addCmd.Flags().StringVar(&addWorkdir, "workdir", "", "Working directory")
	addCmd.Flags().StringVar(&addPath, "path", "", "Additional PATH entries (colon-separated)")
	addCmd.Flags().StringArrayVar(&addEnv, "env", []string{}, "Environment variables (KEY=VALUE)")
	addCmd.Flags().
		StringVar(&addLogDir, "logs", "", "Log directory (default: ~/Library/Logs/<name> for user, /var/log/<name> for system)")
	addCmd.Flags().
		StringVar(&addSandboxProfile, "sandbox-profile", "", "Wrap the service in sandbox-exec with this seatbelt profile (.sb file)")
	addCmd.Flags().
		StringArrayVar(&addExtraLogs, "extra-log", []string{}, "Additional named log file (NAME=PATH, repeatable), viewable with 'logs --source NAME'")
	addCmd.Flags().
		StringVar(&addSchedule.clock, "schedule", "", "Run as a scheduled job at HH:MM daily; comma list, optional day prefix: mon@07:30, mon-fri@07:30, weekdays@, weekend@, daily@")
	addCmd.Flags().
		StringArrayVar(&addSchedule.calendar, "calendar", []string{}, "Run as a scheduled job on launchd calendar fields (minute=0,hour=7,day=1,weekday=1,month=1; a field may be a range such as hour=9-17; repeatable)")
	addCmd.Flags().
		StringVar(&addSchedule.every, "every", "", "Run as a scheduled job every interval (Go duration such as 1h or 30m)")

	_ = addCmd.MarkFlagRequired("name")
}

func runAdd(cmd *cobra.Command, args []string) error {
	// Check for sudo if needed
	if err := checkSudo(); err != nil {
		return err
	}

	// Parse command and arguments
	// Everything passed to the command is the actual command + args to run
	if len(args) == 0 {
		return fmt.Errorf(
			"command is required (e.g., t-man add --name myservice -- /path/to/command)",
		)
	}

	commandPath, err := resolveCommandPath(args[0])
	if err != nil {
		return err
	}
	commandArgs := args[1:]

	// Parse environment variables
	envMap := make(map[string]string)
	for _, env := range addEnv {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid environment variable format: %s (expected KEY=VALUE)", env)
		}
		envMap[parts[0]] = parts[1]
	}

	// Add PATH if specified
	if addPath != "" {
		if existingPath, ok := envMap["PATH"]; ok {
			envMap["PATH"] = addPath + ":" + existingPath
		} else {
			envMap["PATH"] = addPath
		}
	}

	// Determine log directory and paths
	logDir := addLogDir
	if logDir == "" {
		if !daemonMode {
			homeDir, _ := os.UserHomeDir()
			logDir = filepath.Join(homeDir, tman.UserLogsDir, addName)
		} else {
			logDir = filepath.Join("/var/log", addName)
		}
	}

	// Create log directory if it doesn't exist
	if !dryRun {
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	stdoutPath := filepath.Join(logDir, "stdout.log")
	stderrPath := filepath.Join(logDir, "stderr.log")

	// Resolve sandbox profile to an absolute path (~ expansion + abs)
	sandboxProfile := addSandboxProfile
	if sandboxProfile != "" {
		resolved, err := expandPath(sandboxProfile)
		if err != nil {
			return fmt.Errorf("failed to resolve sandbox profile path: %w", err)
		}
		sandboxProfile = resolved
	}

	// Parse extra log sources (NAME=PATH)
	extraLogs := make(map[string]string)
	for _, entry := range addExtraLogs {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid extra log format: %s (expected NAME=PATH)", entry)
		}
		path, err := expandPath(parts[1])
		if err != nil {
			return fmt.Errorf("failed to resolve extra log path for %s: %w", parts[0], err)
		}
		extraLogs[parts[0]] = path
	}
	if len(extraLogs) == 0 {
		extraLogs = nil // keep the hash identical to definitions predating extra logs
	}

	// Build service definition
	def := &service.Definition{
		Name:            addName,
		Command:         commandPath,
		Args:            commandArgs,
		WorkingDir:      addWorkdir,
		Environment:     envMap,
		StandardOutPath: stdoutPath,
		StandardErrPath: stderrPath,
		SandboxProfile:  sandboxProfile,
		ExtraLogs:       extraLogs,
	}
	if err := addSchedule.apply(def); err != nil {
		return err
	}

	// A long-lived service starts at load and is restarted by launchd; a
	// scheduled job does neither, launchd fires it from its triggers only
	longLived := !def.Scheduled()
	def.RunAtLoad = longLived
	def.KeepAlive = longLived

	// Validate the definition
	if err := def.Validate(); err != nil {
		return fmt.Errorf("invalid service configuration: %w", err)
	}

	// Record the profile content digest so profile edits reconcile
	if err := def.PopulateSandboxDigest(); err != nil {
		return fmt.Errorf("invalid sandbox profile: %w", err)
	}

	if dryRun {
		fmt.Println("Dry run mode - would perform the following:")
		fmt.Printf("Service: %s\n", def.Name)
		fmt.Printf("Command: %s %s\n", def.Command, strings.Join(def.Args, " "))
		if def.WorkingDir != "" {
			fmt.Printf("Working Directory: %s\n", def.WorkingDir)
		}
		if len(def.Environment) > 0 {
			fmt.Println("Environment:")
			for k, v := range def.Environment {
				fmt.Printf("  %s=%s\n", k, v)
			}
		}
		fmt.Printf("Logs: %s, %s\n", stdoutPath, stderrPath)
		for source, path := range def.ExtraLogs {
			fmt.Printf("Extra Log: %s=%s\n", source, path)
		}
		if def.SandboxProfile != "" {
			fmt.Printf("Sandbox Profile: %s\n", def.SandboxProfile)
		}
		if def.Scheduled() {
			fmt.Printf("Schedule: %s\n", def.ScheduleString())
		}
		return nil
	}

	// Create manager and reconciler
	manager := launchd.NewManager(GetVersion(), !daemonMode)
	reconciler := reconcile.NewReconciler(manager)

	// Reconcile the service
	result, err := reconciler.Reconcile(getContext(), def)
	if err != nil {
		return fmt.Errorf("reconciliation failed: %w", err)
	}

	// Report the result
	switch result.Change.Type {
	case reconcile.ChangeTypeCreate:
		fmt.Printf("✓ Service '%s' created\n", addName)
	case reconcile.ChangeTypeUpdate:
		fmt.Printf("✓ Service '%s' updated\n", addName)
	case reconcile.ChangeTypeNone:
		fmt.Printf("✓ Service '%s' already up to date\n", addName)
	default:
		fmt.Printf("✓ Service '%s' reconciled: %s\n", addName, result.Change.Type)
	}

	return nil
}

// expandPath resolves a user-supplied path to an absolute one, expanding a
// leading ~/ to the home directory
func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to resolve home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[2:])
	}
	return filepath.Abs(path)
}

// resolveCommandPath resolves the command given to add into an absolute path.
// An absolute path is kept as is, a path containing a slash is expanded (a
// leading ~/ becomes the home directory) and made absolute, and a bare name is
// looked up in PATH.
func resolveCommandPath(command string) (string, error) {
	if filepath.IsAbs(command) {
		return command, nil
	}
	if strings.Contains(command, "/") {
		resolved, err := expandPath(command)
		if err != nil {
			return "", fmt.Errorf("failed to resolve command path: %w", err)
		}
		return resolved, nil
	}
	resolved, err := resolveCommand(command)
	if err != nil {
		return "", fmt.Errorf(
			"command not found in PATH: %s (tried: %s, then PATH)",
			command, strings.Join(commonCommandDirs, ", "),
		)
	}
	return resolved, nil
}

// commonCommandDirs are consulted for a bare command name before PATH, in
// this order, so a Homebrew binary wins over a version manager's shim.
var commonCommandDirs = []string{
	"/usr/local/bin",
	"/usr/bin",
	"/bin",
	"/opt/homebrew/bin",
}

// resolveCommand attempts to find a command in commonCommandDirs, then PATH.
func resolveCommand(cmd string) (string, error) {
	for _, dir := range commonCommandDirs {
		fullPath := filepath.Join(dir, cmd)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			return fullPath, nil
		}
	}

	// Try exec.LookPath as fallback
	path := os.Getenv("PATH")
	if path != "" {
		for _, dir := range strings.Split(path, ":") {
			fullPath := filepath.Join(dir, cmd)
			if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
				return fullPath, nil
			}
		}
	}

	return "", fmt.Errorf("command not found in PATH: %s", cmd)
}
