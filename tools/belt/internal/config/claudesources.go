package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The Claude Code settings scopes belt reads auto memory from. Claude Code's
// precedence, highest first: managed, project local, shared project, user
// (https://code.claude.com/docs/en/settings, "Settings precedence"). The
// --settings flag sits between managed and local; a hook cannot see it.
const (
	ScopeUser    = "user"
	ScopeProject = "project"
	ScopeLocal   = "local"
	ScopeManaged = "managed"
)

// ClaudeManagedDir is the macOS system directory of Claude Code's file-based
// managed settings: managed-settings.json and the managed-settings.d
// drop-ins. belt reads no MDM profile and no server-managed settings.
const ClaudeManagedDir = "/Library/Application Support/ClaudeCode"

// EnvDisableAutoMemory is Claude Code's switch for auto memory. It outranks
// autoMemoryEnabled in either direction: 1 turns auto memory off, 0 keeps it
// on whatever the settings say.
const EnvDisableAutoMemory = "CLAUDE_CODE_DISABLE_AUTO_MEMORY"

// SettingsSource is one Claude Code settings file and the scope Claude Code
// reads it at.
type SettingsSource struct {
	Scope string
	Path  string
}

// fromProject reports a file a repository can supply: Claude Code honors
// such a file's autoMemoryDirectory, but not under the read block. A local
// file may be the user's own and untracked, which Claude Code honors under
// the block too. belt does not ask git which it is and counts every local
// file as supplied, so in doubt the store stays in.
func (s SettingsSource) fromProject() bool {
	return s.Scope == ScopeProject || s.Scope == ScopeLocal
}

func (s SettingsSource) String() string { return fmt.Sprintf("%s settings (%s)", s.Scope, s.Path) }

// ClaudeSources locates the Claude Code settings sources that do not depend
// on the session directory. An empty field reads no file there.
type ClaudeSources struct {
	User       string // ~/.claude/settings.json
	ManagedDir string // ClaudeManagedDir
}

// For lists the sources Claude Code reads for a session started in cwd,
// lowest precedence first, so a later source wins. root is the main checkout
// root of the git repository holding cwd, "" outside one. The shared project
// file is read from cwd. The project local file sits at root (for a linked
// worktree, the main checkout's root), with cwd's own file below it, which
// Claude Code still reads. Outside a repository, or when the root is the
// home directory, the local file sits in cwd. So ~/.claude/settings.local.json
// is a source only for a session started in the home directory. A
// managed-settings.d directory that cannot be listed is an error.
func (c ClaudeSources) For(cwd, root string) ([]SettingsSource, error) {
	var out []SettingsSource
	if c.User != "" {
		out = append(out, SettingsSource{ScopeUser, c.User})
	}
	out = append(out, projectSources(cwd, root, c.User)...)
	managed, err := managedSources(c.ManagedDir)
	return append(out, managed...), err
}

func projectSources(cwd, root, user string) []SettingsSource {
	if cwd == "" {
		return nil
	}
	var out []SettingsSource
	// A session in the home directory reads ~/.claude/settings.json as both
	// files; listing it once keeps its scope the user one.
	if shared := filepath.Join(cwd, ".claude", "settings.json"); shared != user {
		out = append(out, SettingsSource{ScopeProject, shared})
	}
	out = append(
		out,
		SettingsSource{ScopeLocal, filepath.Join(cwd, ".claude", "settings.local.json")},
	)
	home, _ := os.UserHomeDir()
	if root != "" && root != home && filepath.Clean(root) != filepath.Clean(cwd) {
		out = append(
			out,
			SettingsSource{ScopeLocal, filepath.Join(root, ".claude", "settings.local.json")},
		)
	}
	return out
}

// managedSources lists managed-settings.json, then every visible *.json file
// in managed-settings.d in name order, the order Claude Code merges them in.
func managedSources(dir string) ([]SettingsSource, error) {
	if dir == "" {
		return nil, nil
	}
	out := []SettingsSource{{ScopeManaged, filepath.Join(dir, "managed-settings.json")}}
	drops := filepath.Join(dir, "managed-settings.d")
	entries, err := os.ReadDir(drops)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		out = append(out, SettingsSource{ScopeManaged, filepath.Join(drops, name)})
	}
	return out, nil
}

