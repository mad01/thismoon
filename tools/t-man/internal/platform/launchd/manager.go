package launchd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

const (
	// UserLaunchAgentsDir is the directory for user-level LaunchAgents
	UserLaunchAgentsDir = "Library/LaunchAgents"

	// SystemLaunchDaemonsDir is the directory for system-level LaunchDaemons
	SystemLaunchDaemonsDir = "/Library/LaunchDaemons"
)

// Manager implements the service.Manager interface for launchd
type Manager struct {
	launchctl *LaunchctlClient
	version   string
	userMode  bool   // true for user services, false for system services
	plistDir  string // overrides the platform plist directory when set (tests only)
}

// NewManager creates a new launchd manager
func NewManager(version string, userMode bool) *Manager {
	return &Manager{
		launchctl: NewLaunchctlClient(),
		version:   version,
		userMode:  userMode,
	}
}

// getPlistDir returns the directory where plist files are stored
func (m *Manager) getPlistDir() (string, error) {
	if m.plistDir != "" {
		return m.plistDir, nil
	}
	if m.userMode {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		return filepath.Join(homeDir, UserLaunchAgentsDir), nil
	}
	return SystemLaunchDaemonsDir, nil
}

// getPlistPath returns the full path to a service's plist file
func (m *Manager) getPlistPath(name string) (string, error) {
	dir, err := m.getPlistDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%s.plist", name)), nil
}

// createService creates a new service
func (m *Manager) createService(ctx context.Context, def *service.Definition) error {
	// Generate plist content
	plistContent, err := GeneratePlist(def, m.version)
	if err != nil {
		return fmt.Errorf("failed to generate plist: %w", err)
	}

	// Write plist file
	plistPath, err := m.getPlistPath(def.Name)
	if err != nil {
		return err
	}

	// Use O_CREATE|O_EXCL to atomically create and fail if file already exists.
	// This also prevents writing through a symlink since the symlink target would
	// need to not exist for O_EXCL to succeed.
	f, err := os.OpenFile(plistPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("failed to create plist file: %w", err)
	}
	if _, err := f.Write(plistContent); err != nil {
		_ = f.Close()
		_ = os.Remove(plistPath)
		return fmt.Errorf("failed to write plist file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(plistPath)
		return fmt.Errorf("failed to close plist file: %w", err)
	}

	// Load the service
	if err := m.launchctl.Load(ctx, plistPath); err != nil {
		// Clean up plist file if load fails
		_ = os.Remove(plistPath)
		return fmt.Errorf("failed to load service: %w", err)
	}

	return nil
}

// updateService updates an existing service safely.
// It writes the new plist to a temp file first, then unloads the old service,
// replaces the plist, and loads the new one. If loading the new plist fails,
// it restores the old plist and reloads it.
func (m *Manager) updateService(ctx context.Context, def *service.Definition) error {
	plistPath, err := m.getPlistPath(def.Name)
	if err != nil {
		return err
	}

	// Reject symlinks to prevent writing to unexpected locations
	fi, err := os.Lstat(plistPath)
	if err != nil {
		return fmt.Errorf("failed to stat plist path: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to update: plist path is a symlink: %s", plistPath)
	}

	// Save old plist content for rollback
	oldContent, err := os.ReadFile(plistPath)
	if err != nil {
		return fmt.Errorf("failed to read existing plist for backup: %w", err)
	}

	// Generate new plist content
	plistContent, err := GeneratePlist(def, m.version)
	if err != nil {
		return fmt.Errorf("failed to generate plist: %w", err)
	}

	// Write new plist to temp file first to validate it can be written
	dir := filepath.Dir(plistPath)
	tmpFile, err := os.CreateTemp(dir, "t-man-*.plist.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }() // clean up temp file in all paths

	if _, err := tmpFile.Write(plistContent); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write temp plist: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp plist: %w", err)
	}

	// Unload the existing service
	_ = m.launchctl.Unload(ctx, plistPath) // continue even if unload fails

	// Replace the plist file
	if err := os.Rename(tmpPath, plistPath); err != nil {
		// Rename failed; try to reload old service
		_ = m.launchctl.Load(ctx, plistPath)
		return fmt.Errorf("failed to replace plist file: %w", err)
	}

	// Load the updated service
	if err := m.launchctl.Load(ctx, plistPath); err != nil {
		// Loading new plist failed — restore old plist and reload
		if writeErr := os.WriteFile(plistPath, oldContent, 0o644); writeErr != nil {
			return fmt.Errorf(
				"failed to load new service (%w) AND failed to restore old plist: %v",
				err,
				writeErr,
			)
		}
		_ = m.launchctl.Load(ctx, plistPath)
		return fmt.Errorf(
			"failed to load updated service (rolled back to previous version): %w",
			err,
		)
	}

	return nil
}

// Create creates a single service without affecting other managed services
func (m *Manager) Create(ctx context.Context, def *service.Definition) error {
	return m.createService(ctx, def)
}

// Update updates a single service in place without affecting other managed services
func (m *Manager) Update(ctx context.Context, def *service.Definition) error {
	return m.updateService(ctx, def)
}

// List returns all currently managed services
func (m *Manager) List(ctx context.Context) ([]*service.Definition, error) {
	dir, err := m.getPlistDir()
	if err != nil {
		return nil, err
	}

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []*service.Definition{}, nil
	}

	// Read all plist files in the directory
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	definitions := []*service.Definition{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".plist" {
			continue
		}

		plistPath := filepath.Join(dir, entry.Name())

		// Read plist file
		data, err := os.ReadFile(plistPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping unreadable plist %s: %v\n", plistPath, err)
			continue
		}

		// Check if it's managed by t-man or serviceman (for auto-adoption)
		isTManManaged, err := IsManagedByTMan(data)
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"warning: skipping plist with parse error %s: %v\n",
				plistPath,
				err,
			)
			continue
		}
		isServicemanManaged := IsServicemanPlist(data)

		if !isTManManaged && !isServicemanManaged {
			continue // Skip plists not managed by t-man or serviceman
		}

		// Parse the plist
		plistData, err := ParsePlist(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping invalid plist %s: %v\n", plistPath, err)
			continue
		}

		// Convert to service.Definition
		def := m.plistToDefinition(plistData)
		definitions = append(definitions, def)
	}

	return definitions, nil
}

