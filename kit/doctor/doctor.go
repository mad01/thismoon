// Package doctor runs a component's executable self-checks: the shared
// diagnostics behind `<bin> doctor` for serve-bearing components
// (ADR-0009). Checks beat prose because they rot visibly; the ones here
// cover the near-identical trio every backing service needs — service
// reachable, store readable, and the version-skew probe that catches "new
// binary on disk, old process serving".
package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc"
)

// Check is one named diagnostic. Run returns nil when the check passes.
type Check struct {
	Name string
	Run  func(ctx context.Context) error
}

// skip reports a check that passed vacuously because it had nothing to
// examine. Run prints it as "ok" with the note instead of a failure.
type skip struct{ note string }

func (s skip) Error() string { return s.note }

// Skip returns the error a Check.Run returns when it passed vacuously and
// wants to say why: Collect records it as StatusSkipped with note as the
// detail, and Run prints "ok <name> (<note>)".
//
// It is for the states that are neither a pass worth nothing nor a failure —
// a component that has not been configured yet, a probe with nothing to
// compare against. Without it a component's own check can only pass silently
// or fail, which pushes "nothing is wrong, but here is what you would do
// next" into a channel outside the report.
func Skip(note string) error { return skip{note} }

// probeTimeout bounds each HTTP probe: a diagnostic that hangs is worse
// than one that fails.
const probeTimeout = 3 * time.Second

// Check outcomes, as they serialize in a Report.
const (
	StatusOK      = "ok"      // the check passed
	StatusSkipped = "skipped" // the check had nothing to examine; counts as a pass
	StatusFail    = "fail"    // the check failed; Detail carries the cause
)

// Result is one check's outcome.
type Result struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Report is a doctor run in machine-readable form, for the MCP doctor tool
// an agent reaches for when it has no shell to run `<bin> doctor` in.
type Report struct {
	OK     bool     `json:"ok"`
	Checks []Result `json:"checks"`
}

// Collect runs checks in order and returns their outcomes. A skipped check
// counts as a pass, the same way Run prints it.
func Collect(ctx context.Context, checks []Check) Report {
	report := Report{OK: true, Checks: make([]Result, 0, len(checks))}
	for _, c := range checks {
		err := c.Run(ctx)
		var s skip
		switch {
		case err == nil:
			report.Checks = append(report.Checks, Result{Name: c.Name, Status: StatusOK})
		case errors.As(err, &s):
			report.Checks = append(report.Checks,
				Result{Name: c.Name, Status: StatusSkipped, Detail: s.note})
		default:
			report.OK = false
			report.Checks = append(report.Checks,
				Result{Name: c.Name, Status: StatusFail, Detail: err.Error()})
		}
	}
	return report
}

// failed counts the checks that did not pass.
func (r Report) failed() int {
	n := 0
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			n++
		}
	}
	return n
}

// Run executes checks in order, printing one line per check to w
// ("ok <name>" / "FAIL <name>: <err>"), and returns an error when any
// check failed. On a clean pass the last line points at the operating doc.
func Run(ctx context.Context, w io.Writer, f agentdoc.Facts, checks []Check) error {
	report := Collect(ctx, checks)
	for _, c := range report.Checks {
		switch c.Status {
		case StatusOK:
			fmt.Fprintf(w, "ok %s\n", c.Name)
		case StatusSkipped:
			fmt.Fprintf(w, "ok %s (%s)\n", c.Name, c.Detail)
		default:
			fmt.Fprintf(w, "FAIL %s: %s\n", c.Name, c.Detail)
		}
	}
	if !report.OK {
		return fmt.Errorf("doctor: %d of %d checks failed", report.failed(), len(checks))
	}
	fmt.Fprintf(w, "all checks passed; for semantics and gotchas run '%s docs'\n", f.Bin)
	return nil
}

// ServiceReachable returns a Check that GETs baseURL+"/version" with a
// short timeout and expects 200.
func ServiceReachable(baseURL string) Check {
	return Check{
		Name: "service-reachable",
		Run: func(ctx context.Context) error {
			status, _, err := getVersion(ctx, baseURL)
			if err != nil {
				return fmt.Errorf("service not reachable at %s: %w", baseURL, err)
			}
			if status != http.StatusOK {
				return fmt.Errorf("GET %s/version returned %d, want 200", baseURL, status)
			}
			return nil
		},
	}
}

// StoreReadable returns a Check that the store path (after ~ expansion)
// exists and is readable. An empty path passes with a "skipped" note: the
// component has no store to examine.
func StoreReadable(path string) Check {
	return Check{
		Name: "store-readable",
		Run: func(_ context.Context) error {
			if path == "" {
				return skip{"skipped: no store path configured"}
			}
			fh, err := os.Open(expandTilde(path))
			if err != nil {
				return fmt.Errorf("store not readable: %w", err)
			}
			return fh.Close()
		},
	}
}

// VersionSkew returns a Check comparing the running service's /version
// commit against this binary's own build commit. A mismatch is the "new
// binary on disk, old process serving" failure and fails with the restart
// instruction; an unknown commit on either side passes with a note, since
// a dev build must never produce a false skew.
func VersionSkew(baseURL string) Check {
	return versionSkew(baseURL, buildinfo.Get().Commit)
}

// versionSkew is VersionSkew with the local commit injected, so tests can
// pin both sides of the comparison.
func versionSkew(baseURL, localCommit string) Check {
	return Check{
		Name: "version-skew",
		Run: func(ctx context.Context) error {
			if localCommit == "" {
				return skip{"local build carries no commit; skipping comparison"}
			}
			status, body, err := getVersion(ctx, baseURL)
			if err != nil {
				return fmt.Errorf("cannot read %s/version: %w", baseURL, err)
			}
			if status != http.StatusOK {
				return fmt.Errorf("GET %s/version returned %d, want 200", baseURL, status)
			}
			var remote buildinfo.Info
			if err := json.Unmarshal(body, &remote); err != nil {
				return fmt.Errorf("decode %s/version: %w", baseURL, err)
			}
			if remote.Commit == "" {
				return skip{"service reports no build commit; skipping comparison"}
			}
			if remote.Commit != localCommit {
				return fmt.Errorf(
					"this binary is built from %.7s, the service serves %.7s: "+
						"new binary on disk, old process serving; restart via t-man and re-check",
					localCommit, remote.Commit)
			}
			return nil
		},
	}
}

// getVersion GETs baseURL+"/version" and reads the whole body under the
// probe timeout.
func getVersion(ctx context.Context, baseURL string) (status int, body []byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/version", nil)
	if err != nil {
		return 0, nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err = io.ReadAll(res.Body)
	if err != nil {
		return 0, nil, err
	}
	return res.StatusCode, body, nil
}

// expandTilde rewrites a leading ~ or ~/ to the user's home directory, so
// checks probe the same path the component's CLI resolves. When the home
// directory cannot be determined the path is returned unchanged and the
// check fails with the underlying open error.
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
