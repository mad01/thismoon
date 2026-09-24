// Package server exposes stored presentations over HTTP as single-page views.
// It reads the store and core template fresh on every request, so both content
// updates and template edits are reflected immediately without a restart.
package server

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/baseurl"
	"github.com/mad01/thismoon/services/present/internal/sharedclient"
	"github.com/mad01/thismoon/services/present/internal/store"
	"github.com/mad01/thismoon/webkit"
)

// shellHTML is the static page shell (chrome only). The page body is rendered
// client-side by appJS from the JSON at GET /api/p/{id} — the backend serves
// data, the frontend renders. appJS is served at GET /app.js.
//
//go:embed shell.html
var shellHTML []byte

// pageShell is shellHTML for the mode. A shared instance has no local .this
// sites for the ⌘K picker to jump between and no index for the back link to
// return to, so its shell leaves the cmdk control out (webkit turns the
// shortcut off with it) and points the link at the how-to root. One shell
// file, two substitutions, both pinned by a test.
func pageShell(mode Mode) []byte {
	if mode != ModeShared {
		return shellHTML
	}
	out := bytes.Replace(shellHTML, []byte(`controls="cmdk,`), []byte(`controls="`), 1)
	return bytes.Replace(out, []byte(`back-label="← All"`), []byte(`back-label="← About"`), 1)
}

//go:embed app.js
var appJS []byte

// indexShellHTML is the static index shell (chrome only). The page list is
// rendered client-side by indexJS from the JSON at GET /api/pages. indexJS is
// served at GET /index.js.
//
//go:embed index_shell.html
var indexShellHTML []byte

//go:embed index.js
var indexJS []byte

// Mode selects what the server exposes.
type Mode int

const (
	// ModeLocal is present on one machine: an index of every page, listing
	// and delete without authentication, bound to loopback.
	ModeLocal Mode = iota
	// ModeShared is a network-facing instance: pages by id only, a how-to
	// page instead of an index, author keys on every write, and the MCP
	// server over HTTP at /mcp.
	ModeShared
)

// Server serves the index and individual presentation pages.
type Server struct {
	store   store.Store
	mode    Mode
	workdir string
	info    buildinfo.Info
	baseURL string
	now     func() time.Time
	mcp     http.Handler
	sharer  *sharedclient.Client
	shell   []byte

	watcher      PageWatcher
	heartbeat    time.Duration
	streamsDone  chan struct{}
	closeStreams sync.Once
}

// Options configures a Server. Workdir is where the served assets live;
// Info is present's own build metadata, reported on GET /version. BaseURL
// is the display override for the URLs shared writes return; empty means
// derive it from each request's forwarded headers. MCP, when set in shared
// mode, is mounted at /mcp. Sharer, in local mode, is the shared instance
// pages can be pushed to; nil hides the share button and its endpoint.
// Watcher, when set, mounts GET /p/{id}/events, which pushes a page's version
// to open tabs as it changes; without it tabs poll /p/{id}/version. Heartbeat
// is how often an idle event stream resends the version, DefaultHeartbeat
// when zero. Now defaults to time.Now.
type Options struct {
	Mode      Mode
	Workdir   string
	Info      buildinfo.Info
	BaseURL   string
	Now       func() time.Time
	MCP       http.Handler
	Sharer    *sharedclient.Client
	Watcher   PageWatcher
	Heartbeat time.Duration
}

// New returns a Server backed by the given store.
func New(st store.Store, opts Options) *Server {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	heartbeat := opts.Heartbeat
	if heartbeat <= 0 {
		heartbeat = DefaultHeartbeat
	}
	return &Server{
		watcher:     opts.Watcher,
		heartbeat:   heartbeat,
		streamsDone: make(chan struct{}),
		store:       st,
		mode:        opts.Mode,
		workdir:     opts.Workdir,
		info:        opts.Info,
		baseURL:     opts.BaseURL,
		now:         now,
		mcp:         opts.MCP,
		sharer:      opts.Sharer,
		shell:       pageShell(opts.Mode),
	}
}

