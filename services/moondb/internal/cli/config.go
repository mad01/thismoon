package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/spf13/pflag"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/services/moondb"
)

// configFile is the optional per-machine config, read from the moondb
// config directory (confdir: ~/.config/moondb by default) unless --config
// or MOONDB_CONFIG relocates it (ADR-0011).
const configFile = "config.toml"

// config is the file's shape. Flags and env override every key here.
type config struct {
	Port    int           `toml:"port"`
	Workdir string        `toml:"workdir"`
	BaseURL string        `toml:"base_url"`
	Capture captureConfig `toml:"capture"`
}

// captureConfig is the [capture] table. Enabled nil means on. Exclude, when
// set, replaces the compiled-in default globs.
type captureConfig struct {
	Enabled *bool    `toml:"enabled"`
	Exclude []string `toml:"exclude"`
}

// enabled reports whether the hooks should record anything on this machine.
func (c captureConfig) enabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// excludes is the exclude list in force: the file's when it names one, else
// the defaults.
func (c captureConfig) excludes() []string {
	if len(c.Exclude) > 0 {
		return c.Exclude
	}
	return moondb.DefaultExcludes
}

// The config every command sees, loaded once by the root pre-run. cfgErr is
// why it could not be loaded; a config that is present but broken fails
// closed (ADR-0011), and each command decides how loudly.
var (
	cfg    config
	cfgErr error
)

// defaultConfigPath is where the config lives when nothing relocates it; ""
// when the home directory cannot be resolved, which loadConfig reads as no
// file.
func defaultConfigPath() string {
	path, err := confdir.Path("moondb", configFile)
	if err != nil {
		return ""
	}
	return path
}

// loadConfig reads the config file at path. A missing file is the zero
// config; a file that will not parse is an error.
func loadConfig(path string) (config, error) {
	if path == "" {
		return config{}, nil
	}
	var c config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return config{}, nil
		}
		return config{}, fmt.Errorf("read %s: %w", path, err)
	}
	return c, nil
}

// applyConfig lets the file's values stand in for defaults: a key counts
// only when its flag was not given and its env var is unset, which keeps
// the precedence flag, env, file, default.
func applyConfig(flags *pflag.FlagSet, c config) {
	if c.Port != 0 && fromConfig(flags, "port", "MOONDB_PORT") {
		flagPort = c.Port
	}
	if c.Workdir != "" && fromConfig(flags, "workdir", "MOONDB_WORKDIR") {
		flagWorkdir = c.Workdir
	}
	if c.BaseURL != "" && fromConfig(flags, "base-url", "MOONDB_BASE_URL") {
		flagBaseURL = c.BaseURL
	}
}

// fromConfig reports whether the config file decides the named setting.
func fromConfig(flags *pflag.FlagSet, name, env string) bool {
	return !flags.Changed(name) && os.Getenv(env) == ""
}

// requireConfig is the fail-closed gate every command but record runs: a
// broken config stops the command with the path to fix.
func requireConfig() error {
	if cfgErr != nil {
		return fmt.Errorf("config: %w (fix or remove it; moondb doctor shows the path)", cfgErr)
	}
	return nil
}
