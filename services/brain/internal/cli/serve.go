package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/envdefault"
	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/server"
	"github.com/mad01/thismoon/services/brain/internal/server/ui"
)

// Server timeouts. Shutdown is how long in-flight requests get once the
// process is told to stop.
const (
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 5 * time.Second
)

// maxPort is the highest TCP port.
const maxPort = 65535

func newServeCmd(a *app) *cobra.Command {
	var port int
	c := &cobra.Command{
		Use:   "serve",
		Short: "Serve the read-only web view of the vaults on localhost",
		Long: `Serve the vaults over HTTP on 127.0.0.1: the graph UI at /, and the JSON API
under /api/ that docs/serve.md specifies. serve never writes a vault and
never runs git or gh; its one write is the graph layout cache under the
state directory's layout/. A search refreshes a stale index first, under
the same per-vault lock the CLI takes.

With no config, or none of its vaults readable, serve still starts and says
so through /api/doctor, so a service manager does not restart it in a
loop. --port wins over ` + brain.PortEnv + `, which wins over ` + fmt.Sprint(brain.DefaultPort) + `; port 0
picks a free one. Logs one line per request on stderr.

Typically run as a background service:
  t-man add --name brain-serve -- brain serve --port ` + fmt.Sprint(brain.DefaultPort),
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if port < 0 || port > maxPort {
				return usageErrorf("--port must be 0 to %d, got %d", maxPort, port)
			}
			// run discards the standard logger for zoekt; serve's request
			// log goes to stderr.
			log.SetOutput(cmd.ErrOrStderr())
			cfg, cfgErr := config.Load(a.configPath)
			if cfgErr != nil {
				log.Printf("brain: serving no vault: %v (diagnose with: brain doctor)", cfgErr)
			}
			srv, err := server.New(server.Options{
				Config:     cfg,
				ConfigPath: a.configPath,
				ConfigErr:  cfgErr,
				UI:         ui.Files(),
				Info:       buildinfo.Get(),
			})
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return listen(ctx, srv.Handler(), port)
		},
	}
	c.Flags().IntVar(&port, "port", envdefault.Int(brain.PortEnv, brain.DefaultPort),
		"loopback port to listen on (env "+brain.PortEnv+")")
	return c
}

// listen serves h on the loopback port until ctx ends, then gives
// in-flight requests shutdownTimeout to finish.
func listen(ctx context.Context, h http.Handler, port int) error {
	ln, err := net.Listen("tcp", listenAddr(port))
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: readHeaderTimeout}
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Printf("brain: serving on http://%s", ln.Addr())
	err = srv.Serve(ln)
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-done
	return nil
}

// listenAddr pins the loopback interface: the API serves every note of
// every vault and nothing authenticates a request, so serve must stay
// reachable only from this machine.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
