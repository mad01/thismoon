// Package config loads the prs YAML config: the directories to walk for git
// repos, the repos to exclude, an optional host allowlist, and the poll
// interval. A missing file is not an error — serve runs on defaults and the
// status surfaces that no dirs are configured.
package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/gobwas/glob"
	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/kit/repofind"
)

// DefaultPollInterval is used when the config file is missing or sets no
// poll_interval.
const DefaultPollInterval = 5 * time.Minute

// DefaultExcludes are org/repo globs excluded on every machine unless the
// config's include list opts them back in: the live-test fixture repo full
// of deliberately stale PRs has no place on a real dashboard.
func DefaultExcludes() []string {
	return []string{"mad01/prs-testbed"}
}

// Config is the parsed, validated configuration serve runs with.
type Config struct {
	// Dirs are the roots walked for git repos, ~ already expanded.
	Dirs []string
	// Exclude holds org/repo glob patterns skipped during discovery.
	Exclude []string
	// Include holds org/repo glob patterns that override every exclusion,
	// built-in defaults included — how a test config opts the fixture repo
	// back in.
	Include []string
	// Hosts is an optional allowlist of git hosts; empty polls every host
	// discovered from the repos' origin remotes.
	Hosts []string
	// PollInterval is how often the poller refreshes all repos.
	PollInterval time.Duration
	// Loaded reports whether a config file was read (false = defaults).
	Loaded bool
	// Path is the file the config was read from (or would be read from).
	Path string
}

// fileConfig mirrors the YAML on disk. poll_interval is a Go duration string
// ("5m", "90s").
type fileConfig struct {
	Dirs         []string `yaml:"dirs"`
	Exclude      []string `yaml:"exclude"`
	Include      []string `yaml:"include"`
	Hosts        []string `yaml:"hosts"`
	PollInterval string   `yaml:"poll_interval"`
}

// Load reads the config at path. A missing file returns defaults with
// Loaded=false; a present-but-invalid file is an error, never a silent
// fallback.
func Load(path string) (Config, error) {
	path = repofind.ExpandHome(path)
	cfg := Config{PollInterval: DefaultPollInterval, Path: path}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var fc fileConfig
	if err := yaml.Unmarshal(raw, &fc); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg.Loaded = true
	cfg.Exclude = fc.Exclude
	cfg.Include = fc.Include
	cfg.Hosts = fc.Hosts
	for _, d := range fc.Dirs {
		cfg.Dirs = append(cfg.Dirs, repofind.ExpandHome(d))
	}
	// Validate the globs at the boundary so a typo fails at startup, not
	// silently mid-poll.
	for _, p := range append(append([]string{}, fc.Exclude...), fc.Include...) {
		if _, err := glob.Compile(p, '/'); err != nil {
			return Config{}, fmt.Errorf("parse config %s: pattern %q: %w", path, p, err)
		}
	}
	if fc.PollInterval != "" {
		iv, err := time.ParseDuration(fc.PollInterval)
		if err != nil {
			return Config{}, fmt.Errorf("parse config %s: poll_interval: %w", path, err)
		}
		if iv <= 0 {
			return Config{}, fmt.Errorf(
				"parse config %s: poll_interval must be positive, got %s", path, iv,
			)
		}
		cfg.PollInterval = iv
	}
	return cfg, nil
}

// HostAllowed reports whether host passes the optional allowlist.
func (c Config) HostAllowed(host string) bool {
	return len(c.Hosts) == 0 || slices.Contains(c.Hosts, host)
}
