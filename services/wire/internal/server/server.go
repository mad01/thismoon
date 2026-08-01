// Package server exposes the wire store over HTTP: a webkit-chromed web page,
// the JSON API the MCP server and CLI call, an event stream for the live
// transcript, plus /version and /webkit/. The serve process is the single
// writer of the store and the only place a reader can park waiting for a
// message, so every blocking read lands here.
package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mad01/thismoon/webkit"

	"github.com/mad01/thismoon/services/wire/internal/ref"
	"github.com/mad01/thismoon/services/wire/internal/store"
)

// shellHTML is the static page shell (chrome only). The channel list and the
// transcript are rendered client-side by appJS from the JSON API — the backend
// serves data, the frontend renders. appJS is served at GET /app.js.
//
//go:embed shell.html
var shellHTML []byte

//go:embed app.js
var appJS []byte

const (
	// maxWait caps how long one blocking read may hold a connection. A caller
	// asking for more is served the cap rather than refused: an expired wait
	// returns an empty batch, so the client simply asks again.
	maxWait = 120 * time.Second

	// streamKeepalive is how long the event stream blocks before emitting a
	// comment line. Without it an idle stream looks dead to intermediaries.
	streamKeepalive = 25 * time.Second
)

// Server serves the wire store.
type Server struct {
	store   *store.Store
	version string
	// port is the port serve listens on. It is here only to mint connection
	// strings — the token a session hands to another session — which is why
	// the server, not the store, is what stamps them onto a response.
	port int
}

// New returns a Server backed by st, reporting version on /version and minting
// connection strings against port.
func New(st *store.Store, version string, port int) *Server {
	return &Server{store: st, version: version, port: port}
}

// channelView is a channel as the API returns it: the store's summary plus the
// connection string, which the store cannot build because it does not know
// what port it is being served on. Summary is embedded, so its fields stay at
// the top level of the JSON.
type channelView struct {
	store.Summary
	Connect string `json:"connect"`
}

func (s *Server) view(sum store.Summary) channelView {
	return channelView{Summary: sum, Connect: ref.String(s.port, sum.Name)}
}

// Handler builds the routes, wrapped in request logging.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /app.js", s.handleAppJS)
	mux.HandleFunc("GET /api/channels", s.handleList)
	mux.HandleFunc("POST /api/channels", s.handleOpen)
	mux.HandleFunc("GET /api/channels/{ref}", s.handleGet)
	mux.HandleFunc("POST /api/channels/{ref}/close", s.handleClose)
	mux.HandleFunc("GET /api/channels/{ref}/messages", s.handleRead)
	mux.HandleFunc("POST /api/channels/{ref}/messages", s.handlePost)
	mux.HandleFunc("GET /api/channels/{ref}/stream", s.handleStream)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, "{\"version\":%q}\n", s.version)
	})
	webkit.Mount(mux)
	return logRequests(mux)
}

// handleIndex serves the static chrome-only shell; app.js fetches the API and
// builds the page in the browser.
func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	webkit.NoCacheHTML(w)
	_, _ = w.Write(shellHTML)
}

// handleAppJS serves the client renderer; no-cache so a wire rebuild's app.js
// is picked up on the next load.
func (s *Server) handleAppJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(appJS)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") != ""
	sums := s.store.List(all)
	views := make([]channelView, len(sums))
	for i, sum := range sums {
		views[i] = s.view(sum)
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": views})
}

// openReq is the POST /api/channels body. Every field is optional: a channel
// with no name gets a generated one.
type openReq struct {
	Name        string `json:"name"`
	Topic       string `json:"topic"`
	From        string `json:"from"`
	Conventions string `json:"conventions"`
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req openReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	c, err := s.store.Open(store.OpenInput{
		Name:        req.Name,
		Topic:       req.Topic,
		From:        req.From,
		Conventions: req.Conventions,
	})
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	s.writeSummary(w, http.StatusCreated, c.ID)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	s.writeSummary(w, http.StatusOK, r.PathValue("ref"))
}

// closeReq carries an optional parting note explaining why the conversation
// ended.
type closeReq struct {
	Note string `json:"note"`
}

func (s *Server) handleClose(w http.ResponseWriter, r *http.Request) {
	var req closeReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	c, err := s.store.Close(r.PathValue("ref"), req.Note)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	s.writeSummary(w, http.StatusOK, c.ID)
}

