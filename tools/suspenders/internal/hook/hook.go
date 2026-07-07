// Package hook manages suspenders git hooks in repositories.
package hook

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const marker = "managed by suspenders"

// cslMarker identifies a hook managed by csl (code-search-local). Its
// post-merge hook appends the repo to csl's reindex queue. suspenders already
// queues the same reindex via its csl-reindex post_merge config entry, so on
// takeover the csl hook must NOT be left as an executable <event>.backup: the
// always-chain script would run it on every merge and the reindex would be
// queued twice. We back it up to a non-chaining name instead.
const cslMarker = "# csl-managed-hook:"

// cslReplacedSuffix is where a taken-over csl hook is parked. The always-chain
// script only looks for <event>.backup, never <event>.csl-replaced, so the csl
// reindex is not re-run on every merge.
const cslReplacedSuffix = ".csl-replaced"

// Event represents a git hook event type.
type Event string

const (
	PreCommit Event = "pre-commit"
	PostMerge Event = "post-merge"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// Manager installs and manages suspenders hooks.
type Manager struct {
	BinaryPath string // path to the suspenders binary used in hook scripts
}

// New creates a Manager using PATH-based binary resolution.
func New() *Manager {
	return &Manager{BinaryPath: "suspenders"}
}

// hooksDir resolves the git hooks directory for repoPath. It asks git so that
// core.hooksPath overrides, linked worktrees, and .git-file layouts all resolve
// to the directory git actually runs hooks from. Falls back to the literal
// <repoPath>/.git/hooks only when git is unavailable or the path isn't a repo.
func (m *Manager) hooksDir(repoPath string) string {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "--git-path", "hooks").Output()
	if err != nil {
		return filepath.Join(repoPath, ".git", "hooks")
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return filepath.Join(repoPath, ".git", "hooks")
	}
	// git prints --git-path relative to repoPath (with -C) unless core.hooksPath
	// is an absolute path.
	if !filepath.IsAbs(p) {
		p = filepath.Join(repoPath, p)
	}
	return p
}

func (m *Manager) hookPath(repoPath string, event Event) string {
	return filepath.Join(m.hooksDir(repoPath), string(event))
}

// hookState classifies an existing hook file.
type hookState int

const (
	hookAbsent  hookState = iota // no hook file present
	hookOurs                     // a suspenders-managed regular file
	hookForeign                  // any other hook, including a symlink
)

// classify inspects the hook at hookPath, distinguishing a genuine absence from
// an I/O error (e.g. permission denied) so callers never report a hook we
// cannot read as "not installed". A symlink is always foreign: we never write
// hooks as symlinks, and following one risks corrupting a shared target.
func classify(hookPath string) (hookState, []byte, error) {
	info, err := os.Lstat(hookPath)
	if os.IsNotExist(err) {
		return hookAbsent, nil, nil
	}
	if err != nil {
		return hookAbsent, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return hookForeign, nil, nil
	}
	data, err := os.ReadFile(hookPath)
	if err != nil {
		return hookAbsent, nil, err
	}
	if isManagedByUs(string(data)) {
		return hookOurs, data, nil
	}
	return hookForeign, data, nil
}

// HookScript returns the hook script content for the given event. The script
// chains to a <event>.backup hook (a foreign hook preserved at install time)
// before running suspenders, so pre-existing hooks keep working.
func (m *Manager) HookScript(event Event) string {
	header, body := m.scriptParts(event)
	return header + "# version: " + m.Checksum(event) + "\n" + body
}

// Checksum returns the sha256 hex digest of the hook script body (excluding
// the version line itself) for staleness detection.
func (m *Manager) Checksum(event Event) string {
	header, body := m.scriptParts(event)
	sum := sha256.Sum256([]byte(header + body))
	return fmt.Sprintf("%x", sum)
}

func (m *Manager) scriptParts(event Event) (header, body string) {
	quoted := shellQuote(m.BinaryPath)
	backup := string(event) + ".backup"
	header = "#!/bin/sh\n" +
		"# " + marker + " - do not edit\n"
	body = `hook_dir=$(dirname -- "$0")` + "\n" +
		`if [ -x "$hook_dir/` + backup + `" ]; then` + "\n" +
		`  "$hook_dir/` + backup + `" "$@" || exit $?` + "\n" +
		"fi\n" +
		"exec " + quoted + " hook run " + string(event) + "\n"
	return header, body
}

// Install writes hook scripts to the repo's git hooks dir for the given events.
// If a non-suspenders hook exists for an event, it is backed up with a
// .backup suffix before being replaced.
func (m *Manager) Install(repoPath string, events []Event) error {
	for _, event := range events {
		if err := m.installOne(repoPath, event); err != nil {
			return fmt.Errorf("install %s: %w", event, err)
		}
	}
	return nil
}

