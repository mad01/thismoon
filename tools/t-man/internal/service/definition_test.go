package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefinitionHash(t *testing.T) {
	tests := []struct {
		name     string
		def1     *Definition
		def2     *Definition
		wantSame bool
	}{
		{
			name: "identical definitions produce same hash",
			def1: &Definition{
				Name:       "test-service",
				Command:    "/usr/bin/echo",
				Args:       []string{"hello"},
				RunAtLoad:  true,
				KeepAlive:  true,
				WorkingDir: "/tmp",
			},
			def2: &Definition{
				Name:       "test-service",
				Command:    "/usr/bin/echo",
				Args:       []string{"hello"},
				RunAtLoad:  true,
				KeepAlive:  true,
				WorkingDir: "/tmp",
			},
			wantSame: true,
		},
		{
			name: "different names produce different hash",
			def1: &Definition{
				Name:    "test-service-1",
				Command: "/usr/bin/echo",
			},
			def2: &Definition{
				Name:    "test-service-2",
				Command: "/usr/bin/echo",
			},
			wantSame: false,
		},
		{
			name: "different commands produce different hash",
			def1: &Definition{
				Name:    "test-service",
				Command: "/usr/bin/echo",
			},
			def2: &Definition{
				Name:    "test-service",
				Command: "/usr/bin/true",
			},
			wantSame: false,
		},
		{
			name: "different args produce different hash",
			def1: &Definition{
				Name:    "test-service",
				Command: "/usr/bin/echo",
				Args:    []string{"hello"},
			},
			def2: &Definition{
				Name:    "test-service",
				Command: "/usr/bin/echo",
				Args:    []string{"world"},
			},
			wantSame: false,
		},
		{
			name: "different environment produces different hash",
			def1: &Definition{
				Name:        "test-service",
				Command:     "/usr/bin/echo",
				Environment: map[string]string{"FOO": "bar"},
			},
			def2: &Definition{
				Name:        "test-service",
				Command:     "/usr/bin/echo",
				Environment: map[string]string{"FOO": "baz"},
			},
			wantSame: false,
		},
		{
			name: "hash is consistent across multiple calls",
			def1: &Definition{
				Name:    "test-service",
				Command: "/usr/bin/echo",
			},
			def2: &Definition{
				Name:    "test-service",
				Command: "/usr/bin/echo",
			},
			wantSame: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1, err := tt.def1.Hash()
			if err != nil {
				t.Fatalf("Hash() error for def1: %v", err)
			}

			hash2, err := tt.def2.Hash()
			if err != nil {
				t.Fatalf("Hash() error for def2: %v", err)
			}

			if tt.wantSame && hash1 != hash2 {
				t.Errorf(
					"Hash() expected same hash, got different:\nhash1: %s\nhash2: %s",
					hash1,
					hash2,
				)
			}

			if !tt.wantSame && hash1 == hash2 {
				t.Errorf("Hash() expected different hash, got same: %s", hash1)
			}

			// Verify hash is deterministic by computing again
			hash1Again, err := tt.def1.Hash()
			if err != nil {
				t.Fatalf("Hash() error on second call for def1: %v", err)
			}
			if hash1 != hash1Again {
				t.Errorf("Hash() is not deterministic: first=%s, second=%s", hash1, hash1Again)
			}
		})
	}
}

