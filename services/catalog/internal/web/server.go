package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/catalog/internal/catalog"
	"github.com/mad01/thismoon/webkit"
)

// Server serves the catalog web UI and JSON API. The catalog is held behind a
// read-write mutex so the Refresh endpoint can swap in a freshly scanned copy
// while requests are in flight. Pages and assets are embedded in the binary.
type Server struct {
	mu           sync.RWMutex
	cat          *catalog.Catalog
	roots        []string // source paths; also the allowed roots for UI writes
	registryPath string
	info         buildinfo.Info
}

// New builds a Server, loading the initial catalog from registryPath and
// reporting info on /version. A missing registry file is not fatal: the server
// starts with an empty catalog so a fresh install gets a working UI instead of
// a crash loop under a service manager. Once the registry exists, Refresh (or
// a restart) loads it.
func New(ctx context.Context, registryPath string, info buildinfo.Info) (*Server, error) {
	s := &Server{registryPath: registryPath, info: info}
	if err := s.Reload(ctx); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		log.Printf(
			"catalog web: registry %s not found; serving an empty catalog (create it with a 'sources' list, then Refresh)",
			registryPath,
		)
		s.cat = catalog.NewCatalog(nil)
	}
	return s, nil
}

// newServerWithCatalog builds a Server around an in-memory catalog, for tests
// that should not touch a registry file. roots constrains UI writes.
func newServerWithCatalog(cat *catalog.Catalog, roots []string) *Server {
	return &Server{cat: cat, roots: roots}
}

// Reload re-reads the registry, re-scans every source, and atomically swaps in
// the new catalog.
func (s *Server) Reload(ctx context.Context) error {
	cat, reg, err := catalog.Load(ctx, s.registryPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cat = cat
	s.roots = reg.Paths()
	s.mu.Unlock()
	return nil
}

// snapshot returns the current catalog and write roots under a read lock.
func (s *Server) snapshot() (*catalog.Catalog, []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cat, s.roots
}

// Handler builds the HTTP routes, wrapped in request logging.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// SPA shell: the same page renders list, system and component views; the
	// client reads the path and calls the API.
	page := s.handlePage("index.html")
	mux.HandleFunc("GET /{$}", page)
	mux.HandleFunc("GET /systems/{name}", page)
	mux.HandleFunc("GET /components/{name}", page)

	// JSON API.
	mux.HandleFunc("GET /api/entities", s.handleEntities)
	mux.HandleFunc("GET /api/systems", s.handleSystems)
	mux.HandleFunc("GET /api/systems/{name}", s.handleSystem)
	mux.HandleFunc("GET /api/components", s.handleComponents)
	mux.HandleFunc("GET /api/components/{name}", s.handleComponent)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/owners", s.handleOwners)
	mux.HandleFunc("POST /api/refresh", s.handleRefresh)
	mux.HandleFunc("POST /api/entities", s.handleAdd)

	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /version", s.info.Handler())
	mux.Handle("GET /assets/", assetsHandler())
	webkit.Mount(mux)
	return logRequests(mux)
}

// handlePage serves an embedded HTML page by file name.
func (s *Server) handlePage(name string) http.HandlerFunc {
	body, err := pageBytes(name)
	return func(w http.ResponseWriter, _ *http.Request) {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(body)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

// statusRecorder captures the response status and byte count for access logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// logRequests logs one line per request: method, path, status, bytes, duration.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("catalog web: %s %s -> %d %dB (%s)",
			r.Method, r.URL.Path, rec.status, rec.bytes, time.Since(start).Round(time.Millisecond))
	})
}
