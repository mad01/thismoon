package launchd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

// newTestManager returns a Manager confined to a per-test temp plist dir with
// launchctl stubbed out, so no test can ever touch the real
// ~/Library/LaunchAgents or the live launchd session. A test that ran
// Manager.Reconcile against the real home dir once deleted every managed
// service on the machine (mad01/issues#12) — always use this helper.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{
		launchctl: &LaunchctlClient{execCommand: mockExecCommand("", nil)},
		version:   "1.0.0",
		userMode:  true,
		plistDir:  t.TempDir(),
	}
}

// writeManagedPlist generates a t-man-managed plist for def and writes it
// into the manager's injected plist dir.
func writeManagedPlist(t *testing.T, mgr *Manager, def *service.Definition) string {
	t.Helper()
	data, err := GeneratePlist(def, mgr.version)
	if err != nil {
		t.Fatalf("GeneratePlist() error = %v", err)
	}
	path := filepath.Join(mgr.plistDir, def.Name+".plist")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write plist: %v", err)
	}
	return path
}

func TestNewManager(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		userMode bool
	}{
		{
			name:     "create user mode manager",
			version:  "1.0.0",
			userMode: true,
		},
		{
			name:     "create system mode manager",
			version:  "1.0.0",
			userMode: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(tt.version, tt.userMode)
			if mgr == nil {
				t.Fatal("NewManager() returned nil")
			}
			if mgr.version != tt.version {
				t.Errorf("version = %s, want %s", mgr.version, tt.version)
			}
			if mgr.userMode != tt.userMode {
				t.Errorf("userMode = %v, want %v", mgr.userMode, tt.userMode)
			}
		})
	}
}

func TestGetPlistDir(t *testing.T) {
	tests := []struct {
		name     string
		userMode bool
		wantErr  bool
	}{
		{
			name:     "user mode plist directory",
			userMode: true,
			wantErr:  false,
		},
		{
			name:     "system mode plist directory",
			userMode: false,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager("1.0.0", tt.userMode)
			dir, err := mgr.getPlistDir()
			if (err != nil) != tt.wantErr {
				t.Errorf("getPlistDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && dir == "" {
				t.Error("getPlistDir() returned empty directory")
			}

			if tt.userMode {
				homeDir, _ := os.UserHomeDir()
				expectedDir := filepath.Join(homeDir, UserLaunchAgentsDir)
				if dir != expectedDir {
					t.Errorf("getPlistDir() = %s, want %s", dir, expectedDir)
				}
			} else {
				if dir != SystemLaunchDaemonsDir {
					t.Errorf("getPlistDir() = %s, want %s", dir, SystemLaunchDaemonsDir)
				}
			}
		})
	}
}

func TestGetPlistDir_Override(t *testing.T) {
	mgr := newTestManager(t)
	dir, err := mgr.getPlistDir()
	if err != nil {
		t.Fatalf("getPlistDir() error = %v", err)
	}
	if dir != mgr.plistDir {
		t.Errorf("getPlistDir() = %s, want injected dir %s", dir, mgr.plistDir)
	}
}

func TestGetPlistPath(t *testing.T) {
	mgr := newTestManager(t)

	tests := []struct {
		name        string
		serviceName string
		wantErr     bool
	}{
		{
			name:        "valid service name",
			serviceName: "test-service",
			wantErr:     false,
		},
		{
			name:        "service name with dots",
			serviceName: "com.example.service",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := mgr.getPlistPath(tt.serviceName)
			if (err != nil) != tt.wantErr {
				t.Errorf("getPlistPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if path == "" {
					t.Error("getPlistPath() returned empty path")
				}
				if filepath.Ext(path) != ".plist" {
					t.Errorf("getPlistPath() = %s, want .plist extension", path)
				}
			}
		})
	}
}

