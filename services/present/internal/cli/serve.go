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
	"strconv"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/envdefault"
	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/mcpserver"
	"github.com/mad01/thismoon/services/present/internal/server"
	"github.com/mad01/thismoon/services/present/internal/store"
	"github.com/mad01/thismoon/services/present/internal/store/k8sstore"
)

// Timeouts for one served connection and for shutdown. A client that opens
// a connection and sends no headers must not hold one forever; a shutdown
// waits that long for in-flight requests before dropping them.
const (
	readHeaderTimeout = 10 * time.Second
	drainTimeout      = 10 * time.Second
)

var (
	flagShared    bool
	flagBind      string
	flagStore     string
	flagNamespace string
	flagSweep     time.Duration
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the HTTP server that serves presentation pages",
	Long: `Serve presentation pages over HTTP. The index (/) lists all pages and
/p/<id> serves a single page; both are static chrome-only shells that render
client-side, fetching the page list from /api/pages and each page from
/api/p/<id> as JSON. Pages are read fresh on every request and open tabs poll
for version changes, so content updates appear without a restart.

Typically run as a background service:
  t-man add --name present -- present serve --port 7423

--shared turns the same binary into a network-facing shared instance: there
is no index and no listing, a page is reachable only by the id it was created
with, every write needs the caller's author key as a bearer token, and the
present_* tools are served over HTTP at /mcp (present_create, present_read,
present_source, present_update, present_doctor). Shared mode never binds
implicitly: pass --bind (0.0.0.0 inside a container, 127.0.0.1 to try it on
this machine). Local mode refuses any bind but loopback, because it
authenticates nothing.

--store picks where pages live: fs (the workdir, the default) or k8s, which
keeps each page as a Page custom resource in --namespace and is the store a
shared instance with several replicas uses. With k8s the process also sweeps
expired ephemeral pages every --sweep-interval.`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().BoolVar(&flagShared, "shared",
		envdefault.Bool("PRESENT_SHARED", false),
		"run as a shared instance: pages by id only, author keys on writes, MCP at /mcp (env PRESENT_SHARED)")
	serveCmd.Flags().StringVar(&flagBind, "bind",
		envdefault.String("PRESENT_BIND", present.DefaultBind),
		"interface to listen on (env PRESENT_BIND); shared mode requires it explicitly")
	serveCmd.Flags().StringVar(&flagStore, "store",
		envdefault.String("PRESENT_STORE", present.DefaultStore),
		"page store: fs (the workdir) or k8s (Page custom resources; shared mode only) (env PRESENT_STORE)")
	serveCmd.Flags().StringVar(&flagNamespace, "namespace",
		envdefault.String("PRESENT_NAMESPACE", ""),
		"namespace the k8s store keeps pages in (env PRESENT_NAMESPACE); default: the pod's, else the kubeconfig context's")
	serveCmd.Flags().DurationVar(&flagSweep, "sweep-interval",
		envdefault.Duration("PRESENT_SWEEP_INTERVAL", present.DefaultSweepInterval),
		"how often the k8s store purges expired ephemeral pages (env PRESENT_SWEEP_INTERVAL)")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, _ []string) error {
	bindExplicit := cmd.Flags().Changed("bind") || os.Getenv("PRESENT_BIND") != ""
	if err := checkBind(flagShared, flagBind, bindExplicit); err != nil {
		return err
	}
	if flagStore != "fs" && !flagShared {
		return fmt.Errorf(
			"--store %s is for shared mode; local present writes the workdir",
			flagStore,
		)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	raw, where, err := openStore()
	if err != nil {
		return err
	}
	st := raw
	opts := server.Options{Workdir: flagWorkdir, Info: buildinfo.Get(), BaseURL: flagBaseURL}
	mode := "local"
	if !flagShared {
		// Only a local serve pushes pages elsewhere; a shared instance is
		// where they land.
		opts.Sharer = sharer()
	}
	if flagShared {
		mode = "shared"
		// Expiry first, then the size cap: a shared instance refuses an
		// oversized page whichever store it runs on, and never writes to a
		// page that has already expired.
		st = store.WithSizeLimit(store.WithoutExpired(raw, time.Now), present.MaxPageBytes)
		opts.Mode = server.ModeShared
		opts.MCP, err = sharedMCP(st)
		if err != nil {
			return err
		}
	}
	if ks, ok := raw.(*k8sstore.Store); ok {
		go k8sstore.RunSweeper(ctx, ks, flagSweep, log.Printf)
	} else if flagShared {
		log.Printf("present: shared mode on the filesystem store; run one replica only")
	}
	handler := server.New(st, opts).Handler()
	addr := bindAddr(flagBind, flagPort)
	log.Printf("present: serving %s on http://%s (%s mode)", where, addr, mode)
	return serveUntilSignal(ctx, addr, handler)
}

// openStore opens the store --store names and describes it for the log.
func openStore() (store.Store, string, error) {
	switch flagStore {
	case "fs":
		fs, err := store.NewFS(flagWorkdir)
		if err != nil {
			return nil, "", err
		}
		return fs, "fs " + flagWorkdir, nil
	case "k8s":
		cfg, err := k8sstore.RESTConfig()
		if err != nil {
			return nil, "", err
		}
		client, err := dynamic.NewForConfig(cfg)
		if err != nil {
			return nil, "", fmt.Errorf("kubernetes client: %w", err)
		}
		ns := k8sstore.ResolveNamespace(flagNamespace)
		st, err := k8sstore.New(k8sstore.Config{Client: client, Namespace: ns})
		if err != nil {
			return nil, "", err
		}
		return st, "k8s pages in namespace " + ns, nil
	default:
		return nil, "", fmt.Errorf("unknown --store %q: want fs or k8s", flagStore)
	}
}

// sharedMCP builds the tool server a shared instance mounts at /mcp. One
// server instance serves every request: the transport is stateless, so no
// session lives on this replica, and each tool call carries its own HTTP
// headers (the author key and the forwarded host).
func sharedMCP(st store.Store) (http.Handler, error) {
	srv, err := mcpserver.New(buildinfo.Get().Version, mcpserver.Config{
		Store:   st,
		Mode:    mcpserver.ModeShared,
		BaseURL: flagBaseURL,
		Port:    flagPort,
		Checks:  sharedChecks(st),
	})
	if err != nil {
		return nil, err
	}
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	), nil
}

