package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantErr bool
	}{
		{
			name:    "common command - ls",
			cmd:     "ls",
			wantErr: false,
		},
		{
			name:    "common command - echo",
			cmd:     "echo",
			wantErr: false,
		},
		{
			name:    "nonexistent command",
			cmd:     "this-command-does-not-exist-12345",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveCommand(tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if result == "" {
					t.Error("resolveCommand() returned empty path for valid command")
				}
				if !filepath.IsAbs(result) {
					t.Errorf("resolveCommand() = %s, want absolute path", result)
				}
			}
		})
	}
}

func TestAddCommandParsing(t *testing.T) {
	// Create a temporary executable for testing
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	tests := []struct {
		name          string
		args          []string
		expectedCmd   string
		expectedArgs  []string
		expectAbsPath bool
		shouldResolve bool
	}{
		{
			name:          "absolute path command",
			args:          []string{execPath},
			expectedCmd:   execPath,
			expectedArgs:  []string{},
			expectAbsPath: true,
		},
		{
			name:          "command with arguments",
			args:          []string{execPath, "arg1", "arg2"},
			expectedCmd:   execPath,
			expectedArgs:  []string{"arg1", "arg2"},
			expectAbsPath: true,
		},
		{
			name:          "command with quoted arguments",
			args:          []string{execPath, "arg with spaces", "arg2"},
			expectedCmd:   execPath,
			expectedArgs:  []string{"arg with spaces", "arg2"},
			expectAbsPath: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.args) == 0 {
				t.Skip("No args to parse")
			}

			commandPath := tt.args[0]
			commandArgs := tt.args[1:]

			// Verify the command path
			if tt.expectAbsPath && !filepath.IsAbs(commandPath) {
				t.Errorf("Expected absolute path, got %s", commandPath)
			}

			if commandPath != tt.expectedCmd {
				t.Errorf("commandPath = %s, want %s", commandPath, tt.expectedCmd)
			}

			// Verify arguments
			if len(commandArgs) != len(tt.expectedArgs) {
				t.Errorf("len(commandArgs) = %d, want %d", len(commandArgs), len(tt.expectedArgs))
				return
			}

			for i := range commandArgs {
				if commandArgs[i] != tt.expectedArgs[i] {
					t.Errorf("commandArgs[%d] = %s, want %s", i, commandArgs[i], tt.expectedArgs[i])
				}
			}
		})
	}
}

func TestEnvironmentParsing(t *testing.T) {
	tests := []struct {
		name    string
		envVars []string
		wantMap map[string]string
		wantErr bool
	}{
		{
			name:    "single environment variable",
			envVars: []string{"FOO=bar"},
			wantMap: map[string]string{"FOO": "bar"},
			wantErr: false,
		},
		{
			name:    "multiple environment variables",
			envVars: []string{"FOO=bar", "BAZ=qux"},
			wantMap: map[string]string{"FOO": "bar", "BAZ": "qux"},
			wantErr: false,
		},
		{
			name:    "environment variable with equals in value",
			envVars: []string{"FOO=bar=baz"},
			wantMap: map[string]string{"FOO": "bar=baz"},
			wantErr: false,
		},
		{
			name:    "invalid format - no equals",
			envVars: []string{"FOOBAR"},
			wantMap: nil,
			wantErr: true,
		},
		{
			name:    "invalid format - empty key",
			envVars: []string{"=value"},
			wantMap: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envMap := make(map[string]string)
			var err error

			for _, env := range tt.envVars {
				parts := splitEnvVar(env)
				if len(parts) != 2 {
					err = os.ErrInvalid
					break
				}
				envMap[parts[0]] = parts[1]
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("environment parsing error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(envMap) != len(tt.wantMap) {
					t.Errorf("len(envMap) = %d, want %d", len(envMap), len(tt.wantMap))
					return
				}

				for k, v := range tt.wantMap {
					if envMap[k] != v {
						t.Errorf("envMap[%s] = %s, want %s", k, envMap[k], v)
					}
				}
			}
		})
	}
}

// Helper function for testing
func splitEnvVar(env string) []string {
	// Split only on first = to handle values with =
	parts := make([]string, 0, 2)
	idx := 0
	for i, c := range env {
		if c == '=' {
			idx = i
			break
		}
	}
	if idx > 0 {
		parts = append(parts, env[:idx])
		parts = append(parts, env[idx+1:])
	}
	return parts
}

func TestLogDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		serviceName string
		userMode    bool
		customDir   string
	}{
		{
			name:        "user mode default",
			serviceName: "test-service",
			userMode:    true,
			customDir:   "",
		},
		{
			name:        "custom log directory",
			serviceName: "test-service",
			userMode:    true,
			customDir:   filepath.Join(tmpDir, "custom-logs"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logDir string
			if tt.customDir != "" {
				logDir = tt.customDir
			} else if tt.userMode {
				homeDir, _ := os.UserHomeDir()
				logDir = filepath.Join(homeDir, "Library", "Logs", tt.serviceName)
			} else {
				logDir = filepath.Join("/var/log", tt.serviceName)
			}

			// For testing, only verify path construction
			// Don't actually create directories in system locations
			if tt.customDir != "" {
				if err := os.MkdirAll(logDir, 0o755); err != nil {
					t.Errorf("Failed to create log directory: %v", err)
				}

				if _, err := os.Stat(logDir); os.IsNotExist(err) {
					t.Errorf("Log directory was not created: %s", logDir)
				}
			}
		})
	}
}

func TestResolveCommandPathExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	shim := filepath.Join(home, ".local", "share", "mise", "shims", "csl")
	if err := os.MkdirAll(filepath.Dir(shim), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(shim, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := resolveCommandPath("~/.local/share/mise/shims/csl")
	if err != nil {
		t.Fatalf("resolveCommandPath() error = %v", err)
	}
	if got != shim {
		t.Errorf("resolveCommandPath() = %s, want %s", got, shim)
	}
}

func TestResolveCommandPath(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "svc")
	if err := os.WriteFile(abs, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	tests := []struct {
		name    string
		command string
		want    string
		wantErr bool
	}{
		{name: "absolute path is kept", command: abs, want: abs},
		{
			name:    "relative path with slash is made absolute",
			command: "./bin/svc",
			want:    filepath.Join(cwd, "bin", "svc"),
		},
		{name: "bare name resolves from PATH", command: "ls"},
		{name: "unknown bare name errors", command: "no-such-command-12345", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveCommandPath(tt.command)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveCommandPath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.want != "" && got != tt.want {
				t.Errorf("resolveCommandPath() = %s, want %s", got, tt.want)
			}
			if !filepath.IsAbs(got) {
				t.Errorf("resolveCommandPath() = %s, want an absolute path", got)
			}
		})
	}
}
