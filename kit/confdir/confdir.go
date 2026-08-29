// Package confdir resolves the per-component config and state directories.
// It honors the XDG base directories when they are set and falls back to
// the conventional ~/.config and ~/.local/state roots otherwise, so a
// component gets the same answer wherever it asks.
//
// An unresolvable home directory is an error here, never a relative path.
// Components run from launchd agents, git hooks, and sandboxed execs, all
// of which can hand a process an environment without HOME; degrading to a
// relative path there writes ".config/<component>" into whatever the
// working directory happens to be and reads a config nobody wrote.
package confdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Environment variables naming the XDG roots this package honors. A value
// that is not absolute is invalid per the spec and ignored, which is also
// what keeps a stripped environment from producing a relative directory.
const (
	envConfigHome = "XDG_CONFIG_HOME"
	envStateHome  = "XDG_STATE_HOME"
)

// errNoComponent reports a lookup with no component to resolve for.
// Returning the config root itself would hand the caller a directory
// shared with every other tool.
var errNoComponent = errors.New("confdir: empty component name")

// Dir returns the directory holding component's configuration:
// $XDG_CONFIG_HOME/<component> when that variable holds an absolute path,
// otherwise $HOME/.config/<component>.
func Dir(component string) (string, error) {
	return resolve(component, envConfigHome, ".config")
}

// Path returns the path of file inside component's configuration
// directory.
func Path(component, file string) (string, error) {
	dir, err := Dir(component)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, file), nil
}

// StateDir returns the directory holding component's mutable state:
// $XDG_STATE_HOME/<component> when that variable holds an absolute path,
// otherwise $HOME/.local/state/<component>.
//
// legacy names a directory an earlier release kept state in. When it
// exists on disk it wins outright and is returned unchanged, leading ~ and
// all: an install that already has data keeps reading and writing where
// that data is, while a fresh install lands in the XDG location. Pass ""
// when there is nothing to migrate from.
func StateDir(component, legacy string) (string, error) {
	if legacy != "" {
		path, err := Expand(legacy)
		if err != nil {
			return "", err
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return legacy, nil
		}
	}
	return resolve(component, envStateHome, filepath.Join(".local", "state"))
}

// Expand rewrites a leading ~ or ~/ to the user's home directory and
// returns other paths unchanged. It is the shared replacement for the
// per-component expandTilde copies: a path that cannot be expanded is an
// error rather than a silently cwd-relative one.
func Expand(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := home()
	if err != nil {
		return "", fmt.Errorf("confdir: expand %q: %w", path, err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

// resolve joins component onto the root named by envVar, or onto
// fallbackRel under the home directory when that variable is unset or
// relative.
func resolve(component, envVar, fallbackRel string) (string, error) {
	if component == "" {
		return "", errNoComponent
	}
	if root := os.Getenv(envVar); filepath.IsAbs(root) {
		return filepath.Join(root, component), nil
	}
	h, err := home()
	if err != nil {
		return "", fmt.Errorf("confdir: resolve directory for %q: %w", component, err)
	}
	return filepath.Join(h, fallbackRel, component), nil
}

// home returns the user's home directory, reporting an empty one as an
// error: os.UserHomeDir is happy to return "" with no error on some
// platforms, and "" would join into a root-relative path.
func home() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if h == "" {
		return "", errors.New("home directory is empty")
	}
	return h, nil
}
