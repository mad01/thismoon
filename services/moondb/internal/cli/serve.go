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
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/ingest"
	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
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
  t-man add --name moondb -- moondb serve --port ` + fmt.Sprint(moondb.DefaultPort),
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(_ *cobra.Command, _ []string) error {
	if err := requireConfig(); err != nil {
		return err
	}
	prices, err := priceOverrides(cfg.Prices)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	st, err := local.Open(flagWorkdir)
	if err != nil {
		return err
	}
	defer func() {
		if err := st.Close(); err != nil {
			log.Printf("moondb: close store: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ing := &ingest.Ingester{
		Store: st,
		Spool: &spool.Reader{Dir: filepath.Join(flagWorkdir, moondb.SpoolDirName)},
		Now:   time.Now,
		Logf:  log.Printf,
	}
	go func() {
		if err := ing.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("moondb: ingest stopped: %v", err)
		}
	}()

	svc := &query.Service{Store: st, Prices: trace.NewPrices(prices)}
	srv := &http.Server{
		Addr:              listenAddr(flagPort),
		Handler:           server.New(st, svc, buildinfo.Get()).Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Printf("moondb: serving %s on http://localhost:%d", flagWorkdir, flagPort)
	log.Printf(
		"moondb: register as a background service with: t-man add --name moondb -- moondb serve --port %d",
		flagPort,
	)
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
