// Package sysopen wraps the macOS open command: URLs to their default
// handler, files to their default (or a named) application, apps by name,
// and Finder reveal. The CLI and the MCP server are two thin frontends over
// this package, so a tool call and a manual run always exec open the same
// way.
package sysopen

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/tools/opener"
)

// openBin is absolute: MCP hosts and launchd spawn processes with a minimal
// PATH, so resolving via PATH is the failure mode, not the default.
const openBin = "/usr/bin/open"

// Opener execs the open command. Construct with New.
type Opener struct {
	// run executes open with args. A field so tests can swap it without
	// launching apps or browser tabs.
	run func(args ...string) error
}

// New returns an Opener backed by the real /usr/bin/open.
func New() *Opener { return &Opener{run: run} }

// URL opens raw with the default handler for its scheme — the browser for
// http/https. It returns the URL it opened.
func (o *Opener) URL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("sysopen: not a URL %q: %w", raw, err)
	}
	if u.Scheme == "" {
		return "", fmt.Errorf(
			"sysopen: URL %q has no scheme; prefix it (https://...), or open a local path as a file",
			raw,
		)
	}
	return raw, hint(o.run(raw))
}

// File opens path with its default application. It returns the absolute
// path it opened.
func (o *Opener) File(path string) (string, error) {
	p, err := resolvePath(path)
	if err != nil {
		return "", err
	}
	return p, hint(o.run(p))
}

// App launches (or foregrounds) the application named name, as `open -a`.
func (o *Opener) App(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("sysopen: app name is empty")
	}
	return name, hint(o.run("-a", name))
}

// With opens path with the application named app instead of its default.
func (o *Opener) With(path, app string) (string, error) {
	if app == "" {
		return "", fmt.Errorf("sysopen: app name is empty")
	}
	p, err := resolvePath(path)
	if err != nil {
		return "", err
	}
	return p, hint(o.run("-a", app, p))
}

// Reveal shows path in a Finder window, selected, as `open -R`.
func (o *Opener) Reveal(path string) (string, error) {
	p, err := resolvePath(path)
	if err != nil {
		return "", err
	}
	return p, hint(o.run("-R", p))
}

// resolvePath expands a leading ~, requires the result to be absolute (the
// MCP server process runs from /, so a relative path never means what the
// caller intended), and confirms it exists — open(1) reports a missing file
// with a GUI-flavored error, so a stat beats it to a clear one.
func resolvePath(path string) (string, error) {
	p := expandTilde(path)
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf(
			"sysopen: path %q is not absolute; the server runs from /, pass a full or ~-prefixed path",
			path,
		)
	}
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("sysopen: %w", err)
	}
	return p, nil
}

// expandTilde rewrites a leading ~ or ~/ to the user's home directory. When
// the home directory cannot be determined the path is returned unchanged
// and fails the absolute-path check with the original input named.
func expandTilde(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

// hint points an error's reader at opener's self-diagnosis surface. Every
// exec result crosses here — the one chokepoint the CLI and MCP share.
func hint(err error) error { return agentdoc.Hint(err, opener.Facts()) }

// run executes /usr/bin/open with args, riding stderr on the error so
// failures like "Unable to find application" surface verbatim.
func run(args ...string) error {
	cmd := exec.Command(openBin, args...)
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errOut.String()); msg != "" {
			return fmt.Errorf("sysopen: %s %s: %s: %w", openBin, strings.Join(args, " "), msg, err)
		}
		return fmt.Errorf("sysopen: %s %s: %w", openBin, strings.Join(args, " "), err)
	}
	return nil
}
