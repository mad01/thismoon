package hook

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newTestManager() *Manager {
	return &Manager{BinaryPath: "/usr/local/bin/suspenders"}
}

func makeGitDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	hooksDir := filepath.Join(root, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestHookScript_containsMarker(t *testing.T) {
	m := newTestManager()
	script := m.HookScript(PreCommit)
	if !strings.Contains(script, marker) {
		t.Error("hook script missing marker")
	}
	if !strings.Contains(script, m.BinaryPath) {
		t.Error("hook script missing binary path")
	}
	if !strings.Contains(script, m.Checksum(PreCommit)) {
		t.Error("hook script missing checksum")
	}
}

func TestHookScript_containsEvent(t *testing.T) {
	m := newTestManager()
	for _, event := range []Event{PreCommit, PostMerge} {
		script := m.HookScript(event)
		if !strings.Contains(script, "hook run "+string(event)) {
			t.Errorf("script for %s missing 'hook run %s'", event, event)
		}
	}
}

func TestChecksum_deterministic(t *testing.T) {
	m := newTestManager()
	a := m.Checksum(PreCommit)
	b := m.Checksum(PreCommit)
	if a != b {
		t.Errorf("Checksum not deterministic: %q vs %q", a, b)
	}
}

func TestChecksum_changesWithBinary(t *testing.T) {
	a := (&Manager{BinaryPath: "/a"}).Checksum(PreCommit)
	b := (&Manager{BinaryPath: "/b"}).Checksum(PreCommit)
	if a == b {
		t.Error("different binary paths should produce different checksums")
	}
}

func TestChecksum_changesWithEvent(t *testing.T) {
	m := newTestManager()
	a := m.Checksum(PreCommit)
	b := m.Checksum(PostMerge)
	if a == b {
		t.Error("different events should produce different checksums")
	}
}

func TestInstall_fresh(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if !m.IsInstalled(root, PreCommit) {
		t.Error("hook should be installed")
	}
	if m.Status(root, PreCommit) != "installed" {
		t.Errorf("Status = %q, want installed", m.Status(root, PreCommit))
	}
}

func TestInstall_multiEvent(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	if err := m.Install(root, []Event{PreCommit, PostMerge}); err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if !m.IsInstalled(root, PreCommit) {
		t.Error("pre-commit hook should be installed")
	}
	if !m.IsInstalled(root, PostMerge) {
		t.Error("post-merge hook should be installed")
	}
}

func TestInstall_overwriteOwn(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatal(err)
	}
	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatalf("second Install error: %v", err)
	}
	backup := filepath.Join(root, ".git", "hooks", "pre-commit.backup")
	if _, err := os.Stat(backup); err == nil {
		t.Error("should not create backup when overwriting own hook")
	}
}

func TestInstall_foreignHookBacked(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	foreign := []byte("#!/bin/sh\necho foreign\n")
	hookPath := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hookPath, foreign, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatalf("Install error: %v", err)
	}

	backup := filepath.Join(root, ".git", "hooks", "pre-commit.backup")
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if string(data) != string(foreign) {
		t.Error("backup content mismatch")
	}

	installed, _ := os.ReadFile(hookPath)
	if !strings.Contains(string(installed), "hook run pre-commit") {
		t.Error("installed hook should call 'hook run pre-commit'")
	}
}

