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

// proxyProbeURL is SitesPath on the daemon: the one path d-man answers itself
// on any host, so a 200 there proves the proxy without involving a backend.
const proxyProbeURL = proxyBaseURL + proxy.SitesPath

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

// proxyAnswers GETs the daemon's own sites endpoint on the production port.
// A refused connection means serve is not running; any answer that is not
// the sites list means something other than d-man holds :80.
func proxyAnswers() doctor.Check {
	return doctor.Check{
		Name: "proxy-answers",
		Run: func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, doctorProbeTimeout)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyProbeURL, nil)
			if err != nil {
				return err
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("d-man serve is not answering on 127.0.0.1:80: %w", err)
			}
			defer func() { _ = res.Body.Close() }()
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf(
					"GET %s returned %d, want 200: something other than d-man may hold :80",
					proxyProbeURL, res.StatusCode)
			}
			return nil
		},
	}
}

// noRouteText starts the body of the 502 the proxy answers for a host it has
// no route for (proxy.Handler.ServeHTTP). A test pins the two together.
const noRouteText = "d-man: no route for"

// settingsAnswers GETs the settings document with the Host of the first
// configured route, the only kind of host that serves it. Validate keeps a
// route host off the block list, so a daemon running this routes file never
// answers it with the block page. A serve built before the endpoint proxies
// the path to that route's backend instead, so this catches an old daemon
// still running after an upgrade.
func settingsAnswers() doctor.Check {
	return doctor.Check{
		Name: "settings-answers",
		Run: func(ctx context.Context) error {
			cfg, err := config.Load(flagConfig)
			if err != nil {
				return doctor.Skip("routes file does not load; config-loads says why")
			}
			if len(cfg.Routes) == 0 {
				return doctor.Skip("no routes configured, so no host serves the settings")
			}
			return probeSettings(ctx, proxyBaseURL, cfg.Routes[0].Host(cfg.Suffix))
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Host = host
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("d-man serve is not answering on %s: %w", baseURL, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusBadGateway && bodyHas(res.Body, noRouteText) {
		return fmt.Errorf("GET %s as %s returned 502: the daemon has no route for %s, "+
			"so it runs with another routes file than doctor's --config", url, host, host)
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