// sharedChecks is the doctor set a shared instance answers present_doctor
// with: the tools run inside the serving process, so reachability and skew
// mean nothing there and the store is the one thing that can be down.
func sharedChecks(st store.Store) func(context.Context) []doctor.Check {
	return func(context.Context) []doctor.Check {
		return []doctor.Check{{Name: "store-reachable", Run: st.Ping}}
	}
}

// serveUntilSignal listens on addr and serves until ctx ends (SIGINT or
// SIGTERM).
func serveUntilSignal(ctx context.Context, addr string, handler http.Handler) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	return serveUntilDone(ctx, ln, handler)
}

// serveUntilDone serves ln until ctx ends, then drains in-flight requests
// before returning. A rolling update sends SIGTERM and waits; an abrupt
// exit there turns a deploy into a burst of reset connections. It takes the
// listener rather than an address so a test can serve a port the kernel
// picked.
func serveUntilDone(ctx context.Context, ln net.Listener, handler http.Handler) error {
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: readHeaderTimeout}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Printf("present: shutting down")
		drain, cancel := context.WithTimeout(context.Background(), drainTimeout)
		defer cancel()
		if err := srv.Shutdown(drain); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// checkBind enforces the bind posture. Local mode authenticates nothing, so
// it may only listen on loopback; shared mode is meant to be reached over a
// network, so it must be told where to listen rather than guess.
func checkBind(shared bool, bind string, explicit bool) error {
	if shared {
		if !explicit {
			return errors.New("shared mode needs an explicit --bind (or PRESENT_BIND): " +
				"0.0.0.0 inside a container, 127.0.0.1 to test on this machine")
		}
		return nil
	}
	if !isLoopback(bind) {
		return fmt.Errorf(
			"local mode authenticates nothing, so --bind must stay on loopback (got %q); "+
				"use --shared for a network-facing instance",
			bind,
		)
	}
	return nil
}

func isLoopback(bind string) bool {
	if bind == "localhost" {
		return true
	}
	ip := net.ParseIP(bind)
	return ip != nil && ip.IsLoopback()
}

// bindAddr joins the interface and port into a listen address.
func bindAddr(bind string, port int) string {
	return net.JoinHostPort(bind, strconv.Itoa(port))
}

// listenAddr is the local-mode listen address: the loopback interface,
// explicitly. A bare ":<port>" would listen on 0.0.0.0 (all interfaces),
// exposing pages to anything that can route to this machine, and local mode
// authenticates no request.
func listenAddr(port int) string {
	return bindAddr(present.DefaultBind, port)
}
