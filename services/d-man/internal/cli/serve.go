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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/d-man/internal/config"
	"github.com/mad01/thismoon/services/d-man/internal/hosts"
	"github.com/mad01/thismoon/services/d-man/internal/notify"
	"github.com/mad01/thismoon/services/d-man/internal/proxy"
)

var flagServePort int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the reverse proxy and keep /etc/hosts in sync",
	Long: `Serve is the long-running d-man daemon. On start and on every routes.toml
change it (1) rewrites the managed /etc/hosts block and (2) rebuilds the
127.0.0.1:80 reverse proxy that routes by Host header to each backend.

Run as a root launchd daemon via t-man (writing /etc/hosts and binding :80 both
need root):
  sudo t-man --daemon add --name d-man -- d-man serve --config ~/.config/d-man/routes.toml

It also watches its own binary and exits cleanly when the file changes, so a
ralph rebuild is picked up by launchd's KeepAlive relaunch — no manual restart,
no sudo.`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().
		IntVar(&flagServePort, "port", 80, "port the reverse proxy listens on (loopback only)")
	rootCmd.AddCommand(serveCmd)
}

func runServe(_ *cobra.Command, _ []string) error {
	rh := &reloadableHandler{}
	// Fail fast if the very first load is bad — better to crash-loop visibly
	// than to serve nothing.
	if err := reloadOnce(rh); err != nil {
		return err
	}

	addr := fmt.Sprintf("127.0.0.1:%d", flagServePort)
	srv := &http.Server{Addr: addr, Handler: rh}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go watchLoop(ctx, rh)

	go func() {
		<-ctx.Done()
		log.Printf("d-man: shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	log.Printf("d-man: reverse proxy on http://%s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// reloadableHandler lets the watch loop swap in a fresh proxy on config change
// without restarting the HTTP listener.
type reloadableHandler struct {
	mu sync.RWMutex
	h  http.Handler
}

func (rh *reloadableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rh.mu.RLock()
	h := rh.h
	rh.mu.RUnlock()
	h.ServeHTTP(w, r)
}

func (rh *reloadableHandler) set(h http.Handler) {
	rh.mu.Lock()
	rh.h = h
	rh.mu.Unlock()
}

// reloadOnce loads the config, syncs /etc/hosts, and rebuilds the proxy. On any
// error it returns without mutating the running handler, so a bad edit leaves
// the previous good routes and hosts in place.
func reloadOnce(rh *reloadableHandler) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return err
	}
	res, err := hosts.Sync(flagHostsFile, cfg.Hosts())
	if err != nil {
		return err
	}
	for _, warn := range res.Warnings {
		log.Printf("d-man: %s", warn)
	}
	if res.Changed {
		hostNames := cfg.Hosts()
		log.Printf(
			"d-man: %s updated (%d hosts, backup %s)",
			flagHostsFile,
			len(hostNames),
			res.Backup,
		)
		notify.EmitEvent("d-man", "info", "routes reloaded",
			strings.Join(hostNames, ", "),
			map[string]string{"hosts": fmt.Sprintf("%d", len(hostNames))})
	}
	h, err := proxy.New(cfg.RouteMap(), cfg.Sites())
	if err != nil {
		return err
	}
	rh.set(h)
	log.Printf("d-man: routes loaded: %s", strings.Join(cfg.Hosts(), ", "))
	return nil
}

// watchLoop reloads on routes.toml changes and SIGHUP, and exits(0) when the
// running binary is replaced so launchd relaunches the new build.
func watchLoop(ctx context.Context, rh *reloadableHandler) {
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("d-man: file watching disabled (%v); SIGHUP still reloads", err)
		for {
			select {
			case <-ctx.Done():
				return
			case <-hup:
				reloadOrLog(rh)
			}
		}
	}
	defer func() { _ = w.Close() }()

	cfgReal := resolvePath(flagConfig)
	binReal := selfPath()
	watchDirOf(w, cfgReal)
	watchDirOf(w, binReal)

	const debounce = 300 * time.Millisecond
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	var pendCfg, pendBin bool

	for {
		select {
		case <-ctx.Done():
			return
		case <-hup:
			pendCfg = true
			timer.Reset(debounce)
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			switch {
			case binReal != "" && samePath(ev.Name, binReal):
				pendBin = true
				timer.Reset(debounce)
			case cfgReal != "" && samePath(ev.Name, cfgReal):
				pendCfg = true
				timer.Reset(debounce)
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			log.Printf("d-man: watch error: %v", err)
		case <-timer.C:
			if pendBin {
				log.Printf("d-man: binary changed — exiting for launchd to relaunch the new build")
				os.Exit(0)
			}
			if pendCfg {
				pendCfg = false
				reloadOrLog(rh)
			}
		}
	}
}

func reloadOrLog(rh *reloadableHandler) {
	if err := reloadOnce(rh); err != nil {
		log.Printf("d-man: reload failed, keeping current routes/hosts: %v", err)
		notify.EmitEvent("d-man", "error", "routes reload failed", err.Error(), nil)
	}
}

// watchDirOf watches the directory containing file, so edits that arrive via
// editor temp-file + rename (where the inode changes) are still seen.
func watchDirOf(w *fsnotify.Watcher, file string) {
	if file == "" {
		return
	}
	if err := w.Add(filepath.Dir(file)); err != nil {
		log.Printf("d-man: cannot watch %s: %v", filepath.Dir(file), err)
	}
}

// resolvePath follows symlinks (routes.toml is symlinked from the dotfiles
// repo); falls back to the cleaned path if resolution fails.
func resolvePath(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return filepath.Clean(p)
}

// selfPath is the resolved path of the running executable, or "" if unknown.
func selfPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return resolvePath(exe)
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
