package cli

import (
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/keep/internal/server"
	"github.com/mad01/thismoon/services/keep/internal/store"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the keep HTTP server that owns the store",
	Long: `Serve assertions over HTTP on localhost: the read-only web page at /, the
JSON API the MCP and CLI call, and the pin resolution/hashing that only the
process with a coherent view of the working tree can do.

Typically run as a background service:
  t-man add --name keep -- keep serve --port 7431`,
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

	handler := server.New(st, Version).Handler()
	log.Printf("keep: serving %s on http://localhost:%d", flagWorkdir, flagPort)
	log.Printf(
		"keep: register as a background service with: t-man add --name keep -- keep serve --port %d",
		flagPort,
	)
	return http.ListenAndServe(listenAddr(flagPort), handler)
}

// listenAddr pins the loopback interface: assertions are personal and nothing
// authenticates requests, so the server must stay reachable only from localhost.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