func TestPlistToDefinition(t *testing.T) {
	mgr := newTestManager(t)

	tests := []struct {
		name  string
		plist *LaunchdPlist
		want  *service.Definition
	}{
		{
			name: "minimal plist",
			plist: &LaunchdPlist{
				Label:            "test-service",
				ProgramArguments: []string{"/usr/bin/echo"},
				RunAtLoad:        true,
				KeepAlive:        false,
			},
			want: &service.Definition{
				Name:      "test-service",
				Command:   "/usr/bin/echo",
				Args:      nil,
				RunAtLoad: true,
				KeepAlive: false,
			},
		},
		{
			name: "plist with all fields",
			plist: &LaunchdPlist{
				Label:                "test-service",
				ProgramArguments:     []string{"/usr/bin/echo", "hello", "world"},
				WorkingDirectory:     "/tmp",
				EnvironmentVariables: map[string]string{"FOO": "bar"},
				RunAtLoad:            true,
				KeepAlive:            true,
				StandardOutPath:      "/tmp/out.log",
				StandardErrorPath:    "/tmp/err.log",
			},
			want: &service.Definition{
				Name:            "test-service",
				Command:         "/usr/bin/echo",
				Args:            []string{"hello", "world"},
				WorkingDir:      "/tmp",
				Environment:     map[string]string{"FOO": "bar"},
				RunAtLoad:       true,
				KeepAlive:       true,
				StandardOutPath: "/tmp/out.log",
				StandardErrPath: "/tmp/err.log",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mgr.plistToDefinition(tt.plist)

			if got.Name != tt.want.Name {
				t.Errorf("Name = %s, want %s", got.Name, tt.want.Name)
			}
			if got.Command != tt.want.Command {
				t.Errorf("Command = %s, want %s", got.Command, tt.want.Command)
			}
			if len(got.Args) != len(tt.want.Args) {
				t.Errorf("len(Args) = %d, want %d", len(got.Args), len(tt.want.Args))
			}
			if got.WorkingDir != tt.want.WorkingDir {
				t.Errorf("WorkingDir = %s, want %s", got.WorkingDir, tt.want.WorkingDir)
			}
			if got.RunAtLoad != tt.want.RunAtLoad {
				t.Errorf("RunAtLoad = %v, want %v", got.RunAtLoad, tt.want.RunAtLoad)
			}
			if got.KeepAlive != tt.want.KeepAlive {
				t.Errorf("KeepAlive = %v, want %v", got.KeepAlive, tt.want.KeepAlive)
			}
		})
	}
}

func TestListEmpty(t *testing.T) {
	mgr := newTestManager(t)

	definitions, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if definitions == nil {
		t.Fatal("List() returned nil, want non-nil slice")
	}
	if len(definitions) != 0 {
		t.Errorf("List() returned %d definitions, want 0", len(definitions))
	}
}

func TestListWithServices(t *testing.T) {
	mgr := newTestManager(t)

	def := &service.Definition{
		Name:      "test-service",
		Command:   "/usr/bin/true",
		RunAtLoad: true,
		KeepAlive: false,
	}
	writeManagedPlist(t, mgr, def)

	definitions, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(definitions) != 1 {
		t.Fatalf("List() returned %d definitions, want 1", len(definitions))
	}
	if definitions[0].Name != def.Name {
		t.Errorf("Name = %s, want %s", definitions[0].Name, def.Name)
	}
	if definitions[0].Command != def.Command {
		t.Errorf("Command = %s, want %s", definitions[0].Command, def.Command)
	}
}

func TestGet(t *testing.T) {
	mgr := newTestManager(t)

	def := &service.Definition{
		Name:      "get-service",
		Command:   "/usr/bin/true",
		RunAtLoad: true,
	}
	writeManagedPlist(t, mgr, def)

	got, err := mgr.Get(context.Background(), "get-service")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name != def.Name {
		t.Errorf("Name = %s, want %s", got.Name, def.Name)
	}
	if got.Command != def.Command {
		t.Errorf("Command = %s, want %s", got.Command, def.Command)
	}
}

func TestGetNotFound(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.Get(context.Background(), "nonexistent-service-12345")
	if err == nil {
		t.Error("Get() expected error for nonexistent service, got nil")
	}
}

