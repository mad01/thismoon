// Package check probes a discovered service: HTTP for services with a port,
// launchctl PID lookup for the rest.
package check

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/status/internal/discover"
)

// Result is the outcome of one probe.
type Result struct {
	Up     bool   `json:"up"`
	Detail string `json:"detail"`
}

const probeTimeout = 3 * time.Second

var httpClient = &http.Client{Timeout: probeTimeout}

// Service probes one service and never returns an error — failure to probe is
// itself the "down" signal.
func Service(ctx context.Context, svc discover.Service) Result {
	if svc.HTTP() {
		return overHTTP(ctx, svc.Port)
	}
	return overLaunchctl(ctx, svc.Label, svc.Daemon)
}

// overHTTP counts any HTTP response below 500 as up: a 404 from a service
// without a root route still proves the process accepts and serves requests.
func overHTTP(ctx context.Context, port int) Result {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	if err != nil {
		return Result{Up: false, Detail: err.Error()}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Result{Up: false, Detail: "no response"}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 500 {
		return Result{Up: false, Detail: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return Result{Up: true, Detail: fmt.Sprintf("HTTP %d", resp.StatusCode)}
}

func overLaunchctl(ctx context.Context, label string, daemon bool) Result {
	target := "gui/" + strconv.Itoa(os.Getuid()) + "/" + label
	if daemon {
		target = "system/" + label
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "launchctl", "print", target).Output()
	return FromLaunchctl(string(out))
}

// Runs samples launchd's cumulative spawn counter (`runs = N` in launchctl
// print) for a job. ok is false when the job or the counter isn't visible.
func Runs(ctx context.Context, svc discover.Service) (int, bool) {
	target := "gui/" + strconv.Itoa(os.Getuid()) + "/" + svc.Label
	if svc.Daemon {
		target = "system/" + svc.Label
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "launchctl", "print", target).Output()
	return RunsFromLaunchctl(string(out))
}

// BinaryVersion asks the installed binary on disk for its build sha by running
// `<binary> version` (the cross-tool convention: prints the bare ldflags sha).
// Best-effort: a missing binary, a hang, or output that doesn't look like a
// version token all return "".
func BinaryVersion(ctx context.Context, path string) string {
	if path == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "version").Output()
	if err != nil {
		return ""
	}
	return VersionToken(string(out))
}

// versionTokenRe accepts a single short token: a git sha, "dev", or a semver
// string. Anything with spaces or multiple lines is not a version.
var versionTokenRe = regexp.MustCompile(`^[0-9A-Za-z._+-]{1,64}$`)

// VersionToken trims and validates one line of `version` output. Exported for
// tests.
func VersionToken(out string) string {
	s := strings.TrimSpace(out)
	if versionTokenRe.MatchString(s) {
		return s
	}
	return ""
}

var runsRe = regexp.MustCompile(`(?m)^\s*runs = (\d+)`)

// RunsFromLaunchctl extracts the runs counter from `launchctl print` output.
// Exported for tests.
func RunsFromLaunchctl(out string) (int, bool) {
	m := runsRe.FindStringSubmatch(out)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

var pidRe = regexp.MustCompile(`(?m)^\s*pid = (\d+)`)

// FromLaunchctl classifies `launchctl print` output: a job with a pid line is
// running. Exported for tests.
func FromLaunchctl(out string) Result {
	if m := pidRe.FindStringSubmatch(out); m != nil {
		return Result{Up: true, Detail: "pid " + m[1]}
	}
	return Result{Up: false, Detail: "not running"}
}
