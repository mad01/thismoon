package launchd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

// TestIntegration_PlistGenerationAndParsing tests the full cycle of plist generation and parsing
func TestIntegration_PlistGenerationAndParsing(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	tests := []struct {
		name string
		def  *service.Definition
	}{
		{
			name: "minimal service",
			def: &service.Definition{
				Name:      "test.minimal",
				Command:   execPath,
				RunAtLoad: true,
			},
		},
		{
			name: "service with arguments",
			def: &service.Definition{
				Name:      "test.withargs",
				Command:   execPath,
				Args:      []string{"arg1", "arg2", "arg3"},
				RunAtLoad: true,
				KeepAlive: true,
			},
		},
		{
			name: "service with environment",
			def: &service.Definition{
				Name:    "test.withenv",
				Command: execPath,
				Environment: map[string]string{
					"FOO": "bar",
					"BAZ": "qux",
				},
				RunAtLoad: true,
			},
		},
		{
			name: "full service configuration",
			def: &service.Definition{
				Name:            "test.full",
				Command:         execPath,
				Args:            []string{"--port", "8080"},
				WorkingDir:      tmpDir,
				Environment:     map[string]string{"ENV": "test"},
				RunAtLoad:       true,
				KeepAlive:       true,
				StandardOutPath: filepath.Join(tmpDir, "out.log"),
				StandardErrPath: filepath.Join(tmpDir, "err.log"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate plist
			plistData, err := GeneratePlist(tt.def, "0.1.0")
			if err != nil {
				t.Fatalf("GeneratePlist() error = %v", err)
			}

			// Verify marker comment is present
			if !HasMarkerComment(plistData) {
				t.Error("Generated plist missing marker comment")
			}

			// Parse the generated plist
			parsed, err := ParsePlist(plistData)
			if err != nil {
				t.Fatalf("ParsePlist() error = %v", err)
			}

			// Verify Label
			if parsed.Label != tt.def.Name {
				t.Errorf("Label = %v, want %v", parsed.Label, tt.def.Name)
			}

			// Verify ProgramArguments
			expectedArgs := append([]string{tt.def.Command}, tt.def.Args...)
			if len(parsed.ProgramArguments) != len(expectedArgs) {
				t.Errorf(
					"ProgramArguments length = %v, want %v",
					len(parsed.ProgramArguments),
					len(expectedArgs),
				)
			}

			// Verify t-man metadata
			if parsed.TManMetadata.ManagedBy != ManagedByValue {
				t.Errorf("ManagedBy = %v, want %v", parsed.TManMetadata.ManagedBy, ManagedByValue)
			}

			if parsed.TManMetadata.Version != "0.1.0" {
				t.Errorf("Version = %v, want 0.1.0", parsed.TManMetadata.Version)
			}

			if parsed.TManMetadata.Hash == "" {
				t.Error("Hash is empty")
			}

			// Verify it's detected as managed by t-man
			isManagedByTMan, err := IsManagedByTMan(plistData)
			if err != nil {
				t.Fatalf("IsManagedByTMan() error = %v", err)
			}
			if !isManagedByTMan {
				t.Error("IsManagedByTMan() = false, want true")
			}
		})
	}
}

// TestIntegration_RoundTrip tests Definition -> Plist -> Definition conversion
func TestIntegration_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	original := &service.Definition{
		Name:    "test.roundtrip",
		Command: execPath,
		Args:    []string{"arg1", "arg2"},
		Environment: map[string]string{
			"KEY1": "value1",
			"KEY2": "value2",
		},
		WorkingDir:      tmpDir,
		RunAtLoad:       true,
		KeepAlive:       true,
		StandardOutPath: filepath.Join(tmpDir, "out.log"),
		StandardErrPath: filepath.Join(tmpDir, "err.log"),
	}

	// Step 1: Generate plist from definition
	plistData, err := GeneratePlist(original, "0.1.0")
	if err != nil {
		t.Fatalf("GeneratePlist() error = %v", err)
	}

	// Step 2: Parse plist back
	parsed, err := ParsePlist(plistData)
	if err != nil {
		t.Fatalf("ParsePlist() error = %v", err)
	}

	// Step 3: Reconstruct definition from parsed plist
	reconstructed := &service.Definition{
		Name:            parsed.Label,
		Command:         parsed.ProgramArguments[0],
		Args:            parsed.ProgramArguments[1:],
		WorkingDir:      parsed.WorkingDirectory,
		Environment:     parsed.EnvironmentVariables,
		RunAtLoad:       parsed.RunAtLoad,
		KeepAlive:       parsed.KeepAlive,
		StandardOutPath: parsed.StandardOutPath,
		StandardErrPath: parsed.StandardErrorPath,
	}

	// Step 4: Compare original and reconstructed
	if reconstructed.Name != original.Name {
		t.Errorf("Name = %v, want %v", reconstructed.Name, original.Name)
	}

	if reconstructed.Command != original.Command {
		t.Errorf("Command = %v, want %v", reconstructed.Command, original.Command)
	}

	if len(reconstructed.Args) != len(original.Args) {
		t.Errorf("Args length = %v, want %v", len(reconstructed.Args), len(original.Args))
	}

	if reconstructed.RunAtLoad != original.RunAtLoad {
		t.Errorf("RunAtLoad = %v, want %v", reconstructed.RunAtLoad, original.RunAtLoad)
	}

	if reconstructed.KeepAlive != original.KeepAlive {
		t.Errorf("KeepAlive = %v, want %v", reconstructed.KeepAlive, original.KeepAlive)
	}

	// Step 5: Verify hashes match
	originalHash, err := original.Hash()
	if err != nil {
		t.Fatalf("original.Hash() error = %v", err)
	}

	reconstructedHash, err := reconstructed.Hash()
	if err != nil {
		t.Fatalf("reconstructed.Hash() error = %v", err)
	}

	if originalHash != reconstructedHash {
		t.Errorf(
			"Hash mismatch: original = %v, reconstructed = %v",
			originalHash,
			reconstructedHash,
		)
	}
}

