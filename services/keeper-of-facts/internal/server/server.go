// Package server exposes the kof store over HTTP: a webkit-chromed web page, a
// JSON API the MCP server and CLI call, plus /version and /webkit/. The serve
// process is the single writer of the store and the only one that resolves and
// hashes evidence pins against the working tree.
package server

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/webkit"

	"github.com/mad01/thismoon/services/keeper-of-facts/internal/pin"
	"github.com/mad01/thismoon/services/keeper-of-facts/internal/recall"
	"github.com/mad01/thismoon/services/keeper-of-facts/internal/store"
)

// shellHTML is the static page shell (chrome only). The assertion list is
// rendered client-side by appJS from the JSON at GET /api/assertions — the
// backend serves data, the frontend renders. appJS is served at GET /app.js.
//
//go:embed shell.html
var shellHTML []byte

//go:embed app.js
var appJS []byte

// Server serves the kof store.
type Server struct {
	store  *store.Store
	info   buildinfo.Info
	author string
	now    func() time.Time
	judge  *recall.Judge
}

// New returns a Server backed by st, reporting info on /version and stamping
// author into the provenance of every assertion it creates.
func New(st *store.Store, info buildinfo.Info, author string) *Server {
	return &Server{
		store:  st,
		info:   info,
		author: author,
		now:    func() time.Time { return time.Now().UTC() },
		judge:  recall.NewJudge(),
	}
}

// SetJudge swaps the recall judge, for tests.
func (s *Server) SetJudge(j *recall.Judge) { s.judge = j }

// Handler builds the routes, wrapped in request logging.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /app.js", s.handleAppJS)
	mux.HandleFunc("GET /api/assertions", s.handleList)
	mux.HandleFunc("POST /api/assertions", s.handleCreate)
	mux.HandleFunc("GET /api/assertions/{id}", s.handleGet)
	mux.HandleFunc("POST /api/assertions/{id}/retract", s.handleRetract)
	mux.HandleFunc("POST /api/check", s.handleCheck)
	mux.HandleFunc("POST /api/recall", s.handleRecall)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /version", s.info.Handler())
	webkit.Mount(mux)
	return logRequests(mux)
}

// handleIndex serves the static chrome-only shell; app.js fetches
// GET /api/assertions and builds the list in the browser.
func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(shellHTML)
}

// handleAppJS serves the client renderer; no-cache so a kof rebuild's app.js
// is picked up on the next load.
func (s *Server) handleAppJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(appJS)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := q.Get("kind")
	if kind != "" && !store.ValidKind(kind) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid kind %q", kind))
		return
	}
	status := q.Get("status")
	if status != "" && !store.ValidStatus(status) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid status %q", status))
		return
	}
	as := s.store.List(store.Filter{Subject: q.Get("subject"), Kind: kind, Status: status})
	writeJSON(w, http.StatusOK, map[string]any{"assertions": as})
}

// createReq is the POST /api/assertions body: the assertion fields plus the
// unresolved pin refs the server resolves against the working tree.
type createReq struct {
	Kind       string    `json:"kind"`
	Subject    string    `json:"subject"`
	Statement  string    `json:"statement"`
	Confidence string    `json:"confidence"`
	SessionID  string    `json:"session_id"`
	CostTokens int       `json:"cost_tokens"`
	Links      []string  `json:"links"`
	Pins       []pin.Ref `json:"pins"`
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return
	}
	if len(req.Pins) == 0 {
		writeErr(w, http.StatusBadRequest, errors.New("at least one evidence pin is required"))
		return
	}
	now := s.now()
	pins := make([]pin.Pin, 0, len(req.Pins))
	for _, ref := range req.Pins {
		p, err := pin.Resolve(ref, now)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		pins = append(pins, p)
	}
	a, err := s.store.Assert(store.AssertInput{
		Kind:       req.Kind,
		Subject:    req.Subject,
		Statement:  req.Statement,
		Confidence: req.Confidence,
		Links:      req.Links,
		Author:     s.author,
		SessionID:  req.SessionID,
		CostTokens: req.CostTokens,
		Pins:       pins,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// retractReq carries the counter-evidence note; it is required.
type retractReq struct {
	Note string `json:"note"`
}

func (s *Server) handleRetract(w http.ResponseWriter, r *http.Request) {
	var req retractReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return
	}
	if req.Note == "" {
		writeErr(w, http.StatusBadRequest, errors.New("note is required"))
		return
	}
	a, err := s.store.Retract(r.PathValue("id"), req.Note)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// checkReq is the optional POST /api/check body: an empty or absent id checks
// every non-retracted assertion, a specific id checks just that one.
type checkReq struct {
	ID string `json:"id"`
}

// checkResp mirrors store.CheckReport with the JSON field names the web page
// and MCP client read. store.CheckReport itself carries no json tags.
type checkResp struct {
	Checked    int               `json:"checked"`
	Fresh      int               `json:"fresh"`
	Stale      int               `json:"stale"`
	Flipped    int               `json:"flipped"`
	Assertions []store.Assertion `json:"assertions"`
}

func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	var req checkReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return
	}
	report, err := s.store.Check(req.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, checkResp{
		Checked:    report.Checked,
		Fresh:      report.Fresh,
		Stale:      report.Stale,
		Flipped:    report.Flipped,
		Assertions: report.Assertions,
	})
}

// recallReq is the POST /api/recall body.
type recallReq struct {
	Question string `json:"question"`
}

// handleRecall ranks the store against a question with the model judge. A
// judge failure is a 502 whose message names the kof_query fallback — recall
// degrading must never read as "the store knows nothing".
func (s *Server) handleRecall(w http.ResponseWriter, r *http.Request) {
	var req recallReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return
	}
	if req.Question == "" {
		writeErr(w, http.StatusBadRequest, errors.New("question is required"))
		return
	}
	as, err := s.judge.Rank(r.Context(), req.Question, s.store.List(store.Filter{}))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assertions": as})
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

func writeStoreErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeErr(w, http.StatusBadRequest, err)
}

// logRequests logs one line per request so update problems are visible in
// t-man logs: t-man logs keeper-of-facts --stderr.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf(
			"kof: %s %s (%s)",
			r.Method,
			r.URL.Path,
			time.Since(start).Round(time.Millisecond),
		)
	})
}
