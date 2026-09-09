package cli

import (
	"context"
	"fmt"
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
)

// proxyProbeURL is where doctor expects the running daemon: SitesPath is the
// one path d-man answers itself on any host, so a 200 there proves the proxy
// without involving a backend. The daemon's production port is serve's
// default :80.
const proxyProbeURL = "http://127.0.0.1:80" + proxy.SitesPath

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
