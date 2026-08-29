// Package config reads the optional worklog config file. A file that is not
// there is fine — worklog runs on built-in defaults — but a file that exists
// and cannot be read or parsed is an error: it carries this machine's
// ticket-firewall strings and the store's push remote, and dropping those
// without a word looks exactly like working software.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/kit/confdir"
)

// Component is the name worklog's config directory is keyed by.
const Component = "worklog"

// FileName is the config file inside that directory.
const FileName = "config.yaml"

// Config is the on-disk shape of the worklog config file.
type Config struct {
	Scan   Scan   `yaml:"scan"`
	Remote Remote `yaml:"remote"`
}

// Remote configures a git upstream for the store. The zero value keeps the
// store a local-only git repo, which is the pre-remote behavior.
type Remote struct {
	// URL is the git remote origin points at. Empty means local-only.
	URL string `yaml:"url,omitempty"`
	// Push toggles auto commit+push after each store write; nil means true.
	// It only matters once URL is set.
	Push *bool `yaml:"push,omitempty"`
	// Upstreams is the retired profile-keyed form of URL, still parsed so a
	// config carrying it can be told it is no longer read. Which machine gets
	// which upstream is a provisioning concern (docs/adr/0010): the layer that
	// installs this file writes the URL for the machine it installs on.
	Upstreams map[string]string `yaml:"upstreams,omitempty"`
}

// PushEnabled reports whether store writes should push to the upstream; an
// unset push key means yes.
func (r Remote) PushEnabled() bool {
	return r.Push == nil || *r.Push
}

// RetiredUpstreams reports a config that still keys its upstream by machine
// profile and so resolves to no remote at all. Callers say so out loud rather
// than let the store go quietly local-only.
func (r Remote) RetiredUpstreams() bool {
	return r.URL == "" && len(r.Upstreams) > 0
}

// Scan carries the ticket-firewall strings for `worklog scan`. Empty fields
// fall back to the scan package's built-in defaults where it has any.
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

// Path returns the config file location: override when it is non-empty (the
// --config flag or $WORKLOG_CONFIG), otherwise config.yaml in worklog's
// directory under $XDG_CONFIG_HOME or ~/.config. A home directory that cannot
// be resolved is an error, never a path relative to the working directory.
func Path(override string) (string, error) {
	if override != "" {
		return confdir.Expand(override)
	}
	return confdir.Path(Component, FileName)
}

// Load reads the config file at path. A missing file yields the zero Config
// and no error, since every key is optional; anything else that goes wrong is
// returned, because a config that exists was written to be used.
func Load(path string) (Config, error) {
	var c Config
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c, nil
		}
		return c, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("parsing %s: %w", path, err)
	}
	return c, nil
}

// Exists reports whether a config file is present at path. `worklog config`
// uses it to tell "no file, defaults in use" from "loaded".
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
