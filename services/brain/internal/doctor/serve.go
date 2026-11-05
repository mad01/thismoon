package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	kitdoctor "github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/envdefault"
	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/server"
)

// serveProbeTimeout bounds each request to brain serve.
const serveProbeTimeout = 3 * time.Second

// The serve checks, in report order.
const (
	checkServeReachable = "serve-reachable"
	checkVersionSkew    = "version-skew"
	checkServeVaults    = "serve-vaults"
	checkServeStateDir  = "serve-state-dir"
)

// ServeURL is where brain doctor looks for brain serve: loopback on
// PortEnv, else DefaultPort.
func ServeURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", envdefault.Int(brain.PortEnv, brain.DefaultPort))
}

// checkServe compares what brain serve at baseURL sees with what this CLI
// sees. Nothing listening is not a failure: serve is optional, so every
// serve check is skipped. cfg is nil when the config did not load.
func checkServe(ctx context.Context, baseURL string, cfg *config.Config) []Check {
	reach, up := checkReachable(ctx, baseURL)
	if !up {
		return []Check{
			reach,
			skipped(checkVersionSkew, "needs brain serve"),
			skipped(checkServeVaults, "needs brain serve"),
			skipped(checkServeStateDir, "needs brain serve"),
		}
	}
	checks := []Check{reach, checkSkew(ctx, baseURL)}
	view, err := serveDoctor(ctx, baseURL)
	if err != nil {
		return append(checks,
			Check{
				Name:   checkServeVaults,
				Status: StatusFail,
				Detail: err.Error(),
				Fix:    "restart brain serve so it runs this binary",
			},
			skipped(checkServeStateDir, "needs "+baseURL+"/api/doctor"),
		)
	}
	return append(checks, checkServeVaultsSeen(cfg, view), checkStateDirSeen(view))
}

// checkReachable reports whether serve answers /healthz. Connection
// refused means nothing listens: a skip, and the other checks skip too.
func checkReachable(ctx context.Context, baseURL string) (Check, bool) {
	status, _, err := serveGet(ctx, baseURL+"/healthz")
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return skipped(
			checkServeReachable,
			"nothing listens on "+baseURL+"; start it with brain serve",
		), false
	case err != nil:
		return Check{
			Name:   checkServeReachable,
			Status: StatusFail,
			Detail: err.Error(),
			Fix:    "check what listens on " + baseURL + ", or restart brain serve",
		}, false
	case status != http.StatusOK:
		return Check{
			Name:   checkServeReachable,
			Status: StatusFail,
			Detail: fmt.Sprintf("GET %s/healthz returned %d, want 200", baseURL, status),
			Fix:    "check what listens on " + baseURL + ", or restart brain serve",
		}, false
	}
	return Check{Name: checkServeReachable, Status: StatusOK, Detail: baseURL + " answers"}, true
}

// checkSkew is kit/doctor's version-skew probe in brain's report shape.
func checkSkew(ctx context.Context, baseURL string) Check {
	r := kitdoctor.Collect(ctx, []kitdoctor.Check{kitdoctor.VersionSkew(baseURL)}).Checks[0]
	switch r.Status {
	case kitdoctor.StatusOK:
		return Check{Name: checkVersionSkew, Status: StatusOK, Detail: "serve runs this build"}
	case kitdoctor.StatusSkipped:
		return skipped(checkVersionSkew, r.Detail)
	}
	return Check{
		Name:   checkVersionSkew,
		Status: StatusFail,
		Detail: r.Detail,
		Fix:    "restart brain serve so it runs this binary",
	}
}

// serveDoctor reads serve's own view of its vaults and state directory.
func serveDoctor(ctx context.Context, baseURL string) (server.DoctorView, error) {
	var view server.DoctorView
	status, body, err := serveGet(ctx, baseURL+"/api/doctor")
	switch {
	case err != nil:
		return view, err
	case status == http.StatusNotFound:
		return view, errors.New("serve has no /api/doctor; it predates this check")
	case status != http.StatusOK:
		return view, fmt.Errorf("GET %s/api/doctor returned %d, want 200", baseURL, status)
	}
	if err := json.Unmarshal(body, &view); err != nil {
		return view, fmt.Errorf("decode %s/api/doctor: %w", baseURL, err)
	}
	return view, nil
}

