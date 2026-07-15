package cli

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/spf13/cobra"

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

Typically run as a background service:
  t-man add --name csl-web -- csl web --port 7424`,
	RunE: runWeb,
}

func init() {
	webCmd.Flags().IntVar(&webPortFlag, "port", 7424, "port to listen on (loopback only)")
	rootCmd.AddCommand(webCmd)
}

func runWeb(_ *cobra.Command, _ []string) error {
	cfg, err := webConfig()
	if err != nil {
		return err
	}

	svc, err := web.NewService(cfg)
	if err != nil {
		return err
	}

	handler := web.New(svc, Version).Handler()
	addr := listenAddr(webPortFlag)
	log.Printf("csl web: serving on http://localhost:%d", webPortFlag)
	return http.ListenAndServe(addr, handler)
}

// webConfig loads the csl config for the web server. A missing config file is
// not fatal: the server starts with no configured dirs so a fresh install gets
// a working UI instead of a crash loop under a keep_alive service manager.
// Any other load failure (unreadable file, bad YAML) stays a hard error.
func webConfig() (*config.Config, error) {
	cfg, err := config.Load()
	switch {
	case errors.Is(err, os.ErrNotExist):
		log.Printf(
			"csl web: no config found; serving with no repos (create ~/.config/csl/config.yaml with a 'dirs' list, then restart)",
		)
		return &config.Config{}, nil
	case err != nil:
		return nil, fmt.Errorf("failed to load config: %w", err)
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
