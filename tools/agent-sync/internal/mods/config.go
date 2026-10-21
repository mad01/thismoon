package mods

import (
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
	"github.com/mad01/thismoon/tools/agent-sync/internal/gate"
)

// DefaultDropins is the drop-in directory sync_mods.py always read, so a
// config that names only the mods file reads the same drop-ins.
const DefaultDropins = "~/.config/claude-mods/mods.d"

// The section's keys.
const (
	keyFile         = "file"
	keyDropins      = "dropins"
	keyProfilesFile = "profiles_file"
)

// Config is the mods section, resolved: every path absolute with ~ expanded.
type Config struct {
	// File is the base mods file (mods.json).
	File string
	// Dropins is the directory whose *.json files are read after File, in
	// sorted name order. It need not exist.
	Dropins string
	// ProfilesFile is the TOML file the machine's profile tags come from. It
	// need not exist.
	ProfilesFile string
}

// ParseConfig reads the mods section into a Config. A missing section, a
// missing file key, an unknown key or a relative path is an error naming
// the config line at fault.
func ParseConfig(s config.Section) (Config, error) {
	if !s.Present() {
		return Config{}, s.Errorf(nil, "",
			"section missing; it needs at least %s.%s, the mods file to sync",
			config.SectionMods, keyFile)
	}
	fields, err := s.Fields(keyFile, keyDropins, keyProfilesFile)
	if err != nil {
		return Config{}, err
	}
	node, ok := fields[keyFile]
	if !ok {
		return Config{}, s.Errorf(s.Node(), keyFile,
			"required: the path of the mods file (mods.json) to sync")
	}
	var c Config
	if c.File, err = s.Path(node, keyFile); err != nil {
		return Config{}, err
	}
	if c.Dropins, err = s.PathOr(fields[keyDropins], keyDropins, DefaultDropins); err != nil {
		return Config{}, err
	}
	c.ProfilesFile, err = s.PathOr(fields[keyProfilesFile], keyProfilesFile,
		gate.DefaultProfilesFile)
	if err != nil {
		return Config{}, err
	}
	return c, nil
}
