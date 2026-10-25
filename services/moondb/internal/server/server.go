// Package server exposes a moondb store over HTTP: a webkit-chromed page, the
// read-only JSON API the CLI and the MCP server call, plus /healthz, /version
// and /webkit/. There is no write route: `moondb record` writes the spool and
// serve drains it, so serve stays the store's single writer. Binding to
// loopback is the caller's concern, not the handler's.
package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/webkit"

	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// shellHTML is the static page shell (chrome only). The sessions and turns
// are rendered client-side by appJS from the JSON under /api/ (ADR-0005):
// the backend serves data, the frontend renders. appJS is served at
// GET /app.js.
//
//go:embed shell.html
var shellHTML []byte

//go:embed app.js
var appJS []byte

// Querier answers the two questions the store exists for. *query.Service
// satisfies it; tests pass a fake.
type Querier interface {
	Why(ctx context.Context, req query.WhyRequest) (query.WhyResult, error)
	History(ctx context.Context, req query.HistoryRequest) ([]store.Edit, error)
}

// Server serves one moondb store, read-only.
type Server struct {
	store store.Store
	query Querier
	info  buildinfo.Info
	now   func() time.Time
}

// New returns a Server reading st, answering why and history through q, and
// reporting info on /version.
func New(st store.Store, q Querier, info buildinfo.Info) *Server {
	return &Server{
		store: st,
		query: q,
		info:  info,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// Handler builds the routes, wrapped in request logging.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /app.js", s.handleAppJS)
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleSession)
	mux.HandleFunc("GET /api/turns/{id}", s.handleTurn)
	mux.HandleFunc("GET /api/edits", s.handleEdits)
	mux.HandleFunc("GET /api/why", s.handleWhy)
	mux.HandleFunc("GET /api/history", s.handleHistory)
	mux.HandleFunc("GET /api/chunks/{sha}", s.handleChunk)
	mux.HandleFunc("GET /api/", handleNoRoute)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /version", s.info.Handler())
	webkit.Mount(mux)
	return logRequests(mux)
}

// handleIndex serves the static chrome-only shell; app.js fetches /api/ and
// builds the page in the browser.
func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(shellHTML)
}

// handleAppJS serves the client renderer; no-cache so a rebuild's app.js is
// picked up on the next load.
func (s *Server) handleAppJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(appJS)
}

// handleHealthz answers 204 while the store answers Ping, 503 otherwise.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("moondb: store unavailable: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleNoRoute answers an unknown /api/ path with the JSON error shape, so a
// client talking to an older serve reads a message instead of a bare 404 page.
func handleNoRoute(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotFound, fmt.Errorf("moondb: no API route GET %s", r.URL.Path))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr writes the API's one error shape: {"error": "<message>"}.
func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

// writeBackendErr maps a store or query error to its status: a missing record
// is 404, a question this build cannot answer is 501, anything else is 500.
func writeBackendErr(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, store.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, query.ErrNotImplemented):
		code = http.StatusNotImplemented
	}
	writeErr(w, code, fmt.Errorf("moondb: %w", err))
}

// statusRecorder remembers the status a handler wrote, for the request log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// logRequests logs one line per request with its status, so a failing query
// is visible in the service log without a client attached.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf(
			"moondb: %s %s %d (%s)",
			r.Method,
			r.URL.Path,
			rec.status,
			time.Since(start).Round(time.Millisecond),
		)
	})
}