// checkServeVaultsSeen fails when serve cannot read a vault the CLI's
// config names, or does not know it. Under the sandbox, a vault outside the
// shipped read paths needs its resolved path in the machine-local layer.
func checkServeVaultsSeen(cfg *config.Config, view server.DoctorView) Check {
	if cfg == nil {
		return skipped(checkServeVaults, "needs a config")
	}
	served := map[string]server.DoctorVault{}
	for _, v := range view.Vaults {
		served[v.Name] = v
	}
	var problems, unreadable []string
	missing := false
	for _, v := range cfg.Vaults {
		s, ok := served[v.Name]
		switch {
		case !ok:
			missing = true
			problems = append(
				problems,
				v.Name+": serve does not have it (serve reads "+view.Config+")",
			)
		case !s.Readable:
			path := resolvedPath(v.Path, s.Resolved)
			unreadable = append(unreadable, `"`+path+`"`)
			problems = append(problems, v.Name+": serve cannot read "+path+": "+s.Error)
		}
	}
	if len(problems) == 0 {
		return Check{
			Name:   checkServeVaults,
			Status: StatusOK,
			Detail: fmt.Sprintf("serve reads all %d vault(s)", len(cfg.Vaults)),
		}
	}
	var fixes []string
	if len(unreadable) > 0 {
		fixes = append(fixes, "add "+strings.Join(unreadable, ", ")+" to filesystem.allowRead in "+
			brain.ServeSandboxLocal+", then restart brain serve")
	}
	if missing {
		fixes = append(fixes, "point brain serve at the same config as the CLI (--config or "+
			brain.ConfigEnv+"), then restart it")
	}
	return Check{
		Name:   checkServeVaults,
		Status: StatusFail,
		Detail: strings.Join(problems, "; "),
		Fix:    strings.Join(fixes, "; "),
	}
}

// resolvedPath is path with its symlinks resolved here, where the CLI can
// see them, else the path serve resolved.
func resolvedPath(path, servedResolved string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	if servedResolved != "" {
		return servedResolved
	}
	return path
}

// checkStateDirSeen fails while serve and the CLI resolve different state
// directories, or serve cannot write its layout cache.
func checkStateDirSeen(view server.DoctorView) Check {
	dir, err := brain.StateDir()
	if err != nil {
		return Check{
			Name:   checkServeStateDir,
			Status: StatusFail,
			Detail: err.Error(),
			Fix:    "set HOME, or XDG_STATE_HOME to an absolute path",
		}
	}
	if !samePath(dir, view.StateDir) {
		return Check{
			Name:   checkServeStateDir,
			Status: StatusFail,
			Detail: "the CLI uses " + dir + ", serve uses " + view.StateDir,
			Fix:    "run brain serve with the CLI's XDG_STATE_HOME (or none), then restart it",
		}
	}
	if !view.LayoutDirWritable {
		layout := filepath.Join(dir, brain.LayoutDirName)
		return Check{
			Name:   checkServeStateDir,
			Status: StatusFail,
			Detail: "serve cannot write " + layout,
			Fix: `add "` + dir + `" to filesystem.allowWrite in ` + brain.ServeSandboxLocal +
				", then restart brain serve",
		}
	}
	return Check{Name: checkServeStateDir, Status: StatusOK, Detail: "serve shares " + dir}
}

// samePath compares two directories after resolving symlinks where they
// resolve.
func samePath(a, b string) bool {
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// serveGet GETs url under serveProbeTimeout and reads the whole body.
func serveGet(ctx context.Context, url string) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, serveProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	return res.StatusCode, body, err
}
