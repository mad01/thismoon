// Package config reads the optional worklog config file. A missing or
// malformed file yields the zero Config — worklog must work with no config
// present, falling back to built-in defaults at the point of use.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the on-disk shape of the worklog config file.
type Config struct {
	Scan Scan `yaml:"scan"`
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