// TestIntegration_FileOperations tests actual file writing and reading
func TestIntegration_FileOperations(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	plistDir := filepath.Join(tmpDir, "plists")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		t.Fatalf("Failed to create plist directory: %v", err)
	}

	def := &service.Definition{
		Name:      "test.fileops",
		Command:   execPath,
		Args:      []string{"--test"},
		RunAtLoad: true,
	}

	// Generate plist
	plistData, err := GeneratePlist(def, "0.1.0")
	if err != nil {
		t.Fatalf("GeneratePlist() error = %v", err)
	}

	// Write to file
	plistPath := filepath.Join(plistDir, "test.fileops.plist")
	if err := os.WriteFile(plistPath, plistData, 0o644); err != nil {
		t.Fatalf("Failed to write plist file: %v", err)
	}

	// Read back from file
	readData, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("Failed to read plist file: %v", err)
	}

	// Verify content matches
	if string(readData) != string(plistData) {
		t.Error("File content doesn't match original plist data")
	}

	// Parse the read data
	parsed, err := ParsePlist(readData)
	if err != nil {
		t.Fatalf("ParsePlist() error = %v", err)
	}

	if parsed.Label != def.Name {
		t.Errorf("Label = %v, want %v", parsed.Label, def.Name)
	}

	// Verify file can be detected as t-man managed
	isManagedByTMan, err := IsManagedByTMan(readData)
	if err != nil {
		t.Fatalf("IsManagedByTMan() error = %v", err)
	}
	if !isManagedByTMan {
		t.Error("File should be detected as t-man managed")
	}
}

// TestIntegration_ServicemanDetection tests detection of serviceman-generated plists
func TestIntegration_ServicemanDetection(t *testing.T) {
	// Read serviceman test fixtures
	testdataPath := filepath.Join("..", "..", "..", "testdata", "plists", "serviceman")

	files, err := os.ReadDir(testdataPath)
	if err != nil {
		t.Skipf("Skipping serviceman detection test: %v", err)
		return
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".plist" {
			continue
		}

		t.Run(file.Name(), func(t *testing.T) {
			filePath := filepath.Join(testdataPath, file.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}

			// Verify it contains serviceman marker
			if !IsServicemanPlist(data) {
				t.Error("File should be detected as serviceman-generated")
			}

			// Verify it's NOT detected as t-man managed
			isManagedByTMan, err := IsManagedByTMan(data)
			if err != nil {
				// Serviceman plists won't have TManMetadata, so parsing might fail
				// This is expected
				return
			}
			if isManagedByTMan {
				t.Error("Serviceman plist should NOT be detected as t-man managed")
			}
		})
	}
}

// TestIntegration_TManPlists tests loading and validation of t-man test fixtures
func TestIntegration_TManPlists(t *testing.T) {
	testdataPath := filepath.Join("..", "..", "..", "testdata", "plists", "t-man")

	files, err := os.ReadDir(testdataPath)
	if err != nil {
		t.Skipf("Skipping t-man plist test: %v", err)
		return
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".plist" {
			continue
		}

		t.Run(file.Name(), func(t *testing.T) {
			filePath := filepath.Join(testdataPath, file.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}

			// Verify it has marker comment
			if !HasMarkerComment(data) {
				t.Error("t-man plist should have marker comment")
			}

			// Verify it's detected as t-man managed
			isManagedByTMan, err := IsManagedByTMan(data)
			if err != nil {
				t.Fatalf("IsManagedByTMan() error = %v", err)
			}
			if !isManagedByTMan {
				t.Error("File should be detected as t-man managed")
			}

			// Parse and validate structure
			parsed, err := ParsePlist(data)
			if err != nil {
				t.Fatalf("ParsePlist() error = %v", err)
			}

			// Verify metadata is present
			if parsed.TManMetadata.ManagedBy != ManagedByValue {
				t.Errorf("ManagedBy = %v, want %v", parsed.TManMetadata.ManagedBy, ManagedByValue)
			}

			if parsed.TManMetadata.Hash == "" {
				t.Error("Hash is empty")
			}

			if parsed.TManMetadata.Version == "" {
				t.Error("Version is empty")
			}

			// Verify required fields
			if parsed.Label == "" {
				t.Error("Label is empty")
			}

			if len(parsed.ProgramArguments) == 0 {
				t.Error("ProgramArguments is empty")
			}
		})
	}
}

// TestIntegration_ManagerReconciliation tests reconciliation with temp directories
// Note: This test is skipped because the current Manager implementation uses fixed
// system directories and doesn't support custom plist directories for testing.
// The reconciliation logic is tested in the reconcile package tests instead.
func TestIntegration_ManagerReconciliation(t *testing.T) {
	t.Skip("Manager doesn't support custom plist directories for testing")
}