func TestInstall_genericForeignHookChained(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	foreign := []byte("#!/bin/sh\necho generic-foreign\n")
	hookPath := filepath.Join(root, ".git", "hooks", "post-merge")
	if err := os.WriteFile(hookPath, foreign, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := m.Install(root, []Event{PostMerge}); err != nil {
		t.Fatalf("Install error: %v", err)
	}

	// Generic foreign hook must be parked at <event>.backup and stay
	// executable so the always-chain script runs it before suspenders.
	backup := filepath.Join(root, ".git", "hooks", "post-merge.backup")
	info, err := os.Stat(backup)
	if err != nil {
		t.Fatalf("generic foreign hook should be backed up to .backup: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Error(".backup should be executable so the chain runs the original hook")
	}

	installed, _ := os.ReadFile(hookPath)
	if !strings.Contains(string(installed), "post-merge.backup") {
		t.Error("installed hook should reference <event>.backup for chaining")
	}
}

func TestInstall_cslHookNotChained(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	csl := []byte("#!/bin/sh\n" + cslMarker + " reindex\ncsl reindex queue\n")
	hookPath := filepath.Join(root, ".git", "hooks", "post-merge")
	if err := os.WriteFile(hookPath, csl, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := m.Install(root, []Event{PostMerge}); err != nil {
		t.Fatalf("Install error: %v", err)
	}

	backup := filepath.Join(root, ".git", "hooks", "post-merge.backup")
	if _, err := os.Stat(backup); err == nil {
		t.Fatal("csl takeover must NOT leave an executable <event>.backup " +
			"(would auto-run the csl reindex and double-queue)")
	}

	// The csl hook is parked under a non-chaining name for reference.
	replaced := filepath.Join(root, ".git", "hooks", "post-merge"+cslReplacedSuffix)
	data, err := os.ReadFile(replaced)
	if err != nil {
		t.Fatalf("csl hook should be parked at %s: %v", replaced, err)
	}
	if string(data) != string(csl) {
		t.Error("parked csl hook content mismatch")
	}

	if !m.IsInstalled(root, PostMerge) {
		t.Error("suspenders hook should be installed after csl takeover")
	}
}

func TestInstall_cslHookIdempotent(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	csl := []byte("#!/bin/sh\n" + cslMarker + " reindex\ncsl reindex queue\n")
	hookPath := filepath.Join(root, ".git", "hooks", "post-merge")
	if err := os.WriteFile(hookPath, csl, 0o755); err != nil {
		t.Fatal(err)
	}

	// Install twice: re-running over the resulting suspenders hook must stay
	// stable and must never resurrect an executable <event>.backup.
	for i := 0; i < 2; i++ {
		if err := m.Install(root, []Event{PostMerge}); err != nil {
			t.Fatalf("Install #%d error: %v", i+1, err)
		}
		backup := filepath.Join(root, ".git", "hooks", "post-merge.backup")
		if _, err := os.Stat(backup); err == nil {
			t.Fatalf("install #%d resurrected an executable .backup from csl hook", i+1)
		}
		if m.Status(root, PostMerge) != "installed" {
			t.Errorf(
				"after install #%d, Status = %q, want installed",
				i+1,
				m.Status(root, PostMerge),
			)
		}
	}
}

func TestUninstall_removesHook(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(root, []Event{PreCommit}); err != nil {
		t.Fatalf("Uninstall error: %v", err)
	}
	if m.IsInstalled(root, PreCommit) {
		t.Error("hook should be removed")
	}
}

func TestUninstall_restoresBackup(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	foreign := []byte("#!/bin/sh\necho foreign\n")
	hookPath := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hookPath, foreign, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(root, []Event{PreCommit}); err != nil {
		t.Fatalf("Uninstall error: %v", err)
	}

	restored, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("hook not restored: %v", err)
	}
	if string(restored) != string(foreign) {
		t.Error("restored hook content mismatch")
	}
}

func TestUninstall_multiEvent(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	if err := m.Install(root, []Event{PreCommit, PostMerge}); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(root, []Event{PreCommit, PostMerge}); err != nil {
		t.Fatal(err)
	}
	if m.IsInstalled(root, PreCommit) {
		t.Error("pre-commit should be removed")
	}
	if m.IsInstalled(root, PostMerge) {
		t.Error("post-merge should be removed")
	}
}

func TestNeedsUpdate(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatal(err)
	}
	if m.NeedsUpdate(root, PreCommit) {
		t.Error("freshly installed hook should not need update")
	}

	hookPath := filepath.Join(root, ".git", "hooks", "pre-commit")
	data, _ := os.ReadFile(hookPath)
	tampered := strings.ReplaceAll(string(data), m.Checksum(PreCommit), "deadbeef")
	_ = os.WriteFile(hookPath, []byte(tampered), 0o755)

	if !m.NeedsUpdate(root, PreCommit) {
		t.Error("tampered hook should need update")
	}
}

func TestStatus_states(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	if got := m.Status(root, PreCommit); got != "not-installed" {
		t.Errorf("Status before install = %q, want not-installed", got)
	}

	hookPath := filepath.Join(root, ".git", "hooks", "pre-commit")
	_ = os.WriteFile(hookPath, []byte("#!/bin/sh\necho hi\n"), 0o755)
	if got := m.Status(root, PreCommit); got != "foreign" {
		t.Errorf("Status with foreign hook = %q, want foreign", got)
	}

	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatal(err)
	}
	if got := m.Status(root, PreCommit); got != "installed" {
		t.Errorf("Status after install = %q, want installed", got)
	}

	data, _ := os.ReadFile(hookPath)
	tampered := strings.ReplaceAll(string(data), m.Checksum(PreCommit), "deadbeef")
	_ = os.WriteFile(hookPath, []byte(tampered), 0o755)
	if got := m.Status(root, PreCommit); got != "outdated" {
		t.Errorf("Status when outdated = %q, want outdated", got)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func initRealRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "test")
	return root
}

