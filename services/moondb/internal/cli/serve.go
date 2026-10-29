package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/envdefault"
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/ingest"
	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/scorecard"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// Server timeouts. Shutdown is how long in-flight requests get once the
// process is told to stop; the ingest loop stops with the same context.
const (
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 5 * time.Second
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the moondb HTTP server that owns the store",
	Long: `Serve the edit history over HTTP on localhost: the web page at /, the JSON
API the MCP and CLI call, and the spool drain that turns what the hooks
recorded into store records. serve is the only process that writes the store.

Typically run as a background service:
  t-man add --name moondb -- moondb serve --port ` + fmt.Sprint(moondb.DefaultPort) + `

--replicate-to copies this store to a shared instance: the records in log
order and the chunks they point at, with the key in --replicate-key-file.
The hooks keep writing the local spool; nothing else changes here.

--shared runs the same binary as that shared instance: it drains no spool,
takes records from replicating serves at /api/sync, serves the MCP tools at
/mcp, and wants a key (one of the hashes in --keys-file) on every request
but the page shell. It never binds implicitly: pass --bind host:port
(0.0.0.0:7433 inside a container). --backend cloud keeps its chunks in the
Cloud Storage bucket --gcs-bucket and its log under --data-dir.`,
	RunE: runServe,
}

func init() {
	addServeFlags(serveCmd.Flags())
	rootCmd.AddCommand(serveCmd)
}

func runServe(_ *cobra.Command, _ []string) error {
	if err := requireConfig(); err != nil {
		return err
	}
	if err := checkServeFlags(); err != nil {
		return err
	}
	prices, err := priceOverrides(cfg.Prices)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	table := trace.NewPrices(prices)
	if flagShared {
		return runShared(ctx, table)
	}
	return runLocal(ctx, table)
}

// runLocal serves this machine's store on loopback: it drains the spool
// the hooks write and, with --replicate-to, copies the store to a shared
// instance.
func runLocal(ctx context.Context, table trace.Prices) error {
	st, err := local.Open(flagWorkdir)
	if err != nil {
		return err
	}
	defer closeStore(st)

	ing := &ingest.Ingester{
		Store:  st,
		Spool:  &spool.Reader{Dir: filepath.Join(flagWorkdir, moondb.SpoolDirName)},
		Now:    time.Now,
		Logf:   log.Printf,
		Prices: table,
	}
	go func() {
		if err := ing.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("moondb: ingest stopped: %v", err)
		}
	}()
	if err := startReplicate(ctx, st); err != nil {
		return err
	}
	q := &query.Service{Store: st, Prices: table, Peers: scorecardPeers()}
	return listen(ctx, server.New(st, q, buildinfo.Get()))
}

// closeStore closes st once serving stops, logging a failure: by then
// there is no caller left to return it to.
func closeStore(st store.Store) {
	if err := st.Close(); err != nil {
		log.Printf("moondb: close store: %v", err)
	}
}

// scorecardPeers points a scorecard at the events service on loopback,
// where EVENTS_BASE_URL says, as it does for belt.
func scorecardPeers() *scorecard.HTTPPeers {
	return scorecard.NewHTTPPeers(envdefault.String("EVENTS_BASE_URL", moondb.DefaultEventsURL))
}

// listen serves s on the loopback port until ctx ends.
func listen(ctx context.Context, s *server.Server) error {
	log.Printf("moondb: serving %s on http://localhost:%d", flagWorkdir, flagPort)
	log.Printf(
		"moondb: register as a background service with: t-man add --name moondb -- moondb serve --port %d",
		flagPort,
	)
	return serveOn(ctx, listenAddr(flagPort), s.Handler())
}

// serveOn serves h on addr until ctx ends, then gives in-flight requests
// shutdownTimeout to finish.
func serveOn(ctx context.Context, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// listenAddr pins the loopback interface: the store holds prompts and
// source and nothing authenticates requests, so the server must stay
// reachable only from localhost.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
