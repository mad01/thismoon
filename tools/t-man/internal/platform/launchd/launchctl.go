package launchd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// LaunchctlClient provides methods to interact with launchctl
type LaunchctlClient struct {
	execCommand func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewLaunchctlClient creates a new launchctl client
func NewLaunchctlClient() *LaunchctlClient {
	return &LaunchctlClient{
		execCommand: exec.CommandContext,
	}
}

// Load loads a service plist file with the -w flag (write enable)
func (c *LaunchctlClient) Load(ctx context.Context, plistPath string) error {
	cmd := c.execCommand(ctx, "launchctl", "load", "-w", plistPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to load %s: %w (output: %s)", plistPath, err, string(output))
	}
	return nil
}

// Unload unloads a service plist file
func (c *LaunchctlClient) Unload(ctx context.Context, plistPath string) error {
	cmd := c.execCommand(ctx, "launchctl", "unload", plistPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to unload %s: %w (output: %s)", plistPath, err, string(output))
	}
	return nil
}

// Start starts a service by its label
func (c *LaunchctlClient) Start(ctx context.Context, label string) error {
	cmd := c.execCommand(ctx, "launchctl", "start", label)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start %s: %w (output: %s)", label, err, string(output))
	}
	return nil
}

// Stop stops a service by its label
func (c *LaunchctlClient) Stop(ctx context.Context, label string) error {
	cmd := c.execCommand(ctx, "launchctl", "stop", label)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop %s: %w (output: %s)", label, err, string(output))
	}
	return nil
}

// ServiceStatus represents the status of a launchd service
type ServiceStatus struct {
	Label  string
	PID    string
	Status string
	Loaded bool
}

// List returns all loaded services
func (c *LaunchctlClient) List(ctx context.Context) ([]ServiceStatus, error) {
	cmd := c.execCommand(ctx, "launchctl", "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	return parseListOutput(string(output))
}

// GetStatus returns the status of a specific service.
// It first tries `launchctl print` (modern API). If the service isn't found
// there — which happens for services loaded via the legacy `launchctl load`
// API — it falls back to scanning `launchctl list` output.
func (c *LaunchctlClient) GetStatus(ctx context.Context, label string) (*ServiceStatus, error) {
	cmd := c.execCommand(ctx, "launchctl", "print", fmt.Sprintf("gui/%d/%s", getUserID(), label))
	output, err := cmd.CombinedOutput()
	if err == nil {
		return parsePrintOutput(label, string(output))
	}

	// launchctl print failed — fall back to launchctl list
	services, listErr := c.List(ctx)
	if listErr != nil {
		return nil, fmt.Errorf("service not found: %s", label)
	}

	for _, svc := range services {
		if svc.Label == label {
			return statusFromListEntry(&svc), nil
		}
	}

	return nil, fmt.Errorf("service not found: %s", label)
}

// statusFromListEntry converts a list-format ServiceStatus (where Status is
// a numeric exit code) into a human-readable status.
func statusFromListEntry(svc *ServiceStatus) *ServiceStatus {
	status := "stopped"
	if svc.PID != "-" && svc.PID != "" {
		status = "running"
	} else if svc.Status != "0" && svc.Status != "" {
		status = "error"
	}

	return &ServiceStatus{
		Label:  svc.Label,
		PID:    svc.PID,
		Status: status,
		Loaded: true,
	}
}

// parseListOutput parses the output of 'launchctl list'
// Format:
// PID    Status  Label
// 12345  0       com.example.service
// -      0       com.example.disabled
func parseListOutput(output string) ([]ServiceStatus, error) {
	var services []ServiceStatus
	scanner := bufio.NewScanner(strings.NewReader(output))

	// Skip header line
	if !scanner.Scan() {
		return services, nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		status := ServiceStatus{
			PID:    fields[0],
			Status: fields[1],
			Label:  fields[2],
			Loaded: true,
		}
		services = append(services, status)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error parsing list output: %w", err)
	}

	return services, nil
}

// parsePrintOutput parses the output of 'launchctl print'.
// It uses the first top-level "state = " line and ignores nested ones
// (e.g. inside "resource coalition" blocks) by tracking brace depth.
func parsePrintOutput(label, output string) (*ServiceStatus, error) {
	status := &ServiceStatus{
		Label:  label,
		Loaded: true,
		PID:    "-",
		Status: "unknown",
	}

	stateFound := false
	depth := 0

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		depth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")

		// Only match top-level state (depth <= 1, the outermost block)
		if !stateFound && depth <= 1 {
			if after, ok := strings.CutPrefix(trimmed, "state = "); ok {
				status.Status = after
				stateFound = true
			}
		}

		if after, ok := strings.CutPrefix(trimmed, "pid = "); ok && depth <= 1 {
			status.PID = after
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error parsing print output: %w", err)
	}

	return status, nil
}

// getUserID returns the current user's UID
// This is used for constructing the service domain in launchctl print
func getUserID() int {
	return os.Getuid()
}