// Handler builds the HTTP routes for the server's mode, wrapped in
// forwarded-header defaults and request logging. The page view, its JSON,
// the version poll, delete, assets, and webkit are common, and so is the
// version event stream whenever a watcher is configured; local mode adds
// the index, its listing, and the markdown import; shared mode the how-to
// root, the write API, whoami, and the MCP endpoint. A route the mode does
// not register is a plain 404.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /p/{id}", s.handlePage)
	mux.HandleFunc("GET /api/p/{id}", s.handleAPIPage)
	mux.HandleFunc("GET /app.js", handleAppJS)
	mux.HandleFunc("DELETE /p/{id}", s.handleDelete)
	mux.HandleFunc("GET /p/{id}/version", s.handleVersion)
	if s.watcher != nil {
		mux.HandleFunc("GET /p/{id}/events", s.handleEvents)
	}
	mux.HandleFunc("GET /version", s.info.Handler())
	mux.Handle("GET /assets/", s.assetsHandler())
	webkit.Mount(mux)
	switch s.mode {
	case ModeShared:
		mux.HandleFunc("GET /{$}", s.handleHowTo)
		mux.HandleFunc("POST /api/pages", s.handleSharedCreate)
		mux.HandleFunc("PUT /api/p/{id}", s.handleSharedReplace)
		mux.HandleFunc("GET /api/whoami", s.handleWhoAmI)
		if s.mcp != nil {
			mux.Handle("/mcp", s.mcp)
		}
	default:
		mux.HandleFunc("GET /{$}", s.handleIndex)
		mux.HandleFunc("GET /api/pages", s.handleAPIPages)
		mux.HandleFunc("GET /index.js", handleIndexJS)
		mux.HandleFunc("POST /api/import", s.handleImport)
		if s.sharer != nil {
			mux.HandleFunc("POST /p/{id}/share", s.handleShare)
		}
	}
	return logRequests(baseurl.Middleware(mux))
}

// pageURL is the public URL of a page for the caller of r: the display
// override when configured, else the origin the request arrived on.
func (s *Server) pageURL(r *http.Request, id string) string {
	base := baseurl.FromHeader(r.Header, s.baseURL)
	if base == "" {
		base = s.baseURL
	}
	return base + "/p/" + id
}

// statusRecorder captures the response status code and byte count for access
// logging. status defaults to 200 because a handler that writes a body without
// calling WriteHeader implicitly sends 200. Unwrap lets http.ResponseController
// reach the underlying writer, preserving Flusher/Hijacker through the wrapper.
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

// probeAgent prefixes the User-Agent kubelet sends for readiness and
// liveness probes (kube-probe/1.31 and the like).
const probeAgent = "kube-probe/"

// logRequests logs one line per request: method, path, status, bytes, duration.
// Without this the serve daemon emits only its startup line, leaving update
// problems (404s, wrong workdir, version mismatches) invisible in t-man logs.
// Kubelet probes are the exception: they hit the instance every few seconds
// and would bury every real request. The filter is on the prober's User-Agent
// rather than on the path, because a human or present doctor asking the same
// endpoint is a request worth seeing.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.UserAgent(), probeAgent) {
			return
		}
		log.Printf("present: %s %s -> %d %dB (%s)",
			r.Method, r.URL.Path, rec.status, rec.bytes, time.Since(start).Round(time.Millisecond))
	})
}

// Index pagination defaults and bounds.
const (
	defaultPageSize = 50
	minPageSize     = 1
	maxPageSize     = 200
)

// handleIndex serves the static index shell. The browser fetches the page list
// from GET /api/pages and renders it client-side via /index.js.
func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// no-cache (vs the brief pages' no-store): allow the browser to keep a copy
	// but force revalidation, so a new page added to the index shows on reload.
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(indexShellHTML)
}

// apiPageMeta is the JSON shape of a single index entry.
type apiPageMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updated_at"`
}

// apiPages is the paginated index payload the client renders.
type apiPages struct {
	Pages      []apiPageMeta `json:"pages"`
	Page       int           `json:"page"`
	Size       int           `json:"size"`
	Total      int           `json:"total"`
	TotalPages int           `json:"total_pages"`
	HasPrev    bool          `json:"has_prev"`
	HasNext    bool          `json:"has_next"`
	PrevPage   int           `json:"prev_page"`
	NextPage   int           `json:"next_page"`
}

