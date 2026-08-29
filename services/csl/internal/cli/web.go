package cli

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/csl"
	"github.com/mad01/thismoon/services/csl/internal/refresh"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/web"
)

var webPortFlag int

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Serve a local web UI for code search",
	Long: `Serve a code-search web UI on localhost.

The main page (/) has a search box and example queries; the Examples tab lists
more queries. Results are grouped by repo and file, link to the file on its git
host, and can be expanded inline. A JSON API backs the UI:

  GET /api/search?q=<query>&mode=files|content&repo=&lang=&file=&limit=&context=
  GET /api/read?repo=<name>&file=<path>&start=&end=
  GET /api/repos
  GET /healthz

The server binds to 127.0.0.1 only and uses the same index as ` + "`csl search`" + `.
It serves http://localhost:7424 by default (http://csl.this when d-man fronts
it with that alias).

Typically run as a background service:
  t-man add --name csl-web -- csl web --port 7424

Moving the UI off the default port: --port changes this process only. Other
csl processes — the MCP server building csl_show_file links, ` + "`csl doctor`" + `
probing the UI — learn the new port from CSL_PORT, or from web.base_url in
config.yaml when the UI is fronted by a proxy.`,
	RunE: runWeb,
}

func init() {
	webCmd.Flags().IntVar(&webPortFlag, "port", csl.ResolvedPort(),
		"port to listen on, loopback only (env CSL_PORT)")
	rootCmd.AddCommand(webCmd)
}

func runWeb(_ *cobra.Command, _ []string) error {
	cfg, err := webConfig()
	if err != nil {
		return err
	}
	// Everything in this process that links to the UI should link to the port
	// it actually bound. An explicit web.base_url still wins: it names a front
	// (http://csl.this) that no local port can describe.
	if cfg.Web.BaseURL == "" {
		cfg.Web.BaseURL = csl.BaseURLForPort(webPortFlag)
	}

	svc, err := web.NewService(cfg)
	if err != nil {
		return err
	}

	// The refresher loop runs for the life of the server; manual kicks from
	// the UI work even when the periodic refresh is disabled in config.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ref := refresh.New(cfg)
	go ref.Run(ctx)
	if cfg.RefreshEnabled() {
		log.Printf("csl web: background refresh every %s", cfg.RefreshInterval())
	} else {
		log.Printf("csl web: background refresh disabled (refresh.enabled: false)")
	}

	handler := web.New(svc, buildinfo.Get(), ref).Handler()
	addr := listenAddr(webPortFlag)
	log.Printf("csl web: serving on http://localhost:%d", webPortFlag)
	return http.ListenAndServe(addr, handler)
}

// webConfig loads the csl config for the web server and says so when there is
// none. Load already treats a missing file as defaults, which is what keeps a
// fresh install from crash-looping under a keep_alive service manager; the log
// line is the "and then must say so" half of that (ADR-0011), since a server
// serving zero repos otherwise looks broken rather than unconfigured. A file
// that exists but does not parse stays a hard error.
func webConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	if !cfg.Loaded {
		log.Printf("csl web: serving with no repos — %s, then restart", cfg.EmptyResultHint())
	}
	return cfg, nil
}

// listenAddr pins the loopback interface explicitly. A bare ":<port>" would
// listen on 0.0.0.0 (all interfaces), exposing local source code to anything
// that can route to this machine. The UI is unauthenticated, so it must stay
// reachable only from localhost.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