// Finding 5: a symlinked hook (e.g. a dotfiles-managed shared hook) must not be
// written through — os.WriteFile would follow the link and corrupt the shared
// target. Install moves the symlink aside and writes a fresh regular file.
func TestInstall_symlinkHookNotFollowed(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	shared := filepath.Join(t.TempDir(), "shared-pre-commit")
	sharedContent := "#!/bin/sh\necho shared-dotfiles-hook\n"
	if err := os.WriteFile(shared, []byte(sharedContent), 0o755); err != nil {
		t.Fatal(err)
	}

	hookPath := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.Symlink(shared, hookPath); err != nil {
		t.Fatal(err)
	}

	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatalf("Install error: %v", err)
	}

	got, err := os.ReadFile(shared)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sharedContent {
		t.Errorf("install modified the symlink target: got %q", got)
	}

	info, err := os.Lstat(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("hook path should be a regular file after install, not a symlink")
	}
	if !m.IsInstalled(root, PreCommit) {
		t.Error("suspenders hook should be installed")
	}

	backup := filepath.Join(root, ".git", "hooks", "pre-commit.backup")
	binfo, err := os.Lstat(backup)
	if err != nil {
		t.Fatalf("symlink should be parked at .backup: %v", err)
	}
	if binfo.Mode()&os.ModeSymlink == 0 {
		t.Error(".backup should still be a symlink (os.Rename preserves it)")
	}
}

