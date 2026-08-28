// Package server exposes the event store over HTTP: a webkit-chromed,
// client-rendered timeline page, a JSON API the MCP server and CLI call, plus
// /version and /webkit/. The serve process is the single writer of the store.
package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/webkit"

	"github.com/mad01/thismoon/services/events/internal/event"
	"github.com/mad01/thismoon/services/events/internal/store"
)

//go:embed index.html
var indexHTML []byte

// Server serves the event store.
type Server struct {
	store *store.Store
	info  buildinfo.Info
}

// New returns a Server backed by st, reporting info on /version.
func New(st *store.Store, info buildinfo.Info) *Server {
	return &Server{store: st, info: info}
}

// Handler builds the routes, wrapped in request logging.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/events", s.handleQuery)
	mux.HandleFunc("POST /api/events", s.handleEmit)
	mux.HandleFunc("DELETE /api/events", s.handlePurge)
	mux.HandleFunc("GET /api/sources", s.handleSources)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /version", s.info.Handler())
	webkit.Mount(mux)
	return logRequests(mux)
}

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// A webkit asset bump soft-reloads the open page on the next navigation
	// instead of waiting for a force-refresh.
	webkit.NoCacheHTML(w)
	_, _ = w.Write(indexHTML)
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.Filter{
		Source: q.Get("source"),
		Level:  q.Get("level"),
		Q:      q.Get("q"),
		Since:  q.Get("since"),
		Before: q.Get("before"),
	}
	if f.Level != "" && !validLevel(f.Level) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid level %q (want info|warn|error)", f.Level))
		return
	}
	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 0 {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid limit %q", l))
			return
		}
		f.Limit = n
	}
	evs := s.store.Query(f)
	if evs == nil {
		evs = []event.Event{}
	}
	writeJSON(w, http.StatusOK, evs)
}

func (s *Server) handleEmit(w http.ResponseWriter, r *http.Request) {
	var ev event.Event
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return
	}
	stored, err := s.store.Append(ev)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": stored.ID})
}

// handlePurge drops events from one source: all of them, or only those with
// ID <= before. Source is required so a stray DELETE can't wipe the store.
func (s *Server) handlePurge(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	source := q.Get("source")
	if source == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("source is required"))
		return
	}
	n, err := s.store.Purge(source, q.Get("before"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"purged": n})
}

func (s *Server) handleSources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Sources())
}

func validLevel(level string) bool {
	switch level {
	case event.LevelInfo, event.LevelWarn, event.LevelError:
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

// logRequests logs one line per request so problems are visible in t-man logs:
// t-man logs events --stderr.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf(
			"events: %s %s (%s)",
			r.Method,
			r.URL.Path,
			time.Since(start).Round(time.Millisecond),
		)
	})
}
