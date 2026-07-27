package cli

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"time"

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

	handler := server.New(st, Version, serveAuthor()).Handler()
	go watchStore(st)
	log.Printf("keep: serving %s on http://localhost:%d", flagWorkdir, flagPort)
	log.Printf(
		"keep: register as a background service with: t-man add --name keep -- keep serve --port %d",
		flagPort,
	)
	return http.ListenAndServe(listenAddr(flagPort), handler)
}

// serveAuthor is the identity stamped into the provenance of every assertion
// created through this server: KEEP_AUTHOR when set, else the OS username.
func serveAuthor() string {
	if a := os.Getenv("KEEP_AUTHOR"); a != "" {
		return a
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// storeReloadInterval is how often serve polls the log files for changes made
// by another process (a git pull of a synced workdir, a manual append).
const storeReloadInterval = 2 * time.Second

// watchStore reloads the store whenever its files change on disk underneath
// the running server. It runs for the life of the process.
func watchStore(st *store.Store) {
	t := time.NewTicker(storeReloadInterval)
	defer t.Stop()
	for range t.C {
		reloaded, err := st.ReloadIfChanged()
		if err != nil {
			log.Printf("keep: reload store: %v", err)
			continue
		}
		if reloaded {
			log.Printf("keep: store changed on disk, reloaded")
		}
	}
}

// listenAddr pins the loopback interface: assertions are personal and nothing
// authenticates requests, so the server must stay reachable only from localhost.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
