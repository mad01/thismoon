package cli

import (
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/events/internal/server"
	"github.com/mad01/thismoon/services/events/internal/store"
)

var (
	flagPerSourceCap int
	flagGlobalCap    int
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the events HTTP server (web timeline + JSON API)",
	Long: `Serve the event log over HTTP on localhost: the web timeline at /, and the
JSON API the MCP and CLI call. serve is the single writer of the JSONL store.
It is archive-only — there is no ticker and nothing fires.

Typically run as a background service:
  t-man add --name events -- events serve --port 7430`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().IntVar(&flagPerSourceCap, "per-source-cap", store.DefaultPerSourceCap,
		"max events kept in memory per source")
	serveCmd.Flags().IntVar(&flagGlobalCap, "global-cap", store.DefaultGlobalCap,
		"max events returned across all sources in one query")
	rootCmd.AddCommand(serveCmd)
}

func runServe(_ *cobra.Command, _ []string) error {
	st, err := store.New(flagWorkdir, flagPerSourceCap, flagGlobalCap, nil)
	if err != nil {
		return err
	}
	handler := server.New(st, buildinfo.Get()).Handler()
	log.Printf(
		"events: serving %s on http://localhost:%d (per-source cap %d, global cap %d)",
		flagWorkdir,
		flagPort,
		flagPerSourceCap,
		flagGlobalCap,
	)
	return http.ListenAndServe(listenAddr(flagPort), handler)
}

// listenAddr pins the loopback interface: the event log is personal and nothing
// authenticates requests, so the server must stay reachable only from localhost.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
