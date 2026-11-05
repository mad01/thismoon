package brain

import (
	"os"
	"path/filepath"

	"github.com/mad01/thismoon/kit/confdir"
)

// DefaultStateDir is where a fresh install keeps its indexes when
// XDG_STATE_HOME is unset. The leading ~ is expanded at runtime.
const DefaultStateDir = "~/.local/state/" + Component

// LegacyStateDir is where brain kept its indexes beside config.yaml before
// config and state were split. It stays the state directory while it holds
// LegacyStateProbe, so an existing install keeps its indexes and re-embeds
// nothing.
const LegacyStateDir = "~/.config/" + Component

// LegacyStateProbe proves brain kept state in LegacyStateDir. The directory
// alone proves nothing, because provisioning links config.yaml into it on
// every machine. The literal index's state file is written only when a
// brain index build completes, so nothing else creates it.
const LegacyStateProbe = "index/state.json"

// envStateHome is the XDG variable that roots the state directory.
const envStateHome = "XDG_STATE_HOME"

// State subdirectories.
const (
	indexSubdir    = "index"
	semanticSubdir = "semantic"
)

// DefaultConfigPath returns config.yaml inside brain's config directory:
// $XDG_CONFIG_HOME/brain when that variable is absolute, else ~/.config/brain.
func DefaultConfigPath() (string, error) {
	return confdir.Path(Component, ConfigFile)
}

// StateDir returns the absolute directory holding brain's indexes:
// LegacyStateDir when it holds LegacyStateProbe, otherwise
// $XDG_STATE_HOME/brain, or ~/.local/state/brain when that is unset.
//
// An absolute XDG_STATE_HOME skips the legacy probe. Setting it says where
// state goes, which is how an eval or a test isolates brain from the
// machine's real indexes; the indexes are rebuilt from the vault, so a
// moved state directory costs a rebuild and loses nothing.
func StateDir() (string, error) {
	legacy := confdir.LegacyDir{Dir: LegacyStateDir, Probe: LegacyStateProbe}
	if filepath.IsAbs(os.Getenv(envStateHome)) {
		legacy = confdir.LegacyDir{}
	}
	dir, err := confdir.StateDir(Component, legacy)
	if err != nil {
		return "", err
	}
	return confdir.Expand(dir)
}

// IndexDir returns the literal (zoekt) index directory inside StateDir. It
// also holds state.json, the fingerprints of both indexes.
func IndexDir() (string, error) {
	return stateSubdir(indexSubdir)
}

// SemanticDir returns the default semantic (chromem-go) index directory
// inside StateDir. semantic.index_dir in the config overrides it.
func SemanticDir() (string, error) {
	return stateSubdir(semanticSubdir)
}

func stateSubdir(name string) (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}
