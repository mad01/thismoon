// Package server runs the status HTTP server: a poller (Monitor) probing
// every t-man-managed service, and handlers rendering the dashboard.
package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/mad01/thismoon/webkit"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/status/internal/discover"
	"github.com/mad01/thismoon/services/status/internal/history"
)

// shellHTML is the static page shell (chrome only). The dashboard body is
// rendered client-side by appJS from the JSON at GET /api/status — the backend
// serves data, the frontend renders. appJS is served at GET /app.js.
//
//go:embed shell.html
var shellHTML []byte

//go:embed app.js
var appJS []byte

// Options configures Serve. Zero values get defaults.
type Options struct {
	Port         int
	Interval     time.Duration
	MetaInterval time.Duration
	Workdir      string
	RoutesPath   string
	AgentsDir    string
	DaemonsDir   string
	Info         buildinfo.Info
	HistoryDays  int // days shown on the page
	KeepDays     int // days kept in the history file

	RestartWindow    time.Duration // window for counting launchd respawns
	RestartThreshold int           // respawns within the window that trigger an alert
	RestartCooldown  time.Duration // minimum gap between alerts for the same service
}

func (o *Options) defaults() {
	if o.Interval <= 0 {
		o.Interval = time.Minute
	}
	if o.MetaInterval <= 0 {
		o.MetaInterval = 10 * time.Minute
	}
	if o.HistoryDays <= 0 {
		o.HistoryDays = 30
	}
	if o.KeepDays <= 0 {
		o.KeepDays = 90
	}
	if o.RestartWindow <= 0 {
		o.RestartWindow = time.Hour
	}
	if o.RestartThreshold <= 0 {
		o.RestartThreshold = 50
	}
	if o.RestartCooldown <= 0 {
		o.RestartCooldown = 6 * time.Hour
	}
	if o.AgentsDir == "" || o.DaemonsDir == "" {
		agents, daemons := discover.DefaultDirs()
		if o.AgentsDir == "" {
			o.AgentsDir = agents
		}
		if o.DaemonsDir == "" {
			o.DaemonsDir = daemons
		}
	}
}

// NewMux wires the handlers around a Monitor, reporting info on /version.
func NewMux(m *Monitor, info buildinfo.Info) *http.ServeMux {
	mux := http.NewServeMux()
	webkit.Mount(mux)

	// The page serves a static chrome-only shell; app.js fetches GET /api/status
	// and builds the dashboard in the browser.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(shellHTML)
	})

	mux.HandleFunc("GET /app.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		// no-cache so a status rebuild's app.js is picked up on the next load.
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(appJS)
	})

	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(m.Snapshot())
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /version", info.Handler())

	return mux
}

// Serve loads the history, starts the poller, and runs the HTTP server on
// 127.0.0.1:<port>.
func Serve(opts Options) error {
	opts.defaults()
	store, err := history.Load(filepath.Join(opts.Workdir, "history.json"))
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}
	m := newMonitor(opts, store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.run(ctx)

	mux := NewMux(m, opts.Info)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		log.Printf("%s %s", r.Method, r.URL.Path)
	})
	addr := fmt.Sprintf("127.0.0.1:%d", opts.Port)
	log.Printf(
		"status: serving on http://%s (interval %s, workdir %s)",
		addr,
		opts.Interval,
		opts.Workdir,
	)
	return http.ListenAndServe(addr, handler)
}