// Finding 5: uninstall must restore the parked symlink as a symlink, not a
// flattened copy of the target's contents.
func TestUninstall_restoresSymlink(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	shared := filepath.Join(t.TempDir(), "shared-pre-commit")
	if err := os.WriteFile(shared, []byte("#!/bin/sh\necho shared\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.Symlink(shared, hookPath); err != nil {
		t.Fatal(err)
	}

	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(root, []Event{PreCommit}); err != nil {
		t.Fatalf("Uninstall error: %v", err)
	}

	info, err := os.Lstat(hookPath)
	if err != nil {
		t.Fatalf("hook not restored: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("uninstall should restore the symlink, not a flattened copy")
	}
	target, err := os.Readlink(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if target != shared {
		t.Errorf("restored symlink points to %q, want %q", target, shared)
	}
	if _, err := os.Lstat(hookPath + ".backup"); !os.IsNotExist(err) {
		t.Error(".backup should be removed after restore")
	}
}

// Finding 6: after a csl takeover the original is parked at <event>.csl-replaced.
// Uninstall must restore it, not orphan it and delete the suspenders hook.
func TestUninstall_restoresCSLReplaced(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	csl := []byte("#!/bin/sh\n" + cslMarker + " reindex\ncsl reindex queue\n")
	hookPath := filepath.Join(root, ".git", "hooks", "post-merge")
	if err := os.WriteFile(hookPath, csl, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := m.Install(root, []Event{PostMerge}); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(root, []Event{PostMerge}); err != nil {
		t.Fatalf("Uninstall error: %v", err)
	}

	restored, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("csl hook not restored: %v", err)
	}
	if string(restored) != string(csl) {
		t.Errorf("restored hook content mismatch: got %q", restored)
	}
	replaced := filepath.Join(root, ".git", "hooks", "post-merge"+cslReplacedSuffix)
	if _, err := os.Stat(replaced); !os.IsNotExist(err) {
		t.Error(".csl-replaced should be removed after restore")
	}
}

// Finding 13: an unreadable hook (permission denied) must not be reported as
// "not-installed". Status surfaces it as "error".
func TestStatus_readErrorSurfaced(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	m := newTestManager()
	root := makeGitDir(t)
	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatal(err)
	}
	hooksDir := filepath.Join(root, ".git", "hooks")
	if err := os.Chmod(hooksDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(hooksDir, 0o755) })

	if got := m.Status(root, PreCommit); got != "error" {
		t.Errorf("Status with unreadable hooks dir = %q, want error", got)
	}
	if m.IsInstalled(root, PreCommit) {
		t.Error("IsInstalled should report false when the hook can't be read")
	}
}

// Finding 14: hooks must resolve via git so core.hooksPath wins over the naive
// <repo>/.git/hooks path.
func TestInstall_respectsCoreHooksPath(t *testing.T) {
	m := newTestManager()
	root := initRealRepo(t)

	customHooks := filepath.Join(root, "myhooks")
	runGit(t, root, "config", "core.hooksPath", customHooks)

	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatalf("Install error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(customHooks, "pre-commit"))
	if err != nil {
		t.Fatalf("hook not written to core.hooksPath dir: %v", err)
	}
	if !strings.Contains(string(data), "hook run pre-commit") {
		t.Error("installed hook missing dispatch line")
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "hooks", "pre-commit")); err == nil {
		t.Error("hook should not be written to .git/hooks when core.hooksPath is set")
	}

	if got := m.Status(root, PreCommit); got != "installed" {
		t.Errorf("Status = %q, want installed", got)
	}
	if !m.IsInstalled(root, PreCommit) {
		t.Error("IsInstalled should be true")
	}
}

// Finding 14: in a linked worktree, .git is a file; hooks live in the shared
// hooks dir. git resolution handles this; the naive path does not.
func TestInstall_linkedWorktree(t *testing.T) {
	m := newTestManager()
	main := initRealRepo(t)
	runGit(t, main, "commit", "--allow-empty", "-q", "-m", "init")

	wt := filepath.Join(t.TempDir(), "wt")
	runGit(t, main, "worktree", "add", "-q", wt)

	if err := m.Install(wt, []Event{PreCommit}); err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if !m.IsInstalled(wt, PreCommit) {
		t.Error("hook should be installed for the worktree")
	}
	if got := m.Status(wt, PreCommit); got != "installed" {
		t.Errorf("Status = %q, want installed", got)
	}
	if info, err := os.Stat(filepath.Join(wt, ".git", "hooks", "pre-commit")); err == nil && !info.IsDir() {
		t.Error("hook should not be written under <worktree>/.git/hooks")
	}
}

func TestUpdate(t *testing.T) {
	m := newTestManager()
	root := makeGitDir(t)

	if err := m.Install(root, []Event{PreCommit}); err != nil {
		t.Fatal(err)
	}

	hookPath := filepath.Join(root, ".git", "hooks", "pre-commit")
	data, _ := os.ReadFile(hookPath)
	_ = os.WriteFile(
		hookPath,
		[]byte(strings.ReplaceAll(string(data), m.Checksum(PreCommit), "deadbeef")),
		0o755,
	)

	if err := m.Update(root, PreCommit); err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if m.NeedsUpdate(root, PreCommit) {
		t.Error("hook still needs update after Update()")
	}
}