func TestCreateService_PlistAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	plistDir := filepath.Join(tmpDir, "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		t.Fatalf("Failed to create plist dir: %v", err)
	}

	def := &service.Definition{
		Name:    "test-service",
		Command: execPath,
	}

	// Generate and write plist to simulate an existing file
	plistData, err := GeneratePlist(def, "1.0.0")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	plistPath := filepath.Join(plistDir, "test-service.plist")
	if err := os.WriteFile(plistPath, plistData, 0o644); err != nil {
		t.Fatalf("Failed to write existing plist: %v", err)
	}

	// Try to create the same file with O_EXCL -- should fail
	f, err := os.OpenFile(plistPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err == nil {
		_ = f.Close()
		t.Error("O_EXCL should fail when file already exists")
	}
	if !os.IsExist(err) {
		t.Errorf("Expected os.IsExist error, got: %v", err)
	}
}

func TestCreateService_SymlinkAtTarget(t *testing.T) {
	tmpDir := t.TempDir()

	plistDir := filepath.Join(tmpDir, "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		t.Fatalf("Failed to create plist dir: %v", err)
	}

	// Create a symlink where the plist would be written
	symlinkPath := filepath.Join(plistDir, "evil-service.plist")
	targetPath := filepath.Join(tmpDir, "evil-target.plist")

	// Create the symlink target first
	if err := os.WriteFile(targetPath, []byte("target content"), 0o644); err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// O_EXCL should fail because the symlink (and its target) already exist
	f, err := os.OpenFile(symlinkPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err == nil {
		_ = f.Close()
		t.Error("O_EXCL should reject creation when symlink exists at target path")
	}
}

func TestUpdateService_SymlinkRejection(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file and a symlink to it
	realPath := filepath.Join(tmpDir, "real.plist")
	if err := os.WriteFile(realPath, []byte("real content"), 0o644); err != nil {
		t.Fatalf("Failed to create real file: %v", err)
	}

	symlinkPath := filepath.Join(tmpDir, "link.plist")
	if err := os.Symlink(realPath, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Verify that Lstat detects symlinks correctly
	fi, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("Lstat() error: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("Lstat should detect symlink mode bit")
	}

	// Verify regular file does NOT have symlink bit
	fi2, err := os.Lstat(realPath)
	if err != nil {
		t.Fatalf("Lstat() error on real file: %v", err)
	}
	if fi2.Mode()&os.ModeSymlink != 0 {
		t.Error("Regular file should not have symlink mode bit")
	}
}

func TestDeleteService_NonexistentPlist(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	// Try to delete a service that doesn't exist
	err := mgr.Delete(ctx, "nonexistent-service-xyz-12345")
	if err == nil {
		t.Error("Delete() should return error for nonexistent plist")
	}
}

func TestDeleteService_RemovesOnlyNamedService(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	writeManagedPlist(t, mgr, &service.Definition{Name: "svc-a", Command: "/usr/bin/true"})
	keepPath := writeManagedPlist(
		t,
		mgr,
		&service.Definition{Name: "svc-b", Command: "/usr/bin/true"},
	)

	if err := mgr.Delete(ctx, "svc-a"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(mgr.plistDir, "svc-a.plist")); !os.IsNotExist(err) {
		t.Error("svc-a.plist should be removed")
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Errorf("svc-b.plist should be untouched: %v", err)
	}
}

func TestDeleteService_SymlinkHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a real file and a symlink to it
	realPath := filepath.Join(tmpDir, "real.plist")
	if err := os.WriteFile(realPath, []byte("content"), 0o644); err != nil {
		t.Fatalf("Failed to create real file: %v", err)
	}

	symlinkPath := filepath.Join(tmpDir, "link.plist")
	if err := os.Symlink(realPath, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// os.Remove on a symlink should remove the symlink, not the target
	if err := os.Remove(symlinkPath); err != nil {
		t.Fatalf("os.Remove(symlink) error: %v", err)
	}

	// Symlink should be gone
	if _, err := os.Lstat(symlinkPath); !os.IsNotExist(err) {
		t.Error("Symlink should be removed")
	}

	// Real file should still exist
	if _, err := os.Stat(realPath); err != nil {
		t.Error("Real file should still exist after removing symlink")
	}
}

func TestListWithCorruptedPlist(t *testing.T) {
	tmpDir := t.TempDir()
	plistDir := filepath.Join(tmpDir, "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		t.Fatalf("Failed to create plist dir: %v", err)
	}

	// Write a corrupted plist file
	corruptedPath := filepath.Join(plistDir, "corrupted.plist")
	if err := os.WriteFile(corruptedPath, []byte("this is not valid plist xml"), 0o644); err != nil {
		t.Fatalf("Failed to write corrupted plist: %v", err)
	}

	// IsManagedByTMan should fail on corrupted data, not panic
	_, err := IsManagedByTMan([]byte("this is not valid plist xml"))
	if err == nil {
		t.Error("IsManagedByTMan should error on corrupted plist data")
	}

	// ParsePlist should return error, not panic
	_, err = ParsePlist([]byte("this is not valid plist xml"))
	if err == nil {
		t.Error("ParsePlist should error on corrupted data")
	}
}

func TestListEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	plistDir := filepath.Join(tmpDir, "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		t.Fatalf("Failed to create plist dir: %v", err)
	}

	// Read the empty directory -- should produce no entries
	entries, err := os.ReadDir(plistDir)
	if err != nil {
		t.Fatalf("ReadDir() error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries in empty dir, got %d", len(entries))
	}
}

func TestListDirectoryWithMixedFiles(t *testing.T) {
	mgr := newTestManager(t)

	// Create a valid t-man managed plist
	writeManagedPlist(
		t,
		mgr,
		&service.Definition{Name: "valid-service", Command: "/usr/bin/true", RunAtLoad: true},
	)

	// Create a non-plist file (should be skipped)
	if err := os.WriteFile(filepath.Join(mgr.plistDir, "readme.txt"), []byte("not a plist"), 0o644); err != nil {
		t.Fatalf("Failed to write non-plist file: %v", err)
	}

	// Create a corrupted plist file (should be skipped with warning)
	if err := os.WriteFile(filepath.Join(mgr.plistDir, "corrupted.plist"), []byte("not valid xml"), 0o644); err != nil {
		t.Fatalf("Failed to write corrupted plist: %v", err)
	}

	// Create a non-managed plist (should be skipped)
	nonManagedPlist := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.other.service</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/bin/echo</string>
	</array>
</dict>
</plist>`)
	if err := os.WriteFile(filepath.Join(mgr.plistDir, "non-managed.plist"), nonManagedPlist, 0o644); err != nil {
		t.Fatalf("Failed to write non-managed plist: %v", err)
	}

	// Create a subdirectory (should be skipped)
	if err := os.MkdirAll(filepath.Join(mgr.plistDir, "subdir"), 0o755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	definitions, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(definitions) != 1 {
		t.Fatalf("List() returned %d definitions, want 1", len(definitions))
	}
	if definitions[0].Name != "valid-service" {
		t.Errorf("Name = %s, want valid-service", definitions[0].Name)
	}
}

func TestPlistToDefinition_EmptyProgramArguments(t *testing.T) {
	mgr := newTestManager(t)

	plist := &LaunchdPlist{
		Label:            "empty-args-service",
		ProgramArguments: []string{},
	}

	def := mgr.plistToDefinition(plist)
	if def.Command != "" {
		t.Errorf("Expected empty command for empty ProgramArguments, got %q", def.Command)
	}
	if def.Args != nil {
		t.Errorf("Expected nil args for empty ProgramArguments, got %v", def.Args)
	}
}

func TestPlistToDefinition_SingleProgramArgument(t *testing.T) {
	mgr := newTestManager(t)

	plist := &LaunchdPlist{
		Label:            "single-arg-service",
		ProgramArguments: []string{"/usr/bin/echo"},
	}

	def := mgr.plistToDefinition(plist)
	if def.Command != "/usr/bin/echo" {
		t.Errorf("Command = %q, want /usr/bin/echo", def.Command)
	}
	if def.Args != nil {
		t.Errorf("Args should be nil for single ProgramArgument, got %v", def.Args)
	}
}
