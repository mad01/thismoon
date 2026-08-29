package cli

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/deps/internal/alert"
	"github.com/mad01/thismoon/services/deps/internal/config"
	"github.com/mad01/thismoon/services/deps/internal/discover"
	"github.com/mad01/thismoon/services/deps/internal/osv"
	"github.com/mad01/thismoon/services/deps/internal/registry"
	"github.com/mad01/thismoon/services/deps/internal/scanner"
	"github.com/mad01/thismoon/services/deps/internal/server"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// flagInterval is the maximum age of the last full scan before serve runs
// another. Daily is plenty — advisories trickle in over days.
var flagInterval time.Duration

// heartbeat is how often the loop wakes to check whether a scan is due. Short
// relative to the interval so a wake-from-sleep triggers a catch-up scan
// promptly (a time.Ticker does not advance while the Mac is asleep).
const heartbeat = 15 * time.Minute

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the deps HTTP server and periodic scan loop",
	Long: `Serve dependency findings over HTTP on localhost: the web page at /, the JSON
API the CLI and MCP call, and a background loop that re-checks dependencies
against OSV and fires a macOS notification when one is newly flagged.

Typically run as a background service:
  t-man add --name deps -- deps serve --port 7429`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().DurationVar(&flagInterval, "interval", 24*time.Hour,
		"max age of the last full scan before serve runs another (daily catch-up)")
	rootCmd.AddCommand(serveCmd)
}

// buildEngine wires the store, registry-backed repo resolver, ecosystems, OSV
// checker, and notifier into one Engine shared by serve's loop and handlers. The
// discovery config (exclude_repos / exclude_paths) is read once per scan inside
// the repo resolver, so editing it takes effect on the next cycle without a
// restart.
func buildEngine(st *store.Store, checker scanner.Checker, n alert.Notifier) *scanner.Engine {
	registryPath, configPath := flagRegistry, flagConfig
	return &scanner.Engine{
		Prepare: func() ([]string, discover.Options, error) {
			cfg, err := config.Load(configPath)
			if err != nil {
				return nil, discover.Options{}, err
			}
			repos, err := registry.Repos(registryPath)
			if err != nil {
				return nil, discover.Options{}, err
			}
			kept := repos[:0]
			for _, r := range repos {
				if !cfg.RepoExcluded(r) {
					kept = append(kept, r)
				}
			}
			return kept, discover.Options{ExcludePath: cfg.PathExcluded}, nil
		},
		Ecosystems: discover.Default(),
		Checker:    checker,
		Notifier:   n,
		Store:      st,
	}
}

func runServe(_ *cobra.Command, _ []string) error {
	st, err := store.New(flagWorkdir)
	if err != nil {
		return err
	}
	engine := buildEngine(st, osv.New(), alert.Osascript{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runScanLoop(ctx, st, engine, flagInterval)

	handler := server.New(st, engine, buildinfo.Get()).Handler()
	log.Printf(
		"deps: serving %s on http://localhost:%d (full scan when older than %s)",
		flagWorkdir, flagPort, flagInterval,
	)
	return http.ListenAndServe(listenAddr(flagPort), handler)
}

// maxBackoff caps the offline retry delay. When a scan fails (typically OSV
// unreachable because the Mac is offline), the loop backs off exponentially up
// to this so it isn't retrying every heartbeat for hours — there's nothing to
// gain hammering an API we can't reach.
const maxBackoff = 6 * time.Hour

// runScanLoop keeps the scan fresh: it wakes every heartbeat and runs a full
// check-and-notify whenever the last scan is older than interval. Checking
// staleness rather than counting ticks means a wake from a multi-hour sleep
// triggers the catch-up scan on the next heartbeat instead of silently skipping
// the day. On failure it backs off exponentially (offline-friendly) and resets
// on the next success.
func runScanLoop(
	ctx context.Context,
	st *store.Store,
	engine *scanner.Engine,
	interval time.Duration,
) {
	var (
		failures    int
		nextAttempt time.Time // earliest retry after a failure; zero = no backoff
	)
	cycle := func() {
		now := time.Now()
		if now.Before(nextAttempt) {
			return // backing off after a recent failure
		}
		last := st.Snapshot().ScannedAt
		if !last.IsZero() && now.Sub(last) < interval {
			return // still fresh
		}
		if err := engine.CheckAndNotify(ctx); err != nil {
			failures++
			delay := backoffDelay(failures)
			nextAttempt = now.Add(delay)
			log.Printf(
				"deps: scan cycle failed (attempt %d), backing off %s: %v",
				failures,
				delay,
				err,
			)
			return
		}
		failures = 0
		nextAttempt = time.Time{}
	}
	cycle() // catch up immediately on startup if stale or never scanned
	t := time.NewTicker(heartbeat)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cycle()
		}
	}
}

// backoffDelay grows the retry delay exponentially from one heartbeat, capped at
// maxBackoff: heartbeat, 2×, 4×, … so a long offline stretch settles into
// occasional retries rather than every-heartbeat churn.
func backoffDelay(failures int) time.Duration {
	delay := heartbeat << (failures - 1)
	if delay <= 0 || delay > maxBackoff { // <=0 guards against shift overflow
		return maxBackoff
	}
	return delay
}

// listenAddr pins the loopback interface: findings are personal and nothing
// authenticates requests, so the server stays reachable only from localhost.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
