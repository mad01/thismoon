package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
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
	// Prices are the [prices."<model id>"] tables, keyed by model id.
	Prices map[string]priceConfig `toml:"prices"`
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

// priceConfig is one [prices."<model id>"] table, in USD per million
// tokens. It sets the rates of that model id, keeping the long-prompt tier
// and the fast-mode and US-only multipliers of the built-in row the id
// matches (trace.NewPrices), or adds a row. Input and output are required;
// a cache rate left out derives from input with the prompt-caching
// multipliers in facts.go.
type priceConfig struct {
	Input        *float64 `toml:"input"`
	Output       *float64 `toml:"output"`
	CacheWrite5m *float64 `toml:"cache_write_5m"`
	CacheWrite1h *float64 `toml:"cache_write_1h"`
	CacheRead    *float64 `toml:"cache_read"`
}

// priceOverrides turns the [prices] tables into rates by model id. A table
// without input or output, or with a rate that is negative, NaN or
// infinite, is an error that names it: serve refuses to start rather than
// price with a half-read table.
func priceOverrides(tables map[string]priceConfig) (map[string]moondb.Rates, error) {
	out := make(map[string]moondb.Rates, len(tables))
	for id, p := range tables {
		r, err := p.rates()
		if err != nil {
			return nil, fmt.Errorf("[prices.%q]: %w", id, err)
		}
		out[id] = r
	}
	return out, nil
}

// rates is the table's rates with the left-out cache rates derived. Every
// rate the table names must be a finite number, not negative: TOML accepts
// nan and inf, and either would turn every cost into a value JSON cannot
// encode. A derived rate is checked through the input it comes from.
func (p priceConfig) rates() (moondb.Rates, error) {
	if p.Input == nil || p.Output == nil {
		return moondb.Rates{}, errors.New("input and output are required")
	}
	r := moondb.RatesFrom(*p.Input, *p.Output, *p.Input*moondb.CacheReadMultiplier)
	for _, set := range []struct {
		key   string
		value *float64
		into  *float64
	}{
		{"input", p.Input, &r.Input},
		{"output", p.Output, &r.Output},
		{"cache_write_5m", p.CacheWrite5m, &r.CacheWrite5m},
		{"cache_write_1h", p.CacheWrite1h, &r.CacheWrite1h},
		{"cache_read", p.CacheRead, &r.CacheRead},
	} {
		switch {
		case set.value == nil:
			continue
		case math.IsNaN(*set.value) || math.IsInf(*set.value, 0):
			return moondb.Rates{}, fmt.Errorf("%s = %v: a rate must be a finite number",
				set.key, *set.value)
		case *set.value < 0:
			return moondb.Rates{}, fmt.Errorf("%s = %v: a rate cannot be negative",
				set.key, *set.value)
		}
		*set.into = *set.value
	}
	return r, nil
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
