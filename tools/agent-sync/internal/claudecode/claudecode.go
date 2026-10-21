// Package claudecode holds what the sections know about Claude Code itself:
// the name of its CLI and how to find it on PATH, and the variable that
// moves its config directory. settings, mods and mcp share these, so each
// fact lives here once.
package claudecode

import (
	"errors"
	"os/exec"
)

const (
	// Bin is the claude CLI's name on PATH.
	Bin = "claude"
	// ConfigDirEnv is the variable Claude Code reads its config directory
	// from.
	ConfigDirEnv = "CLAUDE_CONFIG_DIR"
	// DefaultConfigDir is Claude Code's config directory when ConfigDirEnv
	// is unset or empty.
	DefaultConfigDir = "~/.claude"
)

// ErrNotFound reports a claude CLI missing from PATH, worded as the sync
// scripts worded it.
var ErrNotFound = errors.New("`claude` CLI not found on PATH")

// LookPath returns the path of the claude CLI on PATH, or ErrNotFound.
//
// When the first claude on PATH sits in a relative entry (".", or an empty
// one), it counts as missing, as exec.LookPath's ErrDot has it. That is
// stricter than the scripts' shutil.which, which returned the relative path
// and ran that claude.
func LookPath() (string, error) {
	path, err := exec.LookPath(Bin)
	if err != nil {
		return "", ErrNotFound
	}
	return path, nil
}
