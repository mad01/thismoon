package csl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
)

// Component is the name csl resolves its config and state directories
// under.
const Component = "csl"

// DefaultPort is the loopback port `csl web` binds when neither --port nor
// CSL_PORT names another one.
const DefaultPort = 7424

// PortEnv moves the web UI off DefaultPort for every csl process on the
// machine at once. The port matters beyond `csl web`: the MCP server and
// the CLI build links into the UI without being able to see the flag it was
// started with, so a relocated UI has to be announced through the
// environment (or pinned in config as web.base_url).
const PortEnv = "CSL_PORT"

// DefaultBaseURL is the web UI's base URL on DefaultPort, the value
// EffectiveWebBaseURL falls back to when config sets no web.base_url and
// nothing moves the port. A csl.this front is machine-private d-man wiring
// layered over this default.
const DefaultBaseURL = "http://127.0.0.1:7424"

// LegacyStateDir is where csl kept its whole footprint — config file and
// mutable state together — before the two were split. It wins whenever it
// holds LegacyStateProbe, so an install made before the split keeps its
// indexes, socket, and queue exactly where they are: no migration step and
// no re-index.
const LegacyStateDir = "~/.config/" + Component

// LegacyStateProbe is the artifact that proves csl actually kept state in
// LegacyStateDir. The directory alone proves nothing: fleet provisioning
// symlinks config.yaml into it on every machine, so treating its existence
// as evidence would pin even a brand-new machine to the pre-split location.
// The lexical index is the right witness — csl builds it on the first
// search and nothing else creates it.
const LegacyStateProbe = "search-index"

// DaemonLogFile is the search server's rotated log, named here because the
// operating doc points readers at it and package daemon (which writes it)
// imports this package.
const DaemonLogFile = "search-daemon.log"

// BaseURLForPort renders the loopback base URL of a `csl web` listening on
// port. Every base URL csl derives from a port goes through this function,
// so a moved port cannot leave one surface linking to the old one.
func BaseURLForPort(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// ResolvedPort returns the port a csl process assumes the web UI listens on
// when no flag says otherwise: CSL_PORT when set, else DefaultPort.
func ResolvedPort() int {
	return envdefault.Int(PortEnv, DefaultPort)
}

// StateDir returns the absolute directory holding csl's mutable state: the
// lexical and semantic indexes, the search server's socket, PID file and
// log, and the reindex queue. config.yaml is not among them — it lives in
// the config directory, which internal/repo/config resolves.
//
// An install that predates the config/state split keeps everything under
// LegacyStateDir; a fresh one lands under $XDG_STATE_HOME/csl, or
// ~/.local/state/csl when that variable is unset.
func StateDir() (string, error) {
	dir, err := confdir.StateDir(Component, confdir.LegacyDir{
		Dir:   LegacyStateDir,
		Probe: LegacyStateProbe,
	})
	if err != nil {
		return "", err
	}
	return confdir.Expand(dir)
}

// StatePath returns the path of name inside StateDir.
func StatePath(name string) (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// stateDirForDocs renders the resolved state directory the way a reader
// would type it: absolute, with the home directory collapsed back to ~. An
// unresolvable state directory falls back to LegacyStateDir, since a doc
// naming the pre-split location beats one with a blank in it.
func stateDirForDocs() string {
	dir, err := StateDir()
	if err != nil {
		return LegacyStateDir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	rel, err := filepath.Rel(home, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return dir
	}
	return filepath.Join("~", rel)
}