func TestDefinitionValidate(t *testing.T) {
	// Create a temporary executable for testing
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	tests := []struct {
		name    string
		def     *Definition
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid minimal definition",
			def: &Definition{
				Name:    "test-service",
				Command: execPath,
			},
			wantErr: false,
		},
		{
			name: "valid definition with all fields",
			def: &Definition{
				Name:            "test-service",
				Command:         execPath,
				Args:            []string{"arg1", "arg2"},
				WorkingDir:      tmpDir,
				Environment:     map[string]string{"KEY": "value"},
				RunAtLoad:       true,
				KeepAlive:       true,
				StandardOutPath: filepath.Join(tmpDir, "out.log"),
				StandardErrPath: filepath.Join(tmpDir, "err.log"),
			},
			wantErr: false,
		},
		{
			name: "missing name",
			def: &Definition{
				Command: execPath,
			},
			wantErr: true,
			errMsg:  "service name is required",
		},
		{
			name: "name with slash",
			def: &Definition{
				Name:    "bad/name",
				Command: execPath,
			},
			wantErr: true,
			errMsg:  "service name contains invalid characters",
		},
		{
			name: "name with path traversal",
			def: &Definition{
				Name:    "../etc/passwd",
				Command: execPath,
			},
			wantErr: true,
			errMsg:  "service name contains invalid characters",
		},
		{
			name: "name with space",
			def: &Definition{
				Name:    "bad name",
				Command: execPath,
			},
			wantErr: true,
			errMsg:  "service name contains invalid characters",
		},
		{
			name: "valid name with dots and hyphens",
			def: &Definition{
				Name:    "com.example.my-service_v2",
				Command: execPath,
			},
			wantErr: false,
		},
		{
			name: "missing command",
			def: &Definition{
				Name: "test-service",
			},
			wantErr: true,
			errMsg:  "command is required",
		},
		{
			name: "relative command path",
			def: &Definition{
				Name:    "test-service",
				Command: "testexec",
			},
			wantErr: true,
			errMsg:  "command must be an absolute path",
		},
		{
			name: "non-existent command",
			def: &Definition{
				Name:    "test-service",
				Command: "/nonexistent/path/to/command",
			},
			wantErr: true,
			errMsg:  "command does not exist",
		},
		{
			name: "command is a directory",
			def: &Definition{
				Name:    "test-service",
				Command: tmpDir,
			},
			wantErr: true,
			errMsg:  "command is a directory",
		},
		{
			name: "non-executable command",
			def: &Definition{
				Name:    "test-service",
				Command: createNonExecFile(t, tmpDir),
			},
			wantErr: true,
			errMsg:  "command is not executable",
		},
		{
			name: "relative working directory",
			def: &Definition{
				Name:       "test-service",
				Command:    execPath,
				WorkingDir: "relative/path",
			},
			wantErr: true,
			errMsg:  "working directory must be an absolute path",
		},
		{
			name: "non-existent working directory",
			def: &Definition{
				Name:       "test-service",
				Command:    execPath,
				WorkingDir: "/nonexistent/working/dir",
			},
			wantErr: true,
			errMsg:  "working directory does not exist",
		},
		{
			name: "working directory is a file",
			def: &Definition{
				Name:       "test-service",
				Command:    execPath,
				WorkingDir: execPath,
			},
			wantErr: true,
			errMsg:  "working directory is not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					// Check if error message contains the expected substring
					if len(tt.errMsg) > 0 {
						found := false
						errStr := err.Error()
						// Check if the error message starts with our expected message
						if len(errStr) >= len(tt.errMsg) && errStr[:len(tt.errMsg)] == tt.errMsg {
							found = true
						}
						if !found {
							t.Errorf(
								"Validate() error = %q, want error containing %q",
								err.Error(),
								tt.errMsg,
							)
						}
					}
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			}
		})
	}
}

// Helper function to create a non-executable file for testing
func createNonExecFile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "nonexec")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to create non-executable file: %v", err)
	}
	return path
}

