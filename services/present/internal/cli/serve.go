package cli

import (
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/server"
	"github.com/mad01/thismoon/services/present/internal/store"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the local HTTP server that serves presentation pages",
	Long: `Serve presentation pages over HTTP on localhost. The index (/) lists all
pages; /p/<id> renders a single page from the shared core template. The store
and template are read fresh on every request, so content updates and template
edits appear without a restart.

Typically run as a background service:
  t-man add --name present -- present serve --port 7423`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(_ *cobra.Command, _ []string) error {
	if err := render.EnsureTemplate(flagWorkdir); err != nil {
		return err
	}
	st, err := store.New(flagWorkdir)
	if err != nil {
		return err
	}
	handler := server.New(st, flagWorkdir, Version).Handler()
	log.Printf("present: serving %s on http://localhost:%d", flagWorkdir, flagPort)
	return http.ListenAndServe(listenAddr(flagPort), handler)
}

// listenAddr builds the bind address for the HTTP server. It pins the loopback
// interface explicitly: a bare ":<port>" would listen on 0.0.0.0 (all
// interfaces), exposing pages to anything that can route to this machine —
// other devices on the LAN, VPN peers. Pages hold work and research summaries
// and nothing authenticates requests, so the server must stay reachable only
// from localhost, matching the http://localhost URLs the MCP hands out.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