// Get retrieves a specific service by name
func (m *Manager) Get(ctx context.Context, name string) (*service.Definition, error) {
	plistPath, err := m.getPlistPath(name)
	if err != nil {
		return nil, err
	}

	// Check if plist file exists
	data, err := os.ReadFile(plistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, service.ServiceNotFoundError(name)
		}
		return nil, fmt.Errorf("failed to read plist: %w", err)
	}

	// Check if it's managed by t-man or serviceman (for auto-adoption)
	isTManManaged, err := IsManagedByTMan(data)
	if err != nil {
		return nil, fmt.Errorf("failed to check if managed by t-man: %w", err)
	}
	isServicemanManaged := IsServicemanPlist(data)

	if !isTManManaged && !isServicemanManaged {
		return nil, fmt.Errorf("service %s is not managed by t-man or serviceman", name)
	}

	// Parse the plist
	plistData, err := ParsePlist(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plist: %w", err)
	}

	return m.plistToDefinition(plistData), nil
}

// Delete removes a service
func (m *Manager) Delete(ctx context.Context, name string) error {
	plistPath, err := m.getPlistPath(name)
	if err != nil {
		return err
	}

	// Unload the service first (best-effort; service might not be loaded)
	_ = m.launchctl.Unload(ctx, plistPath)

	// Remove the plist file directly, avoiding TOCTOU with stat-then-remove
	if err := os.Remove(plistPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return service.ServiceNotFoundError(name)
		}
		return fmt.Errorf("failed to remove plist file: %w", err)
	}

	return nil
}

