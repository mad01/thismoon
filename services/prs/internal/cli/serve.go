package cli

import (
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/prs/internal/config"
	"github.com/mad01/thismoon/services/prs/internal/github"
	"github.com/mad01/thismoon/services/prs/internal/poller"
	"github.com/mad01/thismoon/services/prs/internal/server"
	"github.com/mad01/thismoon/services/prs/internal/store"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the prs HTTP server that owns the cache and polls GitHub",
	Long: `Serve the PR dashboard over HTTP on localhost: the web page at /, the JSON
API the MCP and CLI call, and the background poller that keeps the cache
fresh against every configured GitHub host.

Typically run as a background service:
  t-man add --name prs -- prs serve --port 7427`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return err
	}
	if len(cfg.Dirs) == 0 {
		log.Printf(
			"prs: no dirs configured in %s — nothing will be polled until dirs are set", cfg.Path,
		)
	}
	st, err := store.New(flagWorkdir)
	if err != nil {
		return err
	}

	fetcher := github.NewClient(github.NewTokenSource())
	p := poller.New(cfg, st, fetcher)
	// The poller runs for the life of the serve process; cobra's context
	// carries the interrupt so a shutdown stops the loop too.
	go p.Run(cmd.Context())

	handler := server.New(st, p, cfg, buildinfo.Get()).Handler()
	log.Printf("prs: serving %s on http://localhost:%d (poll interval %s)",
		flagWorkdir, flagPort, cfg.PollInterval)
	log.Printf(
		"prs: register as a background service with: t-man add --name prs -- prs serve --port %d",
		flagPort,
	)
	return http.ListenAndServe(listenAddr(flagPort), handler)
}

// listenAddr pins the loopback interface: the PR list is personal and
// nothing authenticates requests, so the server must stay reachable only
// from localhost.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