// handleAPIPages returns one window of the page listing as JSON, newest first.
// It carries the pagination state the client needs to render the prev/next
// controls and the total count.
func (s *Server) handleAPIPages(w http.ResponseWriter, r *http.Request) {
	pages, err := s.store.ListMeta(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	size := clampSize(queryInt(r, "size", defaultPageSize))
	total := len(pages)
	totalPages := 1
	if total > 0 {
		totalPages = (total + size - 1) / size
	}
	page := clampPage(queryInt(r, "page", 1), totalPages)

	offset := (page - 1) * size
	end := offset + size
	if end > total {
		end = total
	}
	window := pages[offset:end]

	out := apiPages{
		Pages:      make([]apiPageMeta, 0, len(window)),
		Page:       page,
		Size:       size,
		Total:      total,
		TotalPages: totalPages,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
		PrevPage:   page - 1,
		NextPage:   page + 1,
	}
	for _, p := range window {
		out.Pages = append(out.Pages, apiPageMeta{
			ID:        p.ID,
			Title:     p.Title,
			Version:   p.Version,
			UpdatedAt: p.UpdatedAt.Format("2006-01-02 15:04 MST"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		log.Printf("present: encode api pages: %v", err)
	}
}

// handleIndexJS serves the embedded index client renderer.
func handleIndexJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	// no-cache so a present rebuild's index.js is picked up on the next load.
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(indexJS)
}

// queryInt reads a 1-based integer query parameter, returning def when the
// parameter is absent or unparseable.
func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func clampSize(size int) int {
	if size < minPageSize {
		return minPageSize
	}
	if size > maxPageSize {
		return maxPageSize
	}
	return size
}

func clampPage(page, totalPages int) int {
	if page < 1 {
		return 1
	}
	if page > totalPages {
		return totalPages
	}
	return page
}

// handlePage serves the static shell for an existing page. The browser fetches
// the page data from GET /api/p/{id} and renders the body client-side. We still
// resolve the id here so an unknown page is a 404 rather than an empty shell.
func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	if _, err := s.store.Get(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// no-store: the version poll triggers location.reload() on a bump; a cached
	// document would keep the stale embedded version and reload forever.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.shell)
}

// apiReference is the JSON shape of a reference in the page API.
type apiReference struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// apiPage is the page data the frontend renders. Content is the stored,
// authoring-time-compiled HTML body fragment; Graph is the stored Cytoscape
// init script. The frontend mounts Content and executes Graph.
type apiPage struct {
	ID         string         `json:"id"`
	Title      string         `json:"title"`
	Version    int            `json:"version"`
	HasGraph   bool           `json:"has_graph"`
	Content    string         `json:"content"`
	Graph      string         `json:"graph"`
	References []apiReference `json:"references"`
	Share      apiShare       `json:"share"`
}

// handleAPIPage returns a page as JSON for client-side rendering.
func (s *Server) handleAPIPage(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := apiPage{
		ID:         p.ID,
		Title:      p.Title,
		Version:    p.Version,
		HasGraph:   p.HasGraph,
		Content:    p.Content,
		Graph:      p.Graph,
		References: make([]apiReference, 0, len(p.References)),
		Share:      s.shareState(p),
	}
	for _, ref := range p.References {
		out.References = append(out.References, apiReference{Title: ref.Title, URL: ref.URL})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		log.Printf("present: encode api page: %v", err)
	}
}

// handleAppJS serves the embedded client renderer.
func handleAppJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	// no-cache so a present rebuild's app.js is picked up on the next load.
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(appJS)
}

// handleDelete removes a page. Local mode trusts the caller (the web index
// behind a confirm dialog on loopback); shared mode requires the author's
// key, so 401/403 come before the store is touched. Deleting a local page
// that was shared removes its copy from the shared instance first, so the
// two never disagree about whether the page still exists.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Look up the title before deleting so the event carries it; best-effort
	// locally, load-bearing in shared mode where the author check needs it.
	title := ""
	p, err := s.store.Get(r.Context(), id)
	if err == nil {
		title = p.Title
	}
	if s.mode == ModeShared {
		if err != nil {
			writeStoreError(w, r, err)
			return
		}
		if err := author.Check(p.Author, r.Header); err != nil {
			writeAuthError(w, err)
			return
		}
	} else if err == nil {
		if err := s.dropSharedCopy(r, p); err != nil {
			// The shared instance refused or is unreachable. Keep the local
			// page: deleting it now would strand the copy with no record of
			// where it is.
			http.Error(w, "unshare failed: "+err.Error(), http.StatusBadGateway)
			return
		}
	}
	err = s.store.Delete(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	notify.EmitEvent("present", "info", "page deleted: "+title, "",
		map[string]string{"id": id, "title": title})
}

// dropSharedCopy removes a local page's copy from the shared instance
// before the local page goes away. A page that was never shared is nothing
// to do. Without a sharer configured this process cannot reach the
// instance, so the copy stays where it is and the log says where to find
// it; `present unshare` with the instance configured is the way to remove
// it later.
func (s *Server) dropSharedCopy(r *http.Request, p store.Page) error {
	if p.Shared == nil {
		return nil
	}
	if s.sharer == nil {
		log.Printf(
			"present: deleting local page %s; its shared copy at %s is left in place "+
				"(no shared instance configured here)",
			p.ID, p.Shared.URL,
		)
		return nil
	}
	if err := sharedclient.Unshare(r.Context(), s.store, s.sharer, p.ID); err != nil {
		return fmt.Errorf("remove the shared copy at %s: %w", p.Shared.URL, err)
	}
	return nil
}

// handleVersion answers the live-reload poll every open tab makes. It reads
// metadata only, so a poll never loads the page's bodies, and on a cached
// store it never reaches the backend at all.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetMeta(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "%d", p.Version)
}

func (s *Server) assetsHandler() http.Handler {
	root := filepath.Join(s.workdir, "assets")
	fs := http.StripPrefix("/assets/", http.FileServer(http.Dir(root)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fs.ServeHTTP(w, r)
	})
}
