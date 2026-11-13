package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	dman "github.com/mad01/thismoon/services/d-man"
	"github.com/mad01/thismoon/services/d-man/internal/config"
	"github.com/mad01/thismoon/services/d-man/internal/hosts"
	"github.com/mad01/thismoon/services/d-man/internal/proxy"
	"github.com/mad01/thismoon/services/d-man/internal/settings"
)

// proxyBaseURL is where doctor expects the running daemon: serve's default
// production port :80.
const proxyBaseURL = "http://127.0.0.1:80"

// maxProbeBody caps how much of a probe answer doctor reads.
const maxProbeBody = 64 << 10

// doctorProbeTimeout bounds the proxy probe: a diagnostic that hangs is worse
// than one that fails.
const doctorProbeTimeout = 3 * time.Second

func init() {
	// Checks build at run time so they read the resolved --config and
	// --hosts-file (env and tilde expansion included), not the compile-time
	// defaults.
	rootCmd.AddCommand(agentcli.DoctorCommand(dman.Facts(), func(context.Context) []doctor.Check {
		return []doctor.Check{
			configLoads(),
			hostsEntriesPresent(),
			proxyAnswers(),
			settingsAnswers(),
		}
	}))
}

// configLoads runs the same load every subcommand works from — parse plus
// validation of the resolved routes file — so a bad routes.toml fails here
// with the file named.
func configLoads() doctor.Check {
	return doctor.Check{
		Name: "config-loads",
		Run: func(_ context.Context) error {
			_, err := config.Load(flagConfig)
			return err
		},
	}
}

// hostsEntriesPresent verifies the managed block d-man maintains exists in
// the hosts file and names every host the routes file produces. Read-only:
// doctor never writes the hosts file. When the routes file itself does not
// load, marker presence alone passes — config-loads reports the why.
func hostsEntriesPresent() doctor.Check {
	return doctor.Check{
		Name: "hosts-entries-present",
		Run: func(_ context.Context) error {
			data, err := os.ReadFile(flagHostsFile)
			if err != nil {
				return fmt.Errorf("read %s: %w", flagHostsFile, err)
			}
			block, ok := hosts.ManagedBlock(data)
			if !ok {
				return fmt.Errorf(
					"no managed block in %s: run 'sudo d-man sync' or start the daemon",
					flagHostsFile)
			}
			cfg, err := config.Load(flagConfig)
			if err != nil {
				return nil
			}
			var missing []string
			for _, h := range cfg.Hosts() {
				if !blockHasHost(block, h) {
					missing = append(missing, h)
				}
			}
			if len(missing) > 0 {
				return fmt.Errorf(
					"managed block in %s is missing %s: the daemon has not synced the current routes",
					flagHostsFile,
					strings.Join(missing, ", "),
				)
			}
			return nil
		},
	}
}

// proxyAnswers GETs the daemon's own sites endpoint on the production port,
// as the first configured route's host: the proxy answers it on no other.
// With no route configured it probes as unroutedHost instead. A refused
// connection means serve is not running; any answer that is not the sites
// list (or, with no route, d-man's no-route 502) means something other than
// d-man holds :80.
func proxyAnswers() doctor.Check {
	return doctor.Check{
		Name: "proxy-answers",
		Run: func(ctx context.Context) error {
			host, err := probeHost(flagConfig)
			if err != nil {
				return err
			}
			if host == "" {
				return probeUnrouted(ctx, proxyBaseURL)
			}
			return probeSites(ctx, proxyBaseURL, host)
		},
	}
}

// probeSites fetches SitesPath from the daemon at baseURL as host and wants a
// 200. A 502 carrying the proxy's no-route text means the daemon runs with
// another routes file than doctor's.
func probeSites(ctx context.Context, baseURL, host string) error {
	ctx, cancel := context.WithTimeout(ctx, doctorProbeTimeout)
	defer cancel()
	url := baseURL + proxy.SitesPath
	res, err := getAs(ctx, baseURL, proxy.SitesPath, host)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusBadGateway && bodyHas(res.Body, noRouteText) {
		return noRouteError(url, host)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"GET %s as %s returned %d, want 200: something other than d-man may hold :80",
			url, host, res.StatusCode)
	}
	return nil
}

// unroutedHost is the Host proxy-answers probes as when the routes file has
// no route. The .invalid top-level domain is reserved, so no routes file
// routes it and d-man answers it with its own no-route 502.
const unroutedHost = "d-man-doctor.invalid"

