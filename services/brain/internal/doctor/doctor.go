// Package doctor runs brain's self-checks: the config resolves, the vault
// exists, the index directory is writable, the literal index is fresh and,
// when the semantic layer is on, Ollama answers. `brain doctor` prints the
// report and the brain_doctor MCP tool returns it, so both read the same
// checks in the same shape.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/embedding"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// ollamaProbeTimeout bounds the Ollama health request.
const ollamaProbeTimeout = 3 * time.Second

// Check statuses.
const (
	StatusOK   = "ok"
	StatusFail = "fail"
	StatusSkip = "skip"
)

// Check is one probe and its outcome. Fix names the next step when it failed.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// Report is a doctor run; OK is false when any check failed.
type Report struct {
	OK     bool    `json:"ok"`
	Checks []Check `json:"checks"`
}

// Failed counts the checks that failed.
func (r Report) Failed() int {
	n := 0
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			n++
		}
	}
	return n
}

// Run runs every check against the config at configPath. A check whose
// input an earlier check failed to produce is skipped, not failed, so one
// root cause reads as one failure. Run writes nothing that outlives it.
func Run(ctx context.Context, configPath string) Report {
	cfg, cfgCheck := checkConfig(configPath)
	checks := []Check{cfgCheck}
	if cfg == nil {
		checks = append(checks,
			skipped("vault", "needs a config"),
			skipped("index-dir", "needs a config"),
			skipped("index", "needs a config"),
			skipped("ollama", "needs a config"),
		)
		return newReport(checks)
	}
	vaultCheck := checkVault(cfg)
	dirCheck, indexDir := checkIndexDir()
	checks = append(checks, vaultCheck, dirCheck)
	switch {
	case vaultCheck.Status != StatusOK:
		checks = append(checks, skipped("index", "needs the vault"))
	case dirCheck.Status != StatusOK:
		checks = append(checks, skipped("index", "needs the index directory"))
	default:
		checks = append(checks, checkIndexFresh(cfg, indexDir))
	}
	checks = append(checks, checkOllama(ctx, cfg))
	return newReport(checks)
}

func newReport(checks []Check) Report {
	r := Report{Checks: checks}
	r.OK = r.Failed() == 0
	return r
}

func skipped(name, why string) Check {
	return Check{Name: name, Status: StatusSkip, Detail: why}
}

func checkConfig(path string) (*config.Config, Check) {
	cfg, err := config.Load(path)
	if err == nil {
		return cfg, Check{Name: "config", Status: StatusOK, Detail: path}
	}
	fix := "make " + path + " readable"
	switch {
	case errors.Is(err, fs.ErrNotExist):
		fix = "create " + path + " containing a line `vault: <path to your vault>`, or point --config or " +
			brain.ConfigEnv + " at an existing config"
	case errors.Is(err, config.ErrInvalid):
		fix = "fix " + path + ": a YAML mapping with at least `vault: <path to your vault>`"
	}
	return nil, Check{Name: "config", Status: StatusFail, Detail: err.Error(), Fix: fix}
}

func checkVault(cfg *config.Config) Check {
	if _, err := vault.Open(cfg.Vault); err != nil {
		return Check{
			Name:   "vault",
			Status: StatusFail,
			Detail: err.Error(),
			Fix:    "create the directory or point vault: in the config at an existing one",
		}
	}
	return Check{Name: "vault", Status: StatusOK, Detail: cfg.Vault}
}

// checkIndexDir proves brain can write its literal index, without creating
// the directory when it does not exist yet.
func checkIndexDir() (Check, string) {
	dir, err := brain.IndexDir()
	if err != nil {
		return Check{
			Name:   "index-dir",
			Status: StatusFail,
			Detail: err.Error(),
			Fix:    "set HOME, or XDG_STATE_HOME to an absolute path",
		}, ""
	}
	probed, err := probeWritable(dir)
	if err != nil {
		return Check{
			Name:   "index-dir",
			Status: StatusFail,
			Detail: err.Error(),
			Fix:    "make " + probed + " writable",
		}, dir
	}
	detail := dir + " is writable"
	if probed != dir {
		detail = dir + " does not exist yet; brain index creates it under " + probed + ", which is writable"
	}
	return Check{Name: "index-dir", Status: StatusOK, Detail: detail}, dir
}

// probeWritable creates and removes a file in dir, or in its nearest
// existing ancestor when dir does not exist yet, and returns the directory
// it probed. It creates no directory and leaves no file behind.
func probeWritable(dir string) (string, error) {
	target, err := nearestExisting(dir)
	if err != nil {
		return target, err
	}
	f, err := os.CreateTemp(target, ".brain-doctor-*")
	if err != nil {
		return target, err
	}
	name := f.Name()
	return target, errors.Join(f.Close(), os.Remove(name))
}

func nearestExisting(dir string) (string, error) {
	for {
		info, err := os.Stat(dir)
		switch {
		case err == nil && info.IsDir():
			return dir, nil
		case err == nil:
			return dir, fmt.Errorf("%s is not a directory", dir)
		case !errors.Is(err, fs.ErrNotExist):
			return dir, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir, err
		}
		dir = parent
	}
}

func checkIndexFresh(cfg *config.Config, indexDir string) Check {
	fingerprint, _, err := search.ComputeFingerprint(cfg.Vault)
	if err != nil {
		return Check{Name: "index", Status: StatusFail, Detail: err.Error()}
	}
	state, err := search.LoadState(indexDir)
	if err != nil {
		return Check{
			Name:   "index",
			Status: StatusFail,
			Detail: err.Error(),
			Fix:    "brain index --force",
		}
	}
	if state.Fingerprint == "" {
		return Check{Name: "index", Status: StatusFail, Detail: "never built", Fix: "brain index"}
	}
	if search.IsStale(state, fingerprint) {
		return Check{
			Name:   "index",
			Status: StatusFail,
			Detail: "stale: the vault changed since " + state.IndexedAt.Format(time.RFC3339),
			Fix:    "brain index",
		}
	}
	return Check{
		Name:   "index",
		Status: StatusOK,
		Detail: "fresh, built " + state.IndexedAt.Format(time.RFC3339),
	}
}

func checkOllama(ctx context.Context, cfg *config.Config) Check {
	if !cfg.Semantic.Enabled {
		return skipped("ollama", "semantic.enabled is false")
	}
	ctx, cancel := context.WithTimeout(ctx, ollamaProbeTimeout)
	defer cancel()
	if err := embedding.New(cfg.Semantic.OllamaURL, cfg.Semantic.EmbedModel).Health(ctx); err != nil {
		return Check{
			Name:   "ollama",
			Status: StatusFail,
			Detail: err.Error(),
			Fix:    "start Ollama (ollama serve) or set semantic.enabled: false",
		}
	}
	return Check{Name: "ollama", Status: StatusOK, Detail: cfg.Semantic.OllamaURL + " answers"}
}
