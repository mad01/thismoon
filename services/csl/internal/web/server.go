package web

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/csl/internal/refresh"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/webkit"
)

// searcher is the search backend the HTTP handlers depend on. *Service is the
// production implementation; tests inject a fake to exercise the handlers
// without a live index.
type searcher interface {
	Search(ctx context.Context, opts search.SearchOptions) ([]search.Match, error)
	SemanticSearch(ctx context.Context, req SemanticRequest) (SemanticResult, error)
	HybridSearch(ctx context.Context, req HybridRequest) (HybridResult, error)
	Repos() ([]finder.Repo, error)
	ReadFile(repo, file string, start, end int) (*ReadResult, error)
	GitHealth(ctx context.Context) ([]search.GitHealth, error)
}

// refresher is the background-refresh backend the refresh page depends on.
// *refresh.Refresher is the production implementation; tests inject a fake or
// nil (the handlers answer 503 without one).
type refresher interface {
	Status() refresh.Status
	Kick(only string) error
}

// Server serves the code-search web UI and JSON API. Pages and assets are
// embedded in the binary; search and read run through the searcher.
type Server struct {
	svc  searcher
	info buildinfo.Info
	ref  refresher
}

// New returns a Server backed by the given Service, reporting info at
// GET /version and driving manual refreshes through ref (nil disables the
// refresh API).
func New(svc *Service, info buildinfo.Info, ref refresher) *Server {
	return &Server{svc: svc, info: info, ref: ref}
}

// Handler builds the HTTP routes, wrapped in request logging.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handlePage("index.html"))
	mux.HandleFunc("GET /health", s.handlePage("health.html"))
	mux.HandleFunc("GET /file", s.handlePage("file.html"))
	mux.HandleFunc("GET /refresh", s.handlePage("refresh.html"))
	mux.HandleFunc("GET /api/refresh_status", s.handleRefreshStatus)
	mux.HandleFunc("POST /api/refresh", s.handleRefreshKick)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/semantic_search", s.handleSemanticSearch)
	mux.HandleFunc("GET /api/hybrid_search", s.handleHybridSearch)
	mux.HandleFunc("GET /api/read", s.handleRead)
	mux.HandleFunc("GET /api/repos", s.handleRepos)
	mux.HandleFunc("GET /api/repo_health", s.handleRepoHealth)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /version", s.info.Handler())
	mux.Handle("GET /assets/", assetsHandler())
	webkit.Mount(mux)
	return logRequests(mux)
}

// handlePage serves an embedded HTML page by file name.
func (s *Server) handlePage(name string) http.HandlerFunc {
	body, err := pageBytes(name)
	return func(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("csl web: %s %s -> %d %dB (%s)",
			r.Method, r.URL.Path, rec.status, rec.bytes, time.Since(start).Round(time.Millisecond))
	})
}