// probeUnrouted checks the daemon at baseURL for a routes file with no
// route, where no host serves the sites list. It GETs / as unroutedHost and
// wants d-man's no-route 502, an answer only d-man sends. Every serve
// answers / that way, including one that predates route-only sites.
func probeUnrouted(ctx context.Context, baseURL string) error {
	ctx, cancel := context.WithTimeout(ctx, doctorProbeTimeout)
	defer cancel()
	res, err := getAs(ctx, baseURL, "/", unroutedHost)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusBadGateway && bodyHas(res.Body, noRouteText) {
		return nil
	}
	return fmt.Errorf("GET %s/ as %s returned %d, want d-man's no-route 502: "+
		"something other than d-man may hold :80", baseURL, unroutedHost, res.StatusCode)
}

// getAs GETs path from the daemon at baseURL with host as the Host header.
// A transport error means serve is not answering. The caller closes the body.
func getAs(ctx context.Context, baseURL, path, host string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Host = host
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("d-man serve is not answering on %s: %w", baseURL, err)
	}
	return res, nil
}

// probeHost is the Host doctor probes the daemon's own paths as: the first
// route in the routes file at path, the only kind of host that serves them.
// Validate keeps a route host off the block list, so a daemon running this
// routes file never answers it with the block page. It is empty when the file
// has no route, and a file that does not load skips the check.
func probeHost(path string) (string, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return "", doctor.Skip("routes file does not load; config-loads says why")
	}
	if len(cfg.Routes) == 0 {
		return "", nil
	}
	return cfg.Routes[0].Host(cfg.Suffix), nil
}

// noRouteText starts the body of the 502 the proxy answers for a host it has
// no route for (proxy.Handler.ServeHTTP). A test pins the two together.
const noRouteText = "d-man: no route for"

// noRouteError reports a no-route 502 for host: the daemon loaded another
// routes file than the one doctor read.
func noRouteError(url, host string) error {
	return fmt.Errorf("GET %s as %s returned 502: the daemon has no route for %s, "+
		"so it runs with another routes file than doctor's --config", url, host, host)
}

// settingsAnswers GETs the settings document as the probe host. A serve built
// before the endpoint proxies the path to that route's backend instead, so
// this catches an old daemon still running after an upgrade.
func settingsAnswers() doctor.Check {
	return doctor.Check{
		Name: "settings-answers",
		Run: func(ctx context.Context) error {
			host, err := probeHost(flagConfig)
			if err != nil {
				return err
			}
			if host == "" {
				return doctor.Skip("no routes configured, so no host serves the settings")
			}
			return probeSettings(ctx, proxyBaseURL, host)
		},
	}
}

// probeSettings fetches SettingsPath from the daemon at baseURL as host and
// wants a 200 carrying a settings document of the current version. A 502
// carrying the proxy's no-route text means the daemon runs with another
// routes file, not an old binary; any other 502 is an old daemon whose
// backend for host is down.
func probeSettings(ctx context.Context, baseURL, host string) error {
	ctx, cancel := context.WithTimeout(ctx, doctorProbeTimeout)
	defer cancel()
	url := baseURL + proxy.SettingsPath
	res, err := getAs(ctx, baseURL, proxy.SettingsPath, host)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusBadGateway && bodyHas(res.Body, noRouteText) {
		return noRouteError(url, host)
	}
	const stale = "the running serve may predate the settings endpoint; " +
		"kill it so KeepAlive starts the current binary"
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s as %s returned %d, want 200: %s",
			url, host, res.StatusCode, stale)
	}
	var doc settings.Document
	err = json.NewDecoder(io.LimitReader(res.Body, maxProbeBody)).Decode(&doc)
	if err != nil || doc.V != settings.Version {
		return fmt.Errorf("GET %s as %s is not a v%d settings document: %s",
			url, host, settings.Version, stale)
	}
	return nil
}

// bodyHas reports whether the first maxProbeBody bytes of body contain text.
func bodyHas(body io.Reader, text string) bool {
	data, err := io.ReadAll(io.LimitReader(body, maxProbeBody))
	return err == nil && strings.Contains(string(data), text)
}

// blockHasHost reports whether the managed block maps host on some entry
// line. Matching is per whitespace-separated field so csl.this never matches
// mycsl.this.
func blockHasHost(block, host string) bool {
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		for _, f := range fields[1:] {
			if f == host {
				return true
			}
		}
	}
	return false
}