// AutoMemory is what Claude Code's settings say about auto memory for one
// session.
type AutoMemory struct {
	// Directory is autoMemoryDirectory as written in the highest source that
	// sets it, "" when none does.
	Directory string
	Source    SettingsSource // where Directory came from
	// Off says why Claude Code may load no auto memory from Directory in
	// this session, "" when belt sees nothing that stops it.
	Off string
}

// autoMemoryKeys are the keys ResolveAutoMemory reads from one file. A
// pointer tells a key set to its zero value from one left out.
type autoMemoryKeys struct {
	AutoMemoryDirectory *string `json:"autoMemoryDirectory"`
	AutoMemoryEnabled   *bool   `json:"autoMemoryEnabled"`
	Permissions         struct {
		BlockReadsOutsideWorkingDirectories *bool `json:"blockReadsOutsideWorkingDirectories"`
	} `json:"permissions"`
}

// autoMemoryState gathers the keys across sources: each later source
// replaces a key it sets, and the read block holds once any source sets it.
type autoMemoryState struct {
	dir                    AutoMemory
	enabled                *bool
	enabledFrom, blockedBy SettingsSource
	isBlocked              bool
}

// ResolveAutoMemory reads autoMemoryDirectory, autoMemoryEnabled and
// permissions.blockReadsOutsideWorkingDirectories from sources, lowest
// precedence first, and getenv's EnvDisableAutoMemory. A key set to "" counts
// as set. A missing file adds nothing. A file that cannot be read or does not
// parse, or a key of the wrong type, is an error: belt then cannot tell what
// Claude Code resolved. The agent-memory hint and belt doctor are the only
// readers.
func ResolveAutoMemory(sources []SettingsSource, getenv func(string) string) (AutoMemory, error) {
	var st autoMemoryState
	for _, src := range sources {
		keys, found, err := readAutoMemoryKeys(src.Path)
		if err != nil {
			return AutoMemory{}, err
		}
		if found {
			st.add(src, keys)
		}
	}
	am := st.dir
	am.Off = st.off(getenv(EnvDisableAutoMemory))
	return am, nil
}

func (st *autoMemoryState) add(src SettingsSource, keys autoMemoryKeys) {
	if keys.AutoMemoryDirectory != nil {
		st.dir = AutoMemory{Directory: *keys.AutoMemoryDirectory, Source: src}
	}
	if keys.AutoMemoryEnabled != nil {
		st.enabled, st.enabledFrom = keys.AutoMemoryEnabled, src
	}
	if b := keys.Permissions.BlockReadsOutsideWorkingDirectories; b != nil && *b && !st.isBlocked {
		st.blockedBy, st.isBlocked = src, true
	}
}

// off names the switch that stops Claude Code loading the directory, "" when
// none does. The variable is read liberally for off and strictly for on: a
// value belt misreads as off keeps the store, which costs a second load at
// worst, while one misread as on would leave its facts out of every session.
func (st *autoMemoryState) off(env string) string {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "1", "true", "yes", "on":
		return fmt.Sprintf("auto memory is off: %s=%s", EnvDisableAutoMemory, env)
	case "0":
	default:
		if st.enabled != nil && !*st.enabled {
			return "auto memory is off: autoMemoryEnabled is false in " + st.enabledFrom.String()
		}
	}
	if st.isBlocked && st.dir.Source.fromProject() {
		return "permissions.blockReadsOutsideWorkingDirectories is on in " + st.blockedBy.String() +
			", and the directory came from a project or local file"
	}
	return ""
}

func readAutoMemoryKeys(path string) (autoMemoryKeys, bool, error) {
	var keys autoMemoryKeys
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return keys, false, nil
	}
	if err != nil {
		return keys, false, err
	}
	if err := json.Unmarshal(raw, &keys); err != nil {
		return keys, false, fmt.Errorf("%s: %w", path, err)
	}
	return keys, true, nil
}
