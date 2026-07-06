package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetHookFlags resets package-level hook flags for test isolation.
// Cobra remembers flag values across rootCmd.Execute() calls.
func resetHookFlags() {
	hooksDryRunFlag = false
	hooksForceFlag = false
	hooksRepoFlag = ""
	hooksJSONFlag = false
}

func setupHookTestRepo(t *testing.T, name string) (path string) {
	t.Helper()
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, name)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitInit(t, repoDir)
	gitSetRemote(t, repoDir, "git@github.com:org/"+name+".git")
	return repoDir
}

func TestHooksInstallWritesHook(t *testing.T) {
	repoDir := setupHookTestRepo(t, "myrepo")
	parent := filepath.Dir(repoDir)

	cfg := "dirs:\n  - " + parent + "\nhooks:\n  post_merge:\n    enabled: true\n"
	_, cleanup := setupTestConfig(t, cfg)
	defer cleanup()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"hooks", "install"})
	resetHookFlags()
	defer resetHookFlags()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v\noutput: %s", err, buf.String())
	}

	hookPath := filepath.Join(repoDir, ".git", "hooks", "post-merge")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("hook not written: %v", err)
	}
	if !strings.Contains(string(data), hookMarker) {
		t.Errorf("hook missing marker: %s", data)
	}
	info, _ := os.Stat(hookPath)
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("hook not executable: %v", info.Mode())
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}

func TestHooksInstallPrintsDeprecationNotice(t *testing.T) {
	repoDir := setupHookTestRepo(t, "deprecated")
	parent := filepath.Dir(repoDir)

	cfg := "dirs:\n  - " + parent + "\nhooks:\n  post_merge:\n    enabled: true\n"
	_, cleanup := setupTestConfig(t, cfg)
	defer cleanup()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"hooks", "install"})
	resetHookFlags()
	defer resetHookFlags()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v\noutput: %s", err, buf.String())
	}

	out := buf.String()
	if !strings.Contains(out, "DEPRECATED") {
		t.Errorf("expected deprecation notice in output, got: %s", out)
	}
	if !strings.Contains(out, "suspenders") {
		t.Errorf("expected suspenders mention in deprecation notice, got: %s", out)
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}

func TestHooksInstallRespectsExclude(t *testing.T) {
	repoDir := setupHookTestRepo(t, "skipme")
	parent := filepath.Dir(repoDir)

	cfg := "dirs:\n  - " + parent + "\nhooks:\n  post_merge:\n    enabled: true\n    exclude:\n      - " + repoDir + "\n"
	_, cleanup := setupTestConfig(t, cfg)
	defer cleanup()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"hooks", "install"})
	resetHookFlags()
	defer resetHookFlags()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	hookPath := filepath.Join(repoDir, ".git", "hooks", "post-merge")
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Errorf("excluded repo should not have hook installed: %v", err)
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}

func TestHooksInstallRefusesForeignHook(t *testing.T) {
	repoDir := setupHookTestRepo(t, "foreign")
	parent := filepath.Dir(repoDir)

	hookPath := filepath.Join(repoDir, ".git", "hooks", "post-merge")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho hand-written\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := "dirs:\n  - " + parent + "\nhooks:\n  post_merge:\n    enabled: true\n"
	_, cleanup := setupTestConfig(t, cfg)
	defer cleanup()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"hooks", "install"})
	resetHookFlags()
	defer resetHookFlags()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	data, _ := os.ReadFile(hookPath)
	if strings.Contains(string(data), hookMarker) {
		t.Errorf("foreign hook should not have been overwritten without --force")
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}

func TestHooksStatusJSON(t *testing.T) {
	repoDir := setupHookTestRepo(t, "statusrepo")
	parent := filepath.Dir(repoDir)

	cfg := "dirs:\n  - " + parent + "\nhooks:\n  post_merge:\n    enabled: true\n"
	_, cleanup := setupTestConfig(t, cfg)
	defer cleanup()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"hooks", "status", "--json"})
	resetHookFlags()
	defer resetHookFlags()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v\noutput: %s", err, buf.String())
	}

	var states []hookState
	if err := json.Unmarshal(buf.Bytes(), &states); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	if states[0].Status != "missing" {
		t.Errorf("expected status=missing before install, got %s", states[0].Status)
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}

func TestHooksUninstall(t *testing.T) {
	repoDir := setupHookTestRepo(t, "uninstallme")
	parent := filepath.Dir(repoDir)

	hookPath := filepath.Join(repoDir, ".git", "hooks", "post-merge")
	if err := os.WriteFile(hookPath, []byte(hookScript), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := "dirs:\n  - " + parent + "\nhooks:\n  post_merge:\n    enabled: true\n"
	_, cleanup := setupTestConfig(t, cfg)
	defer cleanup()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"hooks", "uninstall"})
	resetHookFlags()
	defer resetHookFlags()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Errorf("hook should have been removed: err=%v", err)
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}
