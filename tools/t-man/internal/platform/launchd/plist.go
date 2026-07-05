package launchd

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
	"howett.net/plist"
)

const (
	// ManagedByValue is the identifier for t-man managed services
	ManagedByValue = "t-man"

	// MarkerComment is the XML comment added to identify t-man managed plists
	MarkerComment = "<!-- Managed by t-man - DO NOT EDIT MANUALLY -->"

	// SandboxExecPath is the macOS seatbelt wrapper binary
	SandboxExecPath = "/usr/bin/sandbox-exec"
)

// TManMetadata contains metadata about the managed service
type TManMetadata struct {
	Hash      string `plist:"Hash"`
	ManagedBy string `plist:"ManagedBy"`
	Version   string `plist:"Version"`
	// SandboxProfile + SandboxProfileSHA256 record the seatbelt profile so
	// plistToDefinition can unwrap ProgramArguments symmetrically and
	// reconcile detects profile content edits.
	SandboxProfile       string `plist:"SandboxProfile,omitempty"`
	SandboxProfileSHA256 string `plist:"SandboxProfileSHA256,omitempty"`
	// ExtraLogs records additional named log files so plistToDefinition can
	// rebuild Definition.ExtraLogs; without the round-trip every reconcile of
	// a service with extra logs would see a phantom hash diff.
	ExtraLogs map[string]string `plist:"ExtraLogs,omitempty"`
}

// LaunchdPlist represents a launchd property list with t-man metadata
type LaunchdPlist struct {
	Label                string            `plist:"Label"`
	ProgramArguments     []string          `plist:"ProgramArguments"`
	WorkingDirectory     string            `plist:"WorkingDirectory,omitempty"`
	EnvironmentVariables map[string]string `plist:"EnvironmentVariables,omitempty"`
	RunAtLoad            bool              `plist:"RunAtLoad"`
	KeepAlive            bool              `plist:"KeepAlive"`
	StandardOutPath      string            `plist:"StandardOutPath,omitempty"`
	StandardErrorPath    string            `plist:"StandardErrorPath,omitempty"`
	TManMetadata         TManMetadata      `plist:"TManMetadata"`
}

// GeneratePlist creates a launchd plist from a service definition
func GeneratePlist(def *service.Definition, version string) ([]byte, error) {
	if def == nil {
		return nil, fmt.Errorf("service definition cannot be nil")
	}

	// Validate the definition
	if err := def.Validate(); err != nil {
		return nil, fmt.Errorf("invalid service definition: %w", err)
	}

	// Ensure the sandbox profile digest is recorded before hashing, so the
	// stored hash reflects profile content even if the caller skipped it
	if def.SandboxProfile != "" && def.SandboxProfileSHA256 == "" {
		if err := def.PopulateSandboxDigest(); err != nil {
			return nil, fmt.Errorf("failed to digest sandbox profile: %w", err)
		}
	}

	// Calculate hash for the definition
	hash, err := def.Hash()
	if err != nil {
		return nil, fmt.Errorf("failed to calculate definition hash: %w", err)
	}

	// Build program arguments (command + args), wrapped in sandbox-exec when
	// a seatbelt profile is set. t-man supplies -D HOME= so profiles can use
	// (param "HOME") without every caller remembering it. sandbox-exec
	// exec()s the target in place (same PID), so KeepAlive semantics are
	// unchanged.
	var programArgs []string
	if def.SandboxProfile != "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve home directory for sandbox params: %w", err)
		}
		programArgs = []string{SandboxExecPath, "-D", "HOME=" + homeDir, "-f", def.SandboxProfile}
	}
	programArgs = append(programArgs, def.Command)
	programArgs = append(programArgs, def.Args...)

	// Create the plist structure
	plistData := &LaunchdPlist{
		Label:                def.Name,
		ProgramArguments:     programArgs,
		WorkingDirectory:     def.WorkingDir,
		EnvironmentVariables: def.Environment,
		RunAtLoad:            def.RunAtLoad,
		KeepAlive:            def.KeepAlive,
		StandardOutPath:      def.StandardOutPath,
		StandardErrorPath:    def.StandardErrPath,
		TManMetadata: TManMetadata{
			Hash:                 hash,
			ManagedBy:            ManagedByValue,
			Version:              version,
			SandboxProfile:       def.SandboxProfile,
			SandboxProfileSHA256: def.SandboxProfileSHA256,
			ExtraLogs:            def.ExtraLogs,
		},
	}

	// Marshal to XML plist format
	var buf bytes.Buffer
	encoder := plist.NewEncoderForFormat(&buf, plist.XMLFormat)
	if err := encoder.Encode(plistData); err != nil {
		return nil, fmt.Errorf("failed to encode plist: %w", err)
	}

	// Add marker comment after XML declaration
	xmlContent := buf.String()
	xmlWithMarker := addMarkerComment(xmlContent)

	return []byte(xmlWithMarker), nil
}

// addMarkerComment inserts the marker comment after the XML declaration
func addMarkerComment(xmlContent string) string {
	lines := strings.Split(xmlContent, "\n")
	if len(lines) == 0 {
		return xmlContent
	}

	// Find the XML declaration line
	var result []string
	for i, line := range lines {
		result = append(result, line)
		// Add marker after XML declaration (<?xml ... ?>)
		if i == 0 && strings.HasPrefix(strings.TrimSpace(line), "<?xml") {
			result = append(result, MarkerComment)
		}
	}

	return strings.Join(result, "\n")
}

// ParsePlist parses a launchd plist and extracts t-man metadata
func ParsePlist(data []byte) (*LaunchdPlist, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("plist data cannot be empty")
	}

	var plistData LaunchdPlist
	decoder := plist.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&plistData); err != nil {
		return nil, fmt.Errorf("failed to decode plist: %w", err)
	}

	return &plistData, nil
}

// IsManagedByTMan checks if a plist is managed by t-man
func IsManagedByTMan(data []byte) (bool, error) {
	plistData, err := ParsePlist(data)
	if err != nil {
		return false, err
	}

	return plistData.TManMetadata.ManagedBy == ManagedByValue, nil
}

// HasMarkerComment checks if the plist content contains the t-man marker comment
func HasMarkerComment(data []byte) bool {
	return bytes.Contains(data, []byte(MarkerComment))
}

// IsServicemanPlist checks if a plist was generated by serviceman
func IsServicemanPlist(data []byte) bool {
	return bytes.Contains(data, []byte("Generated for serviceman"))
}
