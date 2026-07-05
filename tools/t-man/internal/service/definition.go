package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// Definition represents a service definition with its configuration
type Definition struct {
	Name            string            `json:"name"`
	Command         string            `json:"command"`
	Args            []string          `json:"args,omitempty"`
	WorkingDir      string            `json:"working_dir,omitempty"`
	Environment     map[string]string `json:"environment,omitempty"`
	RunAtLoad       bool              `json:"run_at_load"`
	KeepAlive       bool              `json:"keep_alive"`
	StandardOutPath string            `json:"standard_out_path,omitempty"`
	StandardErrPath string            `json:"standard_err_path,omitempty"`
	// SandboxProfile is the absolute path to a seatbelt profile (.sb); when
	// set, the service runs wrapped in sandbox-exec -f <profile>.
	SandboxProfile string `json:"sandbox_profile,omitempty"`
	// SandboxProfileSHA256 is the content digest of the profile file at the
	// time the definition was built. It feeds Hash() so profile edits
	// reconcile; it must be a stored field (round-tripped via the plist), not
	// re-read at compare time — otherwise current and desired would always
	// agree on content and edits would never produce a diff.
	SandboxProfileSHA256 string `json:"sandbox_profile_sha256,omitempty"`
	// ExtraLogs maps a source name (e.g. "sandbox") to an additional log file
	// written for this service outside stdout/stderr — like a sandbox
	// notification log produced by an external watcher. Viewed via
	// `t-man logs <name> --source <source>`. Round-tripped through the plist
	// (TManMetadata) so reconcile compares it like any other field.
	ExtraLogs map[string]string `json:"extra_logs,omitempty"`
}

// PopulateSandboxDigest reads the sandbox profile file and records its
// content digest. Call after setting SandboxProfile on a freshly built
// definition; no-op when no profile is set.
func (d *Definition) PopulateSandboxDigest() error {
	if d.SandboxProfile == "" {
		return nil
	}
	content, err := os.ReadFile(d.SandboxProfile)
	if err != nil {
		return fmt.Errorf("failed to read sandbox profile: %w", err)
	}
	sum := sha256.Sum256(content)
	d.SandboxProfileSHA256 = hex.EncodeToString(sum[:])
	return nil
}

// Hash returns the SHA256 hash of the service definition content
func (d *Definition) Hash() (string, error) {
	// Create a normalized JSON representation for consistent hashing
	data, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("failed to marshal definition: %w", err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// validNamePattern matches safe service names: alphanumeric, dots, hyphens, underscores
var validNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// Validate checks if the service definition is valid
func (d *Definition) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("service name is required")
	}

	if !validNamePattern.MatchString(d.Name) {
		return fmt.Errorf(
			"service name contains invalid characters (allowed: a-z, A-Z, 0-9, '.', '-', '_'): %s",
			d.Name,
		)
	}

	if d.Command == "" {
		return fmt.Errorf("command is required")
	}

	// Check if command exists and is executable
	if !filepath.IsAbs(d.Command) {
		return fmt.Errorf("command must be an absolute path: %s", d.Command)
	}

	info, err := os.Stat(d.Command)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("command does not exist: %s", d.Command)
		}
		return fmt.Errorf("failed to stat command: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("command is a directory: %s", d.Command)
	}

	// Check if file is executable (Unix permission check)
	mode := info.Mode()
	if mode&0o111 == 0 {
		return fmt.Errorf("command is not executable: %s", d.Command)
	}

	// Validate sandbox profile if specified
	if d.SandboxProfile != "" {
		if !filepath.IsAbs(d.SandboxProfile) {
			return fmt.Errorf("sandbox profile must be an absolute path: %s", d.SandboxProfile)
		}

		info, err := os.Stat(d.SandboxProfile)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("sandbox profile does not exist: %s", d.SandboxProfile)
			}
			return fmt.Errorf("failed to stat sandbox profile: %w", err)
		}

		if info.IsDir() {
			return fmt.Errorf("sandbox profile is a directory: %s", d.SandboxProfile)
		}
	}

	// Validate extra log sources if specified
	for source, path := range d.ExtraLogs {
		if !validNamePattern.MatchString(source) {
			return fmt.Errorf(
				"extra log source name contains invalid characters (allowed: a-z, A-Z, 0-9, '.', '-', '_'): %s",
				source,
			)
		}
		if source == "stdout" || source == "stderr" {
			return fmt.Errorf("extra log source name is reserved: %s", source)
		}
		if !filepath.IsAbs(path) {
			return fmt.Errorf("extra log path must be an absolute path: %s=%s", source, path)
		}
	}

	// Validate working directory if specified
	if d.WorkingDir != "" {
		if !filepath.IsAbs(d.WorkingDir) {
			return fmt.Errorf("working directory must be an absolute path: %s", d.WorkingDir)
		}

		info, err := os.Stat(d.WorkingDir)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("working directory does not exist: %s", d.WorkingDir)
			}
			return fmt.Errorf("failed to stat working directory: %w", err)
		}

		if !info.IsDir() {
			return fmt.Errorf("working directory is not a directory: %s", d.WorkingDir)
		}
	}

	return nil
}
