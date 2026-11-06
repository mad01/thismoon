// Package server is brain serve: the web UI and a read-only JSON API over
// the configured vaults, as docs/serve.md specifies. It never writes a
// vault, never takes a vault's write lock and never runs git or gh. Its one
// write is a graph layout under the state directory's layout/. Searches
// refresh indexes only through library, which takes the per-vault index
// locks. Binding to loopback is the caller's concern, not the handler's.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/webkit"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Options configures New.
type Options struct {
	// Config is the loaded brain config. Nil means none could be loaded:
	// serve then runs with no vault and reports ConfigErr on /api/doctor.
	Config *config.Config
	// ConfigPath is the config file serve was pointed at, for /api/doctor.
	ConfigPath string
	// ConfigErr is why Config is nil.
	ConfigErr error
	// UI holds shell.html, app.js, graph.js and assets/ (the ui package's
	// Files in the binary, an fstest.MapFS in tests).
	UI fs.FS
	// Info is served on /version.
	Info buildinfo.Info
	// Now dates the stale queue and paces the fingerprint memo; nil is
	// time.Now.
	Now func() time.Time
}

// Server serves the vaults of one config, read-only.
type Server struct {
	cfg        *config.Config
	configPath string
	configErr  error
	lib        *library.Library
	ui         fs.FS
	info       buildinfo.Info
	now        func() time.Time
	stateDir   string
	layoutDir  string

	// mu guards the fingerprint memo and the model, which current
	// rebuilds lazily.
	mu       sync.Mutex
	prints   []vaultPrint
	printsAt time.Time
	model    *model

	// layoutMu serializes layout writes and their pruning.
	layoutMu sync.Mutex
}

// New opens the configured vaults whose directories exist; /api/doctor
// names the others. It fails only when the state directory cannot be
// resolved.
func New(opts Options) (*Server, error) {
	cfg := opts.Config
	if cfg == nil {
		cfg = &config.Config{}
	}
	stateDir, err := brain.StateDir()
	if err != nil {
		return nil, fmt.Errorf("brain serve: state directory: %w", err)
	}
	lib, err := library.Open(library.Options{Config: openable(cfg)})
	if err != nil {
		return nil, fmt.Errorf("brain serve: %w", err)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Server{
		cfg:        cfg,
		configPath: opts.ConfigPath,
		configErr:  opts.ConfigErr,
		lib:        lib,
		ui:         opts.UI,
		info:       opts.Info,
		now:        now,
		stateDir:   stateDir,
		layoutDir:  filepath.Join(stateDir, brain.LayoutDirName),
	}, nil
}

// openable returns cfg with only the vaults whose directory opens, so one
// missing vault does not take the others down with it.
func openable(cfg *config.Config) *config.Config {
	out := *cfg
	out.Vaults = nil
	for _, v := range cfg.Vaults {
		if _, err := vault.Open(v.Path); err == nil {
			out.Vaults = append(out.Vaults, v)
		}
	}
	return &out
}

// route is one pattern of the mux and its handler.
type route struct {
	pattern string
	handler http.HandlerFunc
}

// routes lists every route but /webkit/. Only POST /api/layout writes.
func (s *Server) routes() []route {
	return []route{
		{"GET /{$}", s.handleFile("shell.html")},
		{"GET /app.js", s.handleFile("app.js")},
		{"GET /graph.js", s.handleFile("graph.js")},
		{"GET /assets/{file}", s.handleAsset},
		{"GET /healthz", handleHealthz},
		{"GET /version", s.info.Handler()},
		{"GET /api/vaults", s.handleVaults},
		{"GET /api/state", s.handleState},
		{"GET /api/graph", s.handleGraph},
		{"POST /api/layout", s.handleLayout},
		{"GET /api/notes", s.handleNotes},
		{"GET /api/note", s.handleNote},
		{"GET /api/backlinks", s.handleBacklinks},
		{"GET /api/search", s.handleSearch},
		{"GET /api/stale", s.handleStale},
		{"GET /api/check", s.handleCheck},
		{"GET /api/promotions", s.handlePromotions},
		{"GET /api/doctor", s.handleDoctor},
		{"GET /api/", handleNoRoute},
	}
}

// Handler builds the routes, wrapped in request logging.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, r := range s.routes() {
		mux.HandleFunc(r.pattern, r.handler)
	}
	webkit.Mount(mux)
	return logRequests(mux)
}

