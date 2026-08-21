// Package config reads the optional worklog config file. A missing or
// malformed file yields the zero Config — worklog must work with no config
// present, falling back to built-in defaults at the point of use.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Config is the on-disk shape of the worklog config file.
type Config struct {
	Scan   Scan   `yaml:"scan"`
	Remote Remote `yaml:"remote"`
}

// Remote configures a git upstream for the store. With no upstreams the store
// stays a local-only git repo, which is the pre-remote behavior.
type Remote struct {
	// Push toggles auto commit+push after each store write; nil means true.
	// It only matters when an upstream resolves for this machine.
	Push *bool `yaml:"push"`
	// Upstreams maps a machine profile label (from ralph's config.local.toml,
	// e.g. "personal" or "work") to a git remote URL. The machine's first
	// profile with an entry wins, so one fleet-shared config file can send
	// each machine's store to its own private repo.
	Upstreams map[string]string `yaml:"upstreams"`
}

// ResolveUpstream returns the remote URL for a machine with the given profile
// labels: the first profile (in the given order) with an upstream entry.
// Empty when no profile matches or no upstreams are configured.
func (r Remote) ResolveUpstream(profiles []string) string {
	for _, p := range profiles {
		if url := r.Upstreams[p]; url != "" {
			return url
		}
	}
	return ""
}

// PushEnabled reports whether store writes should push to the upstream; an
// unset push key means yes.
func (r Remote) PushEnabled() bool {
	return r.Push == nil || *r.Push
}

// MachineProfiles reads this machine's profile labels from ralph's
// config.local.toml — the same per-machine, gitignored file belt reads.
// $WORKLOG_RALPH_CONFIG overrides the path (tests); a missing or malformed
// file means no profiles, which resolves to no upstream.
func MachineProfiles() []string {
	path := os.Getenv("WORKLOG_RALPH_CONFIG")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".config", "ralph", "config.local.toml")
	}
	var cfg struct {
		Profiles []string `toml:"profiles"`
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil
	}
	return cfg.Profiles
}

// Scan carries the ticket-firewall strings for `worklog scan`. Empty fields
// fall back to the scan package's built-in defaults.
type Scan struct {
	// LinearPrefixes lists the TEAM-NN key prefixes routed to the personal
	// (Linear) world; any other key is treated as internal (Jira).
	LinearPrefixes []string `yaml:"linear_prefixes"`
	// PersonalPathMarkers mark a session cwd as personal when contained in it.
	PersonalPathMarkers []string `yaml:"personal_path_markers"`
	// InternalPathMarkers mark a session cwd as internal when contained in it.
	// GOPATH-style checkouts of non-github.com hosts are internal regardless.
	InternalPathMarkers []string `yaml:"internal_path_markers"`
	// CheckoutRoots are the path fragments under which GOPATH-style checkouts
	// live; the segment directly after a root is read as the git host.
	CheckoutRoots []string `yaml:"checkout_roots"`
	// RepoPathMarkers mark a session cwd as a repo checkout at all; a cwd
	// matching none of them (e.g. a tmp dir) reports no repo.
	RepoPathMarkers []string `yaml:"repo_path_markers"`
}

// Path returns the config file location: $WORKLOG_CONFIG if set, else
// ~/.config/worklog/config.yaml.
func Path() string {
	if p := os.Getenv("WORKLOG_CONFIG"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "worklog", "config.yaml")
}

// Load reads the config file at Path. Missing or unreadable files and YAML
// errors all yield the zero Config.
func Load() Config {
	c, _ := LoadFile()
	return c
}

// LoadFile reads the config file at Path and reports why it could not be used:
// an fs.ErrNotExist-wrapping error when no file is there, a parse error when
// the YAML is malformed. The returned Config is usable either way — whatever
// survived the failure, which for a missing file is the zero value. Callers
// that must explain themselves (`worklog config`) use this; Load discards the
// error because worklog runs on defaults regardless.
func LoadFile() (Config, error) {
	var c Config
	data, err := os.ReadFile(Path())
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("parsing %s: %w", Path(), err)
	}
	return c, nil
}
