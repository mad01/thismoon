// Package server exposes the PR cache over HTTP: a webkit-chromed web page,
// the JSON API the MCP server and CLI call, plus /version and /webkit/. The
// serve process is the single writer of the store and the only one that
// talks to the GitHub hosts.
package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/webkit"

	"github.com/mad01/thismoon/services/prs/internal/config"
	"github.com/mad01/thismoon/services/prs/internal/poller"
	"github.com/mad01/thismoon/services/prs/internal/store"
)

// shellHTML is the static page shell (chrome only). The PR list is rendered
// client-side by appJS from the JSON at GET /api/prs — the backend serves
// data, the frontend renders. appJS is served at GET /app.js.
//
//go:embed shell.html
var shellHTML []byte

//go:embed app.js
var appJS []byte

// Refresher runs one synchronous poll cycle; *poller.Poller satisfies it.
type Refresher interface {
	Refresh(ctx context.Context) (poller.Summary, error)
}

// Server serves the PR cache.
type Server struct {
	store     *store.Store
	refresher Refresher
	cfg       config.Config
	info      buildinfo.Info
}

// New returns a Server backed by st, forcing refreshes through refresher and
// reporting info on /version.
func New(st *store.Store, refresher Refresher, cfg config.Config, info buildinfo.Info) *Server {
	return &Server{store: st, refresher: refresher, cfg: cfg, info: info}
}

// Handler builds the routes, wrapped in request logging.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /app.js", s.handleAppJS)
	mux.HandleFunc("GET /api/prs", s.handleList)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/refresh", s.handleRefresh)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /version", s.info.Handler())
	webkit.Mount(mux)
	return logRequests(mux)
}

// handleIndex serves the static chrome-only shell; app.js fetches
// GET /api/prs and builds the list in the browser.
func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(shellHTML)
}

// handleAppJS serves the client renderer; no-cache so a prs rebuild's app.js
// is picked up on the next load.
func (s *Server) handleAppJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(appJS)
}

// reviewValues are the review_decision filter values the API accepts.
func reviewValues() []string { return []string{"APPROVED", "CHANGES_REQUESTED"} }

// listResponse is the GET /api/prs payload: the filtered PRs, the dropdown
// facets computed over the whole cache, and the cache status for the footer
// and error callouts.
type listResponse struct {
	PRs    []store.PR   `json:"prs"`
	Facets facets       `json:"facets"`
	Status store.Status `json:"status"`
}

type facets struct {
	Repos   []string `json:"repos"`
	Authors []string `json:"authors"`
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sortOrder := q.Get("sort")
	if sortOrder != "" && !store.ValidSort(sortOrder) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid sort %q; valid: %s",
			sortOrder, strings.Join(store.SortValues(), " | ")))
		return
	}
	review := q.Get("review")
	if review != "" && !validReview(review) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid review %q; valid: %s",
			review, strings.Join(reviewValues(), " | ")))
		return
	}
	prs := s.store.List(store.Filter{
		Repo:           q.Get("repo"),
		Author:         q.Get("author"),
		ReviewDecision: review,
		Sort:           sortOrder,
	})
	repos, authors := s.store.Facets()
	// Empty lists serialize as [], not null — MCP and JS consumers should
	// never need a null guard.
	writeJSON(w, http.StatusOK, listResponse{
		PRs:    orEmpty(prs),
		Facets: facets{Repos: orEmpty(repos), Authors: orEmpty(authors)},
		Status: s.store.Status(),
	})
}

// statusResponse is GET /api/status: the cache status plus the config the
// poller runs with, so "why is repo X missing" is answerable from one call.
type statusResponse struct {
	store.Status
	PollInterval string   `json:"poll_interval"`
	Dirs         []string `json:"dirs,omitempty"`
	HostsAllowed []string `json:"hosts_allowed,omitempty"`
	Exclude      []string `json:"exclude,omitempty"`
	ConfigPath   string   `json:"config_path"`
	ConfigLoaded bool     `json:"config_loaded"`
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{
		Status:       s.store.Status(),
		PollInterval: s.cfg.PollInterval.String(),
		Dirs:         s.cfg.Dirs,
		HostsAllowed: s.cfg.Hosts,
		Exclude:      s.cfg.Exclude,
		ConfigPath:   s.cfg.Path,
		ConfigLoaded: s.cfg.Loaded,
	})
}

// refreshResponse mirrors poller.Summary with the duration made JSON-friendly.
type refreshResponse struct {
	poller.Summary
	DurationMS int64 `json:"duration_ms"`
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	sum, err := s.refresher.Refresh(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, refreshResponse{
		Summary:    sum,
		DurationMS: sum.Duration.Milliseconds(),
	})
}

func validReview(v string) bool { return slices.Contains(reviewValues(), v) }

// orEmpty replaces a nil slice with an empty one, for JSON.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
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

// logRequests logs one line per request so update problems are visible in
// t-man logs: t-man logs prs --stderr.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf(
			"prs: %s %s (%s)",
			r.Method,
			r.URL.Path,
			time.Since(start).Round(time.Millisecond),
		)
	})
}