// handleFile serves one file of the UI. Scripts and the shell revalidate on
// every load, so a rebuilt binary's UI shows on the next one.
func (s *Server) handleFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.serveUI(w, r, name)
	}
}

// handleAsset serves a file under the UI's assets/.
func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	name := "assets/" + r.PathValue("file")
	if !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}
	s.serveUI(w, r, name)
}

func (s *Server) serveUI(w http.ResponseWriter, r *http.Request, name string) {
	if s.ui == nil {
		http.NotFound(w, r)
		return
	}
	if info, err := fs.Stat(s.ui, name); err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	webkit.NoCacheHTML(w)
	http.ServeFileFS(w, r, s.ui, name)
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

// handleNoRoute answers an unknown /api/ path with the JSON error shape.
func handleNoRoute(w http.ResponseWriter, r *http.Request) {
	writeError(w, &apiError{
		status:  http.StatusNotFound,
		code:    codeNotFound,
		message: "no API route GET " + r.URL.Path,
	})
}

// API error codes, the "code" of {"error":{"code","message"}}.
const (
	codeUsage       = "usage"
	codeAmbiguous   = "ambiguous"
	codeBadQuery    = "invalid_query"
	codeNotFound    = "not_found"
	codeStaleKey    = "stale_key"
	codeTooLarge    = "too_large"
	codeNoSemantic  = "semantic_unavailable"
	codeServerError = "error"
)

// apiError is an error a handler answers with a status and a code.
type apiError struct {
	status  int
	code    string
	message string
}

func (e *apiError) Error() string { return e.message }

func badRequest(format string, args ...any) error {
	return &apiError{
		status:  http.StatusBadRequest,
		code:    codeUsage,
		message: fmt.Sprintf(format, args...),
	}
}

// writeJSON writes v with the headers every /api/ response carries.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		log.Printf("brain: write response: %v", err)
	}
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError answers err in the one error shape, its status and code taken
// from an apiError or from the vault and library sentinels.
func writeError(w http.ResponseWriter, err error) {
	var api *apiError
	if !errors.As(err, &api) {
		api = classify(err)
	}
	writeJSON(w, api.status, errorEnvelope{Error: errorBody{Code: api.code, Message: api.message}})
}

// classify maps the sentinels behind err to a status and code, the way the
// CLI maps them to exit codes.
func classify(err error) *apiError {
	e := &apiError{
		status:  http.StatusInternalServerError,
		code:    codeServerError,
		message: err.Error(),
	}
	switch {
	case errors.Is(err, vault.ErrAmbiguous):
		e.status, e.code = http.StatusBadRequest, codeAmbiguous
	case errors.Is(err, vault.ErrInvalidPath),
		errors.Is(err, vault.ErrUnknownTier),
		errors.Is(err, vault.ErrUnknownVault):
		e.status, e.code = http.StatusBadRequest, codeUsage
	case errors.Is(err, search.ErrInvalidQuery):
		e.status, e.code = http.StatusBadRequest, codeBadQuery
	case errors.Is(err, vault.ErrNotFound), errors.Is(err, fs.ErrNotExist):
		e.status, e.code = http.StatusNotFound, codeNotFound
	case errors.Is(err, library.ErrSemanticDisabled):
		e.status, e.code = http.StatusServiceUnavailable, codeNoSemantic
	}
	return e
}

// logRequests logs one line per request with its status, so a failing call
// shows in the service log without a client attached.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("brain: %s %s %d (%s)",
			r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// statusRecorder captures the status code a handler writes.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
