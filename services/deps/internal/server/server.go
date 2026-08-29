// Package server exposes the dependency scan over HTTP: a webkit-chromed web
// page, a JSON API the CLI and MCP call, plus /version and /webkit/. The serve
// process is the single writer of the store and the only one that reaches OSV.
package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/webkit"

	"github.com/mad01/thismoon/services/deps/internal/api"
	"github.com/mad01/thismoon/services/deps/internal/scanner"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// shellHTML is the static page shell (chrome only). The page body is rendered
// client-side by appJS from the JSON at GET /api/deps — the backend serves
// data, the frontend renders. appJS is served at GET /app.js.
//
//go:embed shell.html
var shellHTML []byte

//go:embed app.js
var appJS []byte

// Server serves the dependency scan store and drives on-demand scans.
type Server struct {
	store  *store.Store
	engine *scanner.Engine
	info   buildinfo.Info
}

// New returns a Server backed by st and engine, reporting info on /version.
func New(st *store.Store, engine *scanner.Engine, info buildinfo.Info) *Server {
	return &Server{store: st, engine: engine, info: info}
}

// Handler builds the routes, wrapped in request logging.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /app.js", handleAppJS)
	mux.HandleFunc("GET /api/deps", s.handleDeps)
	mux.HandleFunc("GET /api/flagged", s.handleFlagged)
	mux.HandleFunc("POST /api/scan", s.handleScan)
	mux.HandleFunc("POST /api/check", s.handleCheck)
	mux.HandleFunc("POST /api/resolve", s.handleResolve)
	mux.HandleFunc("POST /api/notify", s.handleNotify)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /version", s.info.Handler())
	webkit.Mount(mux)
	return logRequests(mux)
}

// handleIndex serves the static chrome-only shell; app.js fetches GET /api/deps
// and builds the page in the browser.
func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	webkit.NoCacheHTML(w)
	_, _ = w.Write(shellHTML)
}

func handleAppJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	// no-cache so a deps rebuild's app.js is picked up on the next load.
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(appJS)
}

func (s *Server) handleDeps(w http.ResponseWriter, _ *http.Request) {
	sc := s.store.Snapshot()
	deps := api.Enrich(sc.Deps, s.store.IsResolved)
	writeJSON(w, http.StatusOK, map[string]any{
		"scanned_at":     sc.ScannedAt,
		"total":          len(deps),
		"flagged_count":  api.ActiveCount(api.Flagged(deps)),
		"deps":           deps,
		"resolved_fixed": s.store.ResolvedFixed(),
	})
}

func (s *Server) handleFlagged(w http.ResponseWriter, _ *http.Request) {
	sc := s.store.Snapshot()
	s.writeFlagged(
		w,
		sc.ScannedAt,
		len(sc.Deps),
		api.Flagged(api.Enrich(s.store.Flagged(), s.store.IsResolved)),
	)
}

func (s *Server) handleScan(w http.ResponseWriter, _ *http.Request) {
	deps, err := s.engine.Scan()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"scanned_at":   s.store.Snapshot().ScannedAt,
		"total":        len(deps),
		"by_ecosystem": byEcosystem(deps),
	})
}

// handleCheck runs a full check, or — when a `repo` query param is given — a
// single-repo rescan merged into the store.
func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	var err error
	if repo := r.URL.Query().Get("repo"); repo != "" {
		_, err = s.engine.CheckRepo(r.Context(), repo)
	} else {
		_, err = s.engine.Check(r.Context())
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// Report the full current flagged set so the caller sees the merged picture.
	sc := s.store.Snapshot()
	s.writeFlagged(
		w,
		sc.ScannedAt,
		len(sc.Deps),
		api.Flagged(api.Enrich(s.store.Flagged(), s.store.IsResolved)),
	)
}

// resolveReq is the POST /api/resolve body: the advisory keys to acknowledge.
type resolveReq struct {
	Keys []string `json:"keys"`
}

func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	var req resolveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return
	}
	if len(req.Keys) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no keys provided"))
		return
	}
	resolved, err := s.store.Resolve(req.Keys)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if resolved > 0 {
		notify.EmitEvent("deps", "info",
			fmt.Sprintf("%d advisory(ies) acknowledged", resolved),
			strings.Join(req.Keys, ", "),
			map[string]string{"resolved": fmt.Sprintf("%d", resolved)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"resolved": resolved})
}

func (s *Server) handleNotify(w http.ResponseWriter, _ *http.Request) {
	n, err := s.engine.Notify()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"delivered": n})
}

// writeFlagged is the common flagged-list response (used by /api/flagged and
// /api/check): flagged_count is the *active* (unresolved) count.
func (s *Server) writeFlagged(
	w http.ResponseWriter,
	scannedAt any,
	total int,
	flagged []api.Dependency,
) {
	writeJSON(w, http.StatusOK, map[string]any{
		"scanned_at":    scannedAt,
		"total":         total,
		"flagged_count": api.ActiveCount(flagged),
		"flagged":       flagged,
	})
}

func byEcosystem(deps []store.Dependency) map[string]int {
	counts := map[string]int{}
	for _, d := range deps {
		counts[d.Ecosystem]++
	}
	return counts
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

// logRequests logs one line per request so scan problems are visible in t-man
// logs: t-man logs deps --stderr.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf(
			"deps: %s %s (%s)",
			r.Method,
			r.URL.Path,
			time.Since(start).Round(time.Millisecond),
		)
	})
}