func (m *Manager) installOne(repoPath string, event Event) error {
	hooksDir := m.hooksDir(repoPath)
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	hookPath := filepath.Join(hooksDir, string(event))
	state, existing, err := classify(hookPath)
	if err != nil {
		return fmt.Errorf("inspect existing hook: %w", err)
	}

	if state == hookForeign {
		if existing == nil {
			// A symlinked hook (e.g. a dotfiles-managed shared hook). os.WriteFile
			// would follow the link and overwrite the shared target. Move the
			// symlink itself aside to <event>.backup — os.Rename preserves the
			// link — so the always-chain script still runs it, then write a fresh
			// regular file below.
			backupPath := hookPath + ".backup"
			if err := os.Rename(hookPath, backupPath); err != nil {
				return fmt.Errorf("back up symlinked hook: %w", err)
			}
		} else if isManagedByCSL(string(existing)) {
			// csl already queues a reindex on this event; suspenders does the
			// same via its csl-reindex post_merge config entry. Park the csl
			// hook under a non-chaining name (the always-chain script only runs
			// <event>.backup) so the reindex is not queued twice per merge.
			replacedPath := hookPath + cslReplacedSuffix
			if err := os.WriteFile(replacedPath, existing, 0o644); err != nil {
				return fmt.Errorf("set aside csl hook: %w", err)
			}
			fmt.Fprintf(os.Stderr,
				"suspenders: taking over csl-managed %s hook in %s; "+
					"suspenders now owns this event and the reindex runs via the "+
					"csl-reindex post_merge entry (original parked at %s, not chained)\n",
				event, repoPath, replacedPath)
		} else {
			// Generic foreign hook: back it up so the always-chain script runs
			// it before suspenders, preserving its behavior.
			backupPath := hookPath + ".backup"
			if err := os.WriteFile(backupPath, existing, 0o755); err != nil {
				return fmt.Errorf("backup existing hook: %w", err)
			}
		}
	}

	script := m.HookScript(event)
	if err := os.WriteFile(hookPath, []byte(script), 0o755); err != nil {
		return fmt.Errorf("write hook: %w", err)
	}
	return nil
}

// Uninstall removes suspenders hooks and restores any backups.
func (m *Manager) Uninstall(repoPath string, events []Event) error {
	for _, event := range events {
		if err := m.uninstallOne(repoPath, event); err != nil {
			return fmt.Errorf("uninstall %s: %w", event, err)
		}
	}
	return nil
}

func (m *Manager) uninstallOne(repoPath string, event Event) error {
	hookPath := m.hookPath(repoPath, event)

	state, _, err := classify(hookPath)
	if err != nil {
		return fmt.Errorf("inspect hook: %w", err)
	}
	if state != hookOurs {
		// Absent, foreign, or symlinked: nothing of ours to remove.
		return nil
	}

	// Restore a parked original if one exists. os.Rename (not read+write) so a
	// backed-up symlink is restored as a symlink, not flattened to its target's
	// contents. A generic foreign hook is parked at <event>.backup (chained);
	// a taken-over csl hook at <event>.csl-replaced (non-chained). Only one is
	// created per install; prefer .backup for determinism.
	backupPath := hookPath + ".backup"
	if _, err := os.Lstat(backupPath); err == nil {
		return os.Rename(backupPath, hookPath)
	}
	replacedPath := hookPath + cslReplacedSuffix
	if _, err := os.Lstat(replacedPath); err == nil {
		return os.Rename(replacedPath, hookPath)
	}

	return os.Remove(hookPath)
}

// IsInstalled reports whether a suspenders hook is present for the given event.
// A hook that cannot be read (e.g. permission denied) reports false; use Status
// to surface such errors.
func (m *Manager) IsInstalled(repoPath string, event Event) bool {
	state, _, err := classify(m.hookPath(repoPath, event))
	return err == nil && state == hookOurs
}

// NeedsUpdate reports whether the installed hook's checksum differs from current.
func (m *Manager) NeedsUpdate(repoPath string, event Event) bool {
	state, data, err := classify(m.hookPath(repoPath, event))
	if err != nil || state != hookOurs {
		return false
	}
	return hookNeedsUpdate(string(data), m.Checksum(event))
}

// hookNeedsUpdate reports whether a managed hook's embedded version line differs
// from current. A hook with no version line is treated as needing an update.
func hookNeedsUpdate(content, current string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "# version:"); ok {
			return strings.TrimSpace(v) != current
		}
	}
	return true
}

// Update rewrites an installed hook with the current script.
func (m *Manager) Update(repoPath string, event Event) error {
	hookPath := m.hookPath(repoPath, event)
	state, _, err := classify(hookPath)
	if err != nil {
		return fmt.Errorf("inspect hook: %w", err)
	}
	if state != hookOurs {
		return fmt.Errorf("no suspenders hook installed for %s in %s", event, repoPath)
	}
	return os.WriteFile(hookPath, []byte(m.HookScript(event)), 0o755)
}

// Status returns the hook status for the given event: "installed", "outdated",
// "not-installed", "foreign", or "error" (the hook exists but could not be read).
func (m *Manager) Status(repoPath string, event Event) string {
	state, data, err := classify(m.hookPath(repoPath, event))
	if err != nil {
		return "error"
	}
	switch state {
	case hookAbsent:
		return "not-installed"
	case hookForeign:
		return "foreign"
	}
	if hookNeedsUpdate(string(data), m.Checksum(event)) {
		return "outdated"
	}
	return "installed"
}

func isManagedByUs(content string) bool {
	return strings.Contains(content, marker)
}

func isManagedByCSL(content string) bool {
	return strings.Contains(content, cslMarker)
}
