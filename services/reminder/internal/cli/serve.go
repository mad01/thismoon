package cli

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/reminder/internal/notify"
	"github.com/mad01/thismoon/services/reminder/internal/server"
	"github.com/mad01/thismoon/services/reminder/internal/store"
	"github.com/mad01/thismoon/services/reminder/internal/ticker"
)

// tickInterval is how often the firing loop scans for due reminders. A coarser
// interval is fine — a reminder fires within this window of its due time.
const tickInterval = 30 * time.Second

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the reminder HTTP server and firing ticker",
	Long: `Serve reminders over HTTP on localhost: the web page at /, the JSON API the
MCP and CLI call, and the background ticker that fires a macOS notification when
a reminder comes due.

Typically run as a background service:
  t-man add --name reminder -- reminder serve --port 7428`,
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ticker.Run(
		ctx,
		st,
		notify.Osascript{},
		tickInterval,
		func() time.Time { return time.Now().UTC() },
	)

	handler := server.New(st, Version, notify.Osascript{}).Handler()
	log.Printf(
		"reminder: serving %s on http://localhost:%d (tick %s)",
		flagWorkdir,
		flagPort,
		tickInterval,
	)
	return http.ListenAndServe(listenAddr(flagPort), handler)
}

// listenAddr pins the loopback interface: reminders are personal and nothing
// authenticates requests, so the server must stay reachable only from localhost.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