// postReq is one message. From and body are required; kind, reply_to, and
// reply_needed are the optional protocol fields.
type postReq struct {
	From        string `json:"from"`
	Body        string `json:"body"`
	Kind        string `json:"kind"`
	ReplyTo     int64  `json:"reply_to"`
	ReplyNeeded bool   `json:"reply_needed"`
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	var req postReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	m, err := s.store.Post(r.PathValue("ref"), store.PostInput{
		From:        req.From,
		Body:        req.Body,
		Kind:        req.Kind,
		ReplyTo:     req.ReplyTo,
		ReplyNeeded: req.ReplyNeeded,
	})
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// handleRead serves both the plain read and the blocking one: `wait` is a
// number of seconds to park before giving up, and an expired wait is a normal
// empty batch rather than an error, so the caller just asks again.
func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, err := intParam(q, "since", 0)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	limit, err := intParam(q, "limit", 0)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	waitSecs, err := intParam(q, "wait", 0)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	batch, err := s.store.Wait(r.Context(), r.PathValue("ref"), since, int(limit), waitFor(waitSecs))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return // the caller hung up mid-wait
		}
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

// handleStream is the same blocking read as a server-sent event stream, which
// is what the web page follows so an open transcript updates itself. Each
// message event carries its sequence number as the SSE id, so a browser
// reconnect resumes exactly where it left off via Last-Event-ID.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, errors.New("streaming unsupported"))
		return
	}
	ref := r.PathValue("ref")
	since, err := streamCursor(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.store.Resolve(ref); err != nil {
		writeStoreErr(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	// Ask any proxy in front of us not to buffer, or events arrive in clumps.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		batch, err := s.store.Wait(r.Context(), ref, since, 0, streamKeepalive)
		if err != nil {
			return // the browser navigated away, or the channel vanished
		}
		for _, m := range batch.Messages {
			if werr := writeEvent(w, "message", m.Seq, m); werr != nil {
				return
			}
		}
		since = batch.Cursor
		if len(batch.Messages) == 0 {
			if _, werr := io.WriteString(w, ": keepalive\n\n"); werr != nil {
				return
			}
		}
		if batch.Channel.Closed() {
			_ = writeEvent(w, "closed", 0, batch.Channel)
			flusher.Flush()
			return
		}
		flusher.Flush()
	}
}

// streamCursor resolves where a stream starts: the browser's Last-Event-ID on
// a reconnect, else the `since` query param.
func streamCursor(r *http.Request) (int64, error) {
	if id := r.Header.Get("Last-Event-ID"); id != "" {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid Last-Event-ID %q", id)
		}
		return n, nil
	}
	return intParam(r.URL.Query(), "since", 0)
}

// writeEvent emits one SSE frame. JSON never contains a raw newline, so the
// payload always fits a single data line.
func writeEvent(w io.Writer, event string, id int64, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if id > 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", id); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
	return err
}

// writeSummary responds with a channel and its derived counts, the shape every
// channel-returning endpoint uses.
func (s *Server) writeSummary(w http.ResponseWriter, code int, channel string) {
	sum, err := s.store.Get(channel)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, code, s.view(sum))
}

// waitFor clamps a requested wait to the server's cap.
func waitFor(secs int64) time.Duration {
	if secs <= 0 {
		return 0
	}
	if d := time.Duration(secs) * time.Second; d < maxWait {
		return d
	}
	return maxWait
}

// intParam reads a non-negative integer query param, falling back to def when
// it is absent.
func intParam(q url.Values, key string, def int64) (int64, error) {
	raw := q.Get(key)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid %s %q: want a non-negative integer", key, raw)
	}
	return n, nil
}

// decodeBody reads a JSON request body, treating an empty one as the zero
// value so endpoints whose fields are all optional take no body at all.
func decodeBody(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode body: %w", err)
	}
	return nil
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

// writeStoreErr maps a store error to its status: a missing channel is a 404,
// a name clash or a closed channel is a 409, everything else is a bad request.
func writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, err)
	case errors.Is(err, store.ErrNameTaken), errors.Is(err, store.ErrClosed):
		writeErr(w, http.StatusConflict, err)
	default:
		writeErr(w, http.StatusBadRequest, err)
	}
}

// logRequests logs one line per request so problems are visible in t-man logs:
// t-man logs wire --stderr. Blocking reads are logged with their duration,
// which is how a stuck waiter shows up.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("wire: %s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
