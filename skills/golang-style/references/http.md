# HTTP service patterns

The shape every `net/http` server in this repo shares (`reminder`, `present`, `d-man`, `status`). Stdlib only — no chi/gin/echo. Backed by the `net/http` docs and the repo servers.

## ServeMux with Go 1.22 method routing

Build routes on `http.NewServeMux` using method+path patterns (`"GET /path"`, `"POST /path"`) and `{id}` wildcards read back with `r.PathValue("id")`. Wrap the mux in middleware and return it from a `Handler()` method. From `reminder/internal/server/server.go`:

```go
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/reminders", s.handleList)
	mux.HandleFunc("POST /api/reminders", s.handleCreate)
	mux.HandleFunc("GET /api/reminders/{id}", s.handleGet)
	mux.HandleFunc("PUT /api/reminders/{id}", s.handleUpdate)
	mux.HandleFunc("DELETE /api/reminders/{id}", s.handleDelete)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /version", s.info.Handler())
	mux.Handle("GET /webkit/", webkit.Handler())
	return logRequests(mux)
}
```

`GET /{$}` matches exactly `/` (not a catch-all). webkit-chromed tools mount shared UI at `GET /webkit/` and expose `GET /version`, which serves the same four-key build metadata object the CLI prints for `version -o json` (`version`, `commit`, `tag`, `build_time`; see `cli.md`), pretty-printed, `application/json`, `Cache-Control: no-store`:

```json
{
  "version": "9f3c1ab",
  "commit": "9f3c1abf20e4c7d1b8a5e6003f2c9d47a1b6e850",
  "tag": "present/v1.2.3",
  "build_time": "2026-08-13T19:40:02Z"
}
```

The `version` key stays a bare sha: status compares it against the installed binary's own token to catch a service left running on an old build. Serving it is one line: `mux.HandleFunc("GET /version", s.info.Handler())`. Build metadata never changes while the process runs, so the handler renders the body once, at wiring time.

## The `Server` struct and `New`

A `Server` holds its dependencies (the store, its build metadata, and an injectable `now`), and `New` returns a concrete `*Server`. Taking `buildinfo.Info` as a parameter rather than reading the package vars is what lets a test pin a build:

```go
type Server struct {
	store *store.Store
	info  buildinfo.Info
	now   func() time.Time
}

func New(st *store.Store, info buildinfo.Info) *Server {
	return &Server{store: st, info: info, now: func() time.Time { return time.Now().UTC() }}
}
```

## JSON write helpers

Two small helpers keep every handler consistent — one writes JSON, one writes an error as JSON:

```go
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
```

`Cache-Control: no-store` matters for live-reloading pages — it stops a browser caching a page that an MCP update just changed.

## Map store errors to status with a sentinel helper

Don't scatter status codes through handlers. One helper turns a store sentinel into the right code (see `errors.md`):

```go
func writeStoreErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeErr(w, http.StatusBadRequest, err)
}
```

Handlers decode, validate, call the store, and on error call `writeStoreErr(w, err)` then `return`.

## Optional fields in request bodies as pointers

A PUT/PATCH body uses `*string` fields so an omitted field is left unchanged (mirrors the store `Patch` — see `functions.md`):

```go
type updateReq struct {
	Title  *string `json:"title"`
	Body   *string `json:"body"`
	Due    *string `json:"due"`
	Repeat *string `json:"repeat"`
}
```

## Request-logging middleware

One middleware logs a line per request so problems show up in `t-man logs <tool> --stderr`:

```go
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("reminder: %s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
```

Logging is plain `log.Printf` with a `tool:` prefix; slog and structured loggers stay out.

## Graceful shutdown on signal

Servers run under a `signal.NotifyContext` and shut down with a timeout, ignoring the expected `http.ErrServerClosed`. From thismoon `services/d-man/internal/cli/serve.go`:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

go func() {
	<-ctx.Done()
	log.Printf("d-man: shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}()

if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
	return err
}
return nil
```

A server that hot-reloads config swaps its handler behind a `sync.RWMutex` (`d-man`'s `reloadableHandler`) instead of restarting the listener.
