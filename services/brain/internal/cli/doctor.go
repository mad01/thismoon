package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/embedding"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// ollamaProbeTimeout bounds the doctor's Ollama health request.
const ollamaProbeTimeout = 3 * time.Second

// Doctor check statuses.
const (
	statusOK   = "ok"
	statusFail = "fail"
	statusSkip = "skip"
)

// check is one doctor probe and its outcome.
type check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// doctorReport is the doctor's result; OK is false when any check failed.
type doctorReport struct {
	OK     bool    `json:"ok"`
	Checks []check `json:"checks"`
}

func newDoctorCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check config, vault, index and Ollama",
		Long: `Check that brain can work: the config resolves, the vault exists, the index
directory is writable, the literal index is fresh, and (only when
semantic.enabled is true) Ollama answers. Prints one line per check with the
fix for any failure. Exits 0 when every check passes, 1 otherwise.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			report := runDoctor(cmd.Context())
			err := a.emit(cmd.OutOrStdout(), report, func(w io.Writer) error {
				writeDoctorText(w, report)
				return nil
			})
			if err != nil {
				return err
			}
			if !report.OK {
				return &reportedError{err: fmt.Errorf("%d check(s) failed", report.failed())}
			}
			return nil
		},
	}
}

func (r doctorReport) failed() int {
	n := 0
	for _, c := range r.Checks {
		if c.Status == statusFail {
			n++
		}
	}
	return n
}

func writeDoctorText(w io.Writer, r doctorReport) {
	for _, c := range r.Checks {
		fmt.Fprintf(w, "%-4s  %-10s %s", c.Status, c.Name, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(w, " (fix: %s)", c.Fix)
		}
		fmt.Fprintln(w)
	}
}

// runDoctor runs every check in order. A check whose input an earlier check
// failed to produce is skipped, not failed, so one root cause reads as one
// failure.
func runDoctor(ctx context.Context) doctorReport {
	cfg, cfgCheck := checkConfig()
	checks := []check{cfgCheck}
	if cfg == nil {
		checks = append(checks,
			skipped("vault", "needs a config"),
			skipped("index-dir", "needs a config"),
			skipped("index", "needs a config"),
			skipped("ollama", "needs a config"),
		)
		return newDoctorReport(checks)
	}
	vaultCheck := checkVault(cfg)
	dirCheck, indexDir := checkIndexDir()
	checks = append(checks, vaultCheck, dirCheck)
	switch {
	case vaultCheck.Status != statusOK:
		checks = append(checks, skipped("index", "needs the vault"))
	case dirCheck.Status != statusOK:
		checks = append(checks, skipped("index", "needs the index directory"))
	default:
		checks = append(checks, checkIndexFresh(cfg, indexDir))
	}
	checks = append(checks, checkOllama(ctx, cfg))
	return newDoctorReport(checks)
}

func newDoctorReport(checks []check) doctorReport {
	r := doctorReport{Checks: checks}
	r.OK = r.failed() == 0
	return r
}

func skipped(name, why string) check {
	return check{Name: name, Status: statusSkip, Detail: why}
}

func checkConfig() (*config.Config, check) {
	path, err := config.DefaultPath()
	if err != nil {
		return nil, check{Name: "config", Status: statusFail, Detail: err.Error(), Fix: "set HOME"}
	}
	cfg, err := config.LoadFrom(path)
	if err != nil {
		return nil, check{
			Name:   "config",
			Status: statusFail,
			Detail: err.Error(),
			Fix:    "create " + path + " containing a line `vault: <path to your vault>`",
		}
	}
	return cfg, check{Name: "config", Status: statusOK, Detail: path}
}

func checkVault(cfg *config.Config) check {
	if _, err := vault.Open(cfg.Vault); err != nil {
		return check{
			Name:   "vault",
			Status: statusFail,
			Detail: err.Error(),
			Fix:    "create the directory or point vault: in the config at an existing one",
		}
	}
	return check{Name: "vault", Status: statusOK, Detail: cfg.Vault}
}

// checkIndexDir creates the index directory if needed and proves it is
// writable by creating and removing a probe file.
func checkIndexDir() (check, string) {
	dir, err := search.DefaultIndexDir()
	if err == nil {
		err = probeWritable(dir)
	}
	if err != nil {
		return check{
			Name:   "index-dir",
			Status: statusFail,
			Detail: err.Error(),
			Fix:    "make " + dir + " writable",
		}, dir
	}
	return check{Name: "index-dir", Status: statusOK, Detail: dir + " is writable"}, dir
}

func probeWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".doctor-*")
	if err != nil {
		return err
	}
	name := f.Name()
	closeErr := f.Close()
	removeErr := os.Remove(name)
	return errors.Join(closeErr, removeErr)
}

func checkIndexFresh(cfg *config.Config, indexDir string) check {
	fingerprint, _, err := search.ComputeFingerprint(cfg.Vault)
	if err != nil {
		return check{Name: "index", Status: statusFail, Detail: err.Error()}
	}
	state, err := search.LoadState(indexDir)
	if err != nil {
		return check{
			Name:   "index",
			Status: statusFail,
			Detail: err.Error(),
			Fix:    "brain index --force",
		}
	}
	if state.Fingerprint == "" {
		return check{Name: "index", Status: statusFail, Detail: "never built", Fix: "brain index"}
	}
	if search.IsStale(state, fingerprint) {
		return check{
			Name:   "index",
			Status: statusFail,
			Detail: "stale: the vault changed since " + state.IndexedAt.Format(time.RFC3339),
			Fix:    "brain index",
		}
	}
	return check{
		Name:   "index",
		Status: statusOK,
		Detail: "fresh, built " + state.IndexedAt.Format(time.RFC3339),
	}
}

func checkOllama(ctx context.Context, cfg *config.Config) check {
	if !cfg.Semantic.Enabled {
		return skipped("ollama", "semantic.enabled is false")
	}
	ctx, cancel := context.WithTimeout(ctx, ollamaProbeTimeout)
	defer cancel()
	if err := embedding.New(cfg.Semantic.OllamaURL, cfg.Semantic.EmbedModel).Health(ctx); err != nil {
		return check{
			Name:   "ollama",
			Status: statusFail,
			Detail: err.Error(),
			Fix:    "start Ollama (ollama serve) or set semantic.enabled: false",
		}
	}
	return check{Name: "ollama", Status: statusOK, Detail: cfg.Semantic.OllamaURL + " answers"}
}
