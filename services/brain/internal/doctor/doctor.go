// Package doctor runs brain's self-checks: the config resolves, every vault
// exists, the index directory is writable, each vault's literal index is
// fresh, no two notes share an id and, when the semantic layer is on, Ollama
// answers. `brain doctor` prints the
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
	"strings"
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
			skipped("ids", "needs a config"),
			skipped("ollama", "needs a config"),
		)
		return newReport(checks)
	}
	vaultCheck := checkVaults(cfg)
	dirCheck := checkIndexDir()
	checks = append(checks, vaultCheck, dirCheck)
	switch {
	case vaultCheck.Status != StatusOK:
		checks = append(checks, skipped("index", "needs every vault"))
	case dirCheck.Status != StatusOK:
		checks = append(checks, skipped("index", "needs the index directory"))
	default:
		checks = append(checks, checkIndexFresh(cfg))
	}
	if vaultCheck.Status == StatusOK {
		checks = append(checks, checkIDs(cfg))
	} else {
		checks = append(checks, skipped("ids", "needs every vault"))
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

// checkVaults opens every configured vault. The detail names each vault,
// marking the default and the shared ones.
func checkVaults(cfg *config.Config) Check {
	var ok, failed []string
	for _, v := range cfg.Vaults {
		if _, err := vault.Open(v.Path); err != nil {
			failed = append(failed, v.Name+": "+err.Error())
			continue
		}
		ok = append(ok, v.Name+describeVault(v)+": "+v.Path)
	}
	if len(failed) > 0 {
		return Check{
			Name:   "vault",
			Status: StatusFail,
			Detail: strings.Join(failed, "; "),
			Fix:    "create the directory, or point the vault's path in the config at an existing one",
		}
	}
	return Check{Name: "vault", Status: StatusOK, Detail: strings.Join(ok, "; ")}
}

func describeVault(v config.Vault) string {
	switch {
	case v.Default:
		return " (default)"
	case v.Shared:
		return " (shared)"
	}
	return ""
}

// checkIndexDir proves brain can write its literal indexes, without
// creating the directory when it does not exist yet.
func checkIndexDir() Check {
	dir, err := brain.IndexRoot()
	if err != nil {
		return Check{
			Name:   "index-dir",
			Status: StatusFail,
			Detail: err.Error(),
			Fix:    "set HOME, or XDG_STATE_HOME to an absolute path",
		}
	}
	probed, err := probeWritable(dir)
	if err != nil {
		return Check{
			Name:   "index-dir",
			Status: StatusFail,
			Detail: err.Error(),
			Fix:    "make " + probed + " writable",
		}
	}
	detail := dir + " is writable"
	if probed != dir {
		detail = dir + " does not exist yet; brain index creates it under " + probed + ", which is writable"
	}
	return Check{Name: "index-dir", Status: StatusOK, Detail: detail}
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

// checkIndexFresh checks each vault's literal index against its vault. It
// fails when any index is missing or stale, naming the vaults.
func checkIndexFresh(cfg *config.Config) Check {
	var fresh, failed []string
	for _, v := range cfg.Vaults {
		status, ok := indexStatus(v)
		if ok {
			fresh = append(fresh, v.Name+": "+status)
		} else {
			failed = append(failed, v.Name+": "+status)
		}
	}
	if len(failed) > 0 {
		return Check{
			Name:   "index",
			Status: StatusFail,
			Detail: strings.Join(failed, "; "),
			Fix:    "brain index",
		}
	}
	return Check{Name: "index", Status: StatusOK, Detail: strings.Join(fresh, "; ")}
}

// checkIDs fails when two notes across the configured vaults share an id,
// naming them. Notes without an id are counted, not failed.
func checkIDs(cfg *config.Config) Check {
	vaults := make([]*vault.Vault, 0, len(cfg.Vaults))
	for _, c := range cfg.Vaults {
		v, err := vault.Open(c.Path)
		if err != nil {
			return Check{Name: "ids", Status: StatusFail, Detail: c.Name + ": " + err.Error()}
		}
		v.Name = c.Name
		vaults = append(vaults, v)
	}
	report, err := vault.NewSet(vaults...).IDs()
	if err != nil {
		return Check{Name: "ids", Status: StatusFail, Detail: err.Error()}
	}
	if dups := report.DuplicateIDs(); len(dups) > 0 {
		lines := make([]string, 0, len(dups))
		for _, id := range dups {
			refs := make([]string, 0, len(report.Duplicates[id]))
			for _, r := range report.Duplicates[id] {
				refs = append(refs, r.String())
			}
			lines = append(lines, id+" is on "+strings.Join(refs, ", "))
		}
		return Check{
			Name:   "ids",
			Status: StatusFail,
			Detail: strings.Join(lines, "; "),
			Fix:    "keep each id on one note: delete the id line from the copies, then run brain migrate --ids",
		}
	}
	detail := fmt.Sprintf("%d note(s), no id on two notes", report.Notes)
	if report.Missing > 0 {
		detail += fmt.Sprintf(
			"; %d without an id (add them with: brain migrate --ids)",
			report.Missing,
		)
	}
	return Check{Name: "ids", Status: StatusOK, Detail: detail}
}

// indexStatus describes one vault's literal index, and whether it is fresh.
func indexStatus(v config.Vault) (string, bool) {
	dir, err := brain.IndexDir(v.Name)
	if err != nil {
		return err.Error(), false
	}
	fingerprint, _, err := search.ComputeFingerprint(v.Path)
	if err != nil {
		return err.Error(), false
	}
	state, err := search.LoadState(dir)
	if err != nil {
		return err.Error() + " (rebuild with: brain index --force)", false
	}
	switch {
	case state.Fingerprint == "":
		return "never built", false
	case search.IsStale(state, fingerprint):
		return "stale: the vault changed since " + state.IndexedAt.Format(time.RFC3339), false
	}
	return "fresh, built " + state.IndexedAt.Format(time.RFC3339), true
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
