// Package settings reads harvest's settings from the environment. Every
// setting is optional. A Claude Code hook gets no flags, so the environment
// (the settings file's env block, or the shell) is the one place to set them.
package settings

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mad01/thismoon/tools/harvest"
)

// The environment variables harvest reads.
const (
	// EnvChild marks a process the harvest run started. Its own session end
	// must not start another harvest.
	EnvChild = "HARVEST_CHILD"
	// EnvOff switches the hook off when it holds any value.
	EnvOff = "HARVEST_OFF"
	// EnvStateDir relocates the log and the per-session marks.
	EnvStateDir = "HARVEST_STATE_DIR"
	// EnvMinBytes is the digest size below which a session is skipped.
	EnvMinBytes = "HARVEST_MIN_BYTES"
	// EnvMaxBytes caps the digest size; the end of the session is kept.
	EnvMaxBytes = "HARVEST_MAX_BYTES"
	// EnvSkill is the skill file the model gets as its system prompt.
	EnvSkill = "HARVEST_SKILL"
	// EnvBackend forces a model backend: litellm, claude or openrouter.
	EnvBackend = "HARVEST_BACKEND"
	// EnvModel is the model id; empty is the kit/llm default.
	EnvModel = "HARVEST_MODEL"
	// EnvEffort is the reasoning effort; empty keeps the provider default.
	EnvEffort = "HARVEST_EFFORT"
	// EnvSandbox set to off runs the steps without srt.
	EnvSandbox = "HARVEST_SANDBOX"
	// EnvLauncher is the script that starts a step under srt.
	EnvLauncher = "HARVEST_LAUNCHER"
)

// Names lists the settings a step process needs from its parent, in a fixed
// order. The sandbox passes exactly these through its clean environment.
var Names = []string{
	EnvStateDir, EnvMinBytes, EnvMaxBytes, EnvSkill, EnvBackend, EnvModel, EnvEffort,
	EnvSandbox, EnvLauncher,
}

// Defaults for the size settings, in bytes of digest text.
const (
	DefaultMinBytes = 4000
	DefaultMaxBytes = 200000
)

// Defaults for the path settings. A leading ~ expands to $HOME.
const (
	DefaultSkill    = "~/.claude/skills/harvest/SKILL.md"
	DefaultLauncher = "~/code/bin/harvest-step"
)

// sandboxOff is the one value of EnvSandbox that turns the sandbox off.
const sandboxOff = "off"

// Settings is the parsed environment.
type Settings struct {
	StateDir string // absolute path of the state dir
	MinBytes int    // a digest shorter than this is skipped
	MaxBytes int    // a digest longer than this keeps its last MaxBytes bytes
	Skill    string // absolute path of the skill file
	Backend  string // the forced backend; "" auto-selects
	Model    string // the model id; "" is the kit/llm default
	Effort   string // the reasoning effort; "" keeps the provider default
	Sandbox  bool   // run the steps under srt
	Launcher string // absolute path of the script that starts a step under srt
}

// Load parses the settings from getenv (os.Getenv outside tests). A value
// that does not parse is an error naming its variable.
func Load(getenv func(string) string) (Settings, error) {
	s := Settings{
		Backend: getenv(EnvBackend),
		Model:   getenv(EnvModel),
		Effort:  getenv(EnvEffort),
	}
	var err error
	if s.StateDir, err = StateDir(getenv); err != nil {
		return Settings{}, err
	}
	if s.MinBytes, err = byteCount(getenv, EnvMinBytes, DefaultMinBytes, 0); err != nil {
		return Settings{}, err
	}
	if s.MaxBytes, err = byteCount(getenv, EnvMaxBytes, DefaultMaxBytes, 1); err != nil {
		return Settings{}, err
	}
	if s.Skill, err = path(getenv, EnvSkill, DefaultSkill); err != nil {
		return Settings{}, err
	}
	if s.Sandbox, err = sandbox(getenv); err != nil {
		return Settings{}, err
	}
	if s.Launcher, err = path(getenv, EnvLauncher, DefaultLauncher); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// StateDir resolves the state dir on its own: the hook needs the log before
// it has any reason to parse the rest. A leading ~ expands to $HOME.
func StateDir(getenv func(string) string) (string, error) {
	return path(getenv, EnvStateDir, harvest.DefaultStateDir)
}

// path resolves the path in name, or def when it is unset. A leading ~
// expands to $HOME, and the result must be absolute.
func path(getenv func(string) string, name, def string) (string, error) {
	p := getenv(name)
	if p == "" {
		p = def
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home := getenv("HOME")
		if home == "" {
			return "", fmt.Errorf("settings: HOME is not set, so %s cannot be resolved", p)
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("settings: %s must be an absolute path, got %q", name, p)
	}
	return filepath.Clean(p), nil
}

// sandbox reads EnvSandbox. Only "off" turns the sandbox off; any other
// value but "on" is an error, so a typo never runs a step unconfined.
func sandbox(getenv func(string) string) (bool, error) {
	switch v := getenv(EnvSandbox); v {
	case "", "on":
		return true, nil
	case sandboxOff:
		return false, nil
	default:
		return false, fmt.Errorf("settings: %s=%q must be on or off", EnvSandbox, v)
	}
}

// byteCount parses the integer in name, or returns def when it is unset.
func byteCount(getenv func(string) string, name string, def, low int) (int, error) {
	raw := getenv(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("settings: %s=%q is not a whole number: %w", name, raw, err)
	}
	if n < low {
		return 0, fmt.Errorf("settings: %s=%d is below %d", name, n, low)
	}
	return n, nil
}