// Start starts a service
func (m *Manager) Start(ctx context.Context, name string) error {
	// Verify service exists and is managed by t-man
	if _, err := m.Get(ctx, name); err != nil {
		return err
	}

	return m.launchctl.Start(ctx, name)
}

// Stop stops a service
func (m *Manager) Stop(ctx context.Context, name string) error {
	// Verify service exists and is managed by t-man
	if _, err := m.Get(ctx, name); err != nil {
		return err
	}

	return m.launchctl.Stop(ctx, name)
}

// Status returns the status of a service
func (m *Manager) Status(ctx context.Context, name string) (string, error) {
	// Verify service exists and is managed by t-man
	if _, err := m.Get(ctx, name); err != nil {
		return "", err
	}

	status, err := m.launchctl.GetStatus(ctx, name)
	if err != nil {
		return "", err
	}

	return status.Status, nil
}

// RunningPIDs returns the PID of every running launchd job visible to a
// single `launchctl list` call, keyed by label. Labels that are loaded but
// have no running process are omitted. The map covers all jobs in the
// domain, not just t-man-managed ones — callers look up the labels they
// care about.
func (m *Manager) RunningPIDs(ctx context.Context) (map[string]int, error) {
	statuses, err := m.launchctl.List(ctx)
	if err != nil {
		return nil, err
	}

	pids := make(map[string]int, len(statuses))
	for _, svc := range statuses {
		pid, err := strconv.Atoi(svc.PID)
		if err != nil || pid <= 0 {
			continue
		}
		pids[svc.Label] = pid
	}
	return pids, nil
}

// plistToDefinition converts a LaunchdPlist to a service.Definition
func (m *Manager) plistToDefinition(plist *LaunchdPlist) *service.Definition {
	// Extract command and args from ProgramArguments. For sandboxed services
	// the arguments are wrapped as [sandbox-exec -D HOME=... -f <profile>
	// command args...]; strip that prefix symmetrically to GeneratePlist so
	// reconcile compares the real command — otherwise every reconcile would
	// see a phantom diff.
	programArgs := plist.ProgramArguments
	if plist.TManMetadata.SandboxProfile != "" {
		programArgs = unwrapSandboxArgs(programArgs)
	}

	var command string
	var args []string

	if len(programArgs) > 0 {
		command = programArgs[0]
		if len(programArgs) > 1 {
			args = programArgs[1:]
		}
	}

	return &service.Definition{
		Name:                 plist.Label,
		Command:              command,
		Args:                 args,
		WorkingDir:           plist.WorkingDirectory,
		Environment:          plist.EnvironmentVariables,
		RunAtLoad:            plist.RunAtLoad,
		KeepAlive:            plist.KeepAlive,
		StandardOutPath:      plist.StandardOutPath,
		StandardErrPath:      plist.StandardErrorPath,
		SandboxProfile:       plist.TManMetadata.SandboxProfile,
		SandboxProfileSHA256: plist.TManMetadata.SandboxProfileSHA256,
		ExtraLogs:            plist.TManMetadata.ExtraLogs,
	}
}

// unwrapSandboxArgs strips the sandbox-exec wrapper prefix produced by
// GeneratePlist: [sandbox-exec, -D, KEY=VALUE..., -f, <profile>, command,
// args...] -> [command, args...]. Returns the input unchanged if it doesn't
// match the expected shape.
func unwrapSandboxArgs(programArgs []string) []string {
	if len(programArgs) == 0 || programArgs[0] != SandboxExecPath {
		return programArgs
	}
	for i := 1; i < len(programArgs)-1; i++ {
		if programArgs[i] == "-f" {
			// element after -f is the profile path; the rest is command+args
			if i+2 <= len(programArgs)-1 {
				return programArgs[i+2:]
			}
			return nil
		}
	}
	return programArgs
}
