package mcp

import (
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
	"github.com/mad01/thismoon/tools/agent-sync/internal/gate"
)

// Defaults for the paths the config may leave out. They are the paths
// sync_mcp.py hardcoded, so a config that names only the servers file reads
// the same drop-ins and profiles, and keeps the same stamp, as the script.
const (
	// DefaultDropins holds drop-in servers files merged over the base.
	DefaultDropins = "~/.config/claude-mcp/servers.d"
	// DefaultStamp records the hash of the desired set after the last
	// sync that finished.
	DefaultStamp = "~/.cache/claude-mcp/sync.hash"
)

// The section's keys.
const (
	keyServers      = "servers"
	keyDropins      = "dropins"
	keyProfilesFile = "profiles_file"
	keyStamp        = "stamp"
	keyPrune        = "prune"
)

// Config is the mcp section, resolved: every path absolute with ~ expanded.
type Config struct {
	// Servers is the base servers.json: {"servers": {name: spec}}.
	Servers string
	// Dropins holds *.json files of the same shape, merged over Servers
	// in name order. It need not exist.
	Dropins string
	// ProfilesFile is the TOML file the machine's profile tags come from.
	// It need not exist.
	ProfilesFile string
	// Stamp is where sync records the desired set's hash.
	Stamp string
	// Prune removes registered servers the desired set does not name.
	Prune bool
}

// Options are the per-run flags, as opposed to the config file's settings.
type Options struct {
	// Force re-checks every registered server's spec in sync and check,
	// even when the stamp says the desired set has not changed.
	Force bool
}

// ParseConfig reads the mcp section into a Config. A missing section, a
// missing servers key, an unknown key, a relative path or a prune value that
// is not a bool is an error naming the config line at fault.
func ParseConfig(s config.Section) (Config, error) {
	if !s.Present() {
		return Config{}, s.Errorf(nil, "",
			"section missing; it needs at least %s.%s, the servers.json to register",
			config.SectionMCP, keyServers)
	}
	fields, err := s.Fields(keyServers, keyDropins, keyProfilesFile, keyStamp, keyPrune)
	if err != nil {
		return Config{}, err
	}
	node, ok := fields[keyServers]
	if !ok {
		return Config{}, s.Errorf(s.Node(), keyServers,
			"required: the path of the servers.json to register")
	}
	var c Config
	if c.Servers, err = s.Path(node, keyServers); err != nil {
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
	if c.Stamp, err = s.PathOr(fields[keyStamp], keyStamp, DefaultStamp); err != nil {
		return Config{}, err
	}
	if node, ok := fields[keyPrune]; ok {
		if c.Prune, err = s.Bool(node, keyPrune); err != nil {
			return Config{}, err
		}
	}
	return c, nil
}
