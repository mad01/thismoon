package cli

import (
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	catalogroot "github.com/mad01/thismoon/services/catalog"
	"github.com/mad01/thismoon/services/catalog/internal/web"
)

var webPortFlag int

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Serve the catalog web UI on localhost",
	Long: `Serve the systems-catalog web UI on localhost.

The UI lists Systems and Components, supports search by name or owner, and has
System and Component detail pages showing full metadata. A Refresh button
re-scans the source repos; an Add form writes a new service-info.yaml into a
registered repo. A JSON API backs the UI:

  GET  /api/entities | /api/systems | /api/components
  GET  /api/systems/{name} | /api/components/{name}
  GET  /api/search?q=&owner=&kind=
  GET  /api/owners
  POST /api/refresh
  POST /api/entities

The server binds to 127.0.0.1 only; the UI is unauthenticated and must stay
reachable from localhost alone.`,
	RunE: runWeb,
}

func init() {
	webCmd.Flags().IntVar(&webPortFlag, "port", catalogroot.DefaultPort,
		"port to listen on (loopback only)")
	rootCmd.AddCommand(webCmd)
}

func runWeb(cmd *cobra.Command, _ []string) error {
	srv, err := web.New(cmd.Context(), registryPath, buildinfo.Get())
	if err != nil {
		return fmt.Errorf("failed to load catalog: %w\n\nHint: create %s with a 'sources' list", err, registryPath)
	}
	addr := listenAddr(webPortFlag)
	log.Printf("catalog web: serving on http://localhost:%d", webPortFlag)
	return http.ListenAndServe(addr, srv.Handler())
}

// listenAddr pins the loopback interface explicitly. A bare ":<port>" would
// listen on 0.0.0.0, exposing the unauthenticated UI to the local network.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