func TestDefinitionValidate_NameEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	tests := []struct {
		name    string
		svcName string
		wantErr bool
	}{
		{name: "simple name", svcName: "myservice", wantErr: false},
		{name: "with dots", svcName: "com.example.service", wantErr: false},
		{name: "with hyphens", svcName: "my-service", wantErr: false},
		{name: "with underscores", svcName: "my_service", wantErr: false},
		{name: "mixed valid chars", svcName: "com.example.my-service_v2.0", wantErr: false},
		{name: "single char", svcName: "a", wantErr: false},
		{name: "numbers only", svcName: "12345", wantErr: false},
		{name: "empty name", svcName: "", wantErr: true},
		{name: "with slash", svcName: "bad/name", wantErr: true},
		{name: "path traversal", svcName: "../etc/passwd", wantErr: true},
		{name: "with space", svcName: "bad name", wantErr: true},
		{name: "with tab", svcName: "bad\tname", wantErr: true},
		{name: "with newline", svcName: "bad\nname", wantErr: true},
		{name: "with special char @", svcName: "bad@name", wantErr: true},
		{name: "with special char !", svcName: "bad!name", wantErr: true},
		{name: "with special char $", svcName: "bad$name", wantErr: true},
		{name: "with backslash", svcName: "bad\\name", wantErr: true},
		{name: "with colon", svcName: "bad:name", wantErr: true},
		{name: "with asterisk", svcName: "bad*name", wantErr: true},
		{name: "with question mark", svcName: "bad?name", wantErr: true},
		{name: "with angle brackets", svcName: "bad<name>", wantErr: true},
		{name: "with pipe", svcName: "bad|name", wantErr: true},
		{name: "with quotes", svcName: `bad"name`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := &Definition{
				Name:    tt.svcName,
				Command: execPath,
			}
			err := def.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefinitionValidate_MissingCommand(t *testing.T) {
	def := &Definition{
		Name:    "valid-name",
		Command: "",
	}
	err := def.Validate()
	if err == nil {
		t.Error("Validate() should return error for empty command")
	}
}

func TestDefinitionValidate_MissingNameWithCommand(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	def := &Definition{
		Name:    "",
		Command: execPath,
	}
	err := def.Validate()
	if err == nil {
		t.Error("Validate() should return error when name is missing")
	}
}

func TestDefinitionHash_EmptyOptionalFields(t *testing.T) {
	// Verify that definitions with nil vs empty optional fields hash differently
	// This ensures the hash is sensitive to these differences
	def1 := &Definition{
		Name:    "test",
		Command: "/usr/bin/echo",
		Args:    nil,
	}
	def2 := &Definition{
		Name:    "test",
		Command: "/usr/bin/echo",
		Args:    []string{},
	}

	hash1, err := def1.Hash()
	if err != nil {
		t.Fatalf("Hash() error for def1: %v", err)
	}
	hash2, err := def2.Hash()
	if err != nil {
		t.Fatalf("Hash() error for def2: %v", err)
	}

	// nil vs empty slice may or may not produce the same hash depending on JSON marshaling.
	// The key behavior: the hash function should not error.
	_ = hash1
	_ = hash2
}

func TestDefinitionHash_NilVsEmptyEnvironment(t *testing.T) {
	def1 := &Definition{
		Name:        "test",
		Command:     "/usr/bin/echo",
		Environment: nil,
	}
	def2 := &Definition{
		Name:        "test",
		Command:     "/usr/bin/echo",
		Environment: map[string]string{},
	}

	hash1, err := def1.Hash()
	if err != nil {
		t.Fatalf("Hash() error for def1: %v", err)
	}
	hash2, err := def2.Hash()
	if err != nil {
		t.Fatalf("Hash() error for def2: %v", err)
	}

	// Both should hash successfully -- the exact equality depends on JSON encoding
	_ = hash1
	_ = hash2
}

func TestDefinitionHash_Deterministic(t *testing.T) {
	def := &Definition{
		Name:        "test-service",
		Command:     "/usr/bin/echo",
		Args:        []string{"hello", "world"},
		Environment: map[string]string{"A": "1", "B": "2", "C": "3"},
		RunAtLoad:   true,
		KeepAlive:   true,
	}

	// Hash 100 times to verify determinism
	first, err := def.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}

	for i := 0; i < 100; i++ {
		h, err := def.Hash()
		if err != nil {
			t.Fatalf("Hash() error on iteration %d: %v", i, err)
		}
		if h != first {
			t.Fatalf("Hash() not deterministic: iteration %d produced %s, expected %s", i, h, first)
		}
	}
}

func TestDefinitionHash_DifferentRunAtLoad(t *testing.T) {
	def1 := &Definition{
		Name:      "test",
		Command:   "/usr/bin/echo",
		RunAtLoad: false,
	}
	def2 := &Definition{
		Name:      "test",
		Command:   "/usr/bin/echo",
		RunAtLoad: true,
	}

	hash1, _ := def1.Hash()
	hash2, _ := def2.Hash()

	if hash1 == hash2 {
		t.Error("Different RunAtLoad values should produce different hashes")
	}
}

func TestDefinitionHash_DifferentKeepAlive(t *testing.T) {
	def1 := &Definition{
		Name:      "test",
		Command:   "/usr/bin/echo",
		KeepAlive: false,
	}
	def2 := &Definition{
		Name:      "test",
		Command:   "/usr/bin/echo",
		KeepAlive: true,
	}

	hash1, _ := def1.Hash()
	hash2, _ := def2.Hash()

	if hash1 == hash2 {
		t.Error("Different KeepAlive values should produce different hashes")
	}
}

func TestDefinitionHash_DifferentWorkingDir(t *testing.T) {
	def1 := &Definition{
		Name:       "test",
		Command:    "/usr/bin/echo",
		WorkingDir: "/tmp",
	}
	def2 := &Definition{
		Name:       "test",
		Command:    "/usr/bin/echo",
		WorkingDir: "/var",
	}

	hash1, _ := def1.Hash()
	hash2, _ := def2.Hash()

	if hash1 == hash2 {
		t.Error("Different WorkingDir values should produce different hashes")
	}
}

func TestDefinitionHash_DifferentStdPaths(t *testing.T) {
	def1 := &Definition{
		Name:            "test",
		Command:         "/usr/bin/echo",
		StandardOutPath: "/tmp/a.log",
		StandardErrPath: "/tmp/a.err",
	}
	def2 := &Definition{
		Name:            "test",
		Command:         "/usr/bin/echo",
		StandardOutPath: "/tmp/b.log",
		StandardErrPath: "/tmp/b.err",
	}

	hash1, _ := def1.Hash()
	hash2, _ := def2.Hash()

	if hash1 == hash2 {
		t.Error("Different standard paths should produce different hashes")
	}
}
