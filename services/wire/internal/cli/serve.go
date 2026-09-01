package cli

import (
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/wire/internal/server"
	"github.com/mad01/thismoon/services/wire/internal/store"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the wire HTTP server that owns the store",
	Long: `Serve channels over HTTP on localhost: the web page at /, the JSON API the
MCP and CLI call, and the event stream a live transcript follows.

serve is also where a blocking read parks (a session waiting for another
session's reply is a goroutine in this process) so it has to be running for
any of the other surfaces to work.

Typically run as a background service:
  t-man add --name wire -- wire serve --port 7432`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(_ *cobra.Command, _ []string) error {
	st, err := store.New(flagWorkdir)
	if err != nil {
		return err
	}

	handler := server.New(st, buildinfo.Get(), flagPort).Handler()
	log.Printf("wire: serving %s on http://localhost:%d", flagWorkdir, flagPort)
	log.Printf(
		"wire: register as a background service with: t-man add --name wire -- wire serve --port %d",
		flagPort,
	)
	// No WriteTimeout: a long-polled read and an event stream both hold a
	// connection open on purpose, and a write deadline would sever them.
	srv := &http.Server{Addr: listenAddr(flagPort), Handler: handler}
	return srv.ListenAndServe()
}

// listenAddr pins the loopback interface: conversations between sessions are
// personal and nothing authenticates requests, so the server must stay
// reachable only from localhost.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
