// Package server exposes the reminder store over HTTP: a webkit-chromed web
// page, a JSON API the MCP server and CLI call, plus /version and /webkit/.
// The serve process is the single writer of the store.
package server

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	kitnotify "github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/services/reminder/internal/notify"
	"github.com/mad01/thismoon/services/reminder/internal/store"
	"github.com/mad01/thismoon/webkit"
)

// shellHTML is the static page shell (chrome only). The reminder list is
// rendered client-side by appJS from the JSON at GET /api/reminders — the
// backend serves data, the frontend renders. appJS is served at GET /app.js.
//
//go:embed shell.html
var shellHTML []byte

//go:embed app.js
var appJS []byte

// Server serves the reminder store.
type Server struct {
	store    *store.Store
	notifier notify.Notifier
	info     buildinfo.Info
	now      func() time.Time
}

// New returns a Server backed by st, reporting info on /version. notifier is
// the same delivery path the ticker uses; the test/fire endpoints reuse it so an
// on-demand notification is identical to a real one.
func New(st *store.Store, info buildinfo.Info, notifier notify.Notifier) *Server {
	return &Server{
		store:    st,
		notifier: notifier,
		info:     info,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// Handler builds the routes, wrapped in request logging.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /app.js", s.handleAppJS)
	mux.HandleFunc("GET /api/reminders", s.handleList)
	mux.HandleFunc("POST /api/reminders", s.handleCreate)
	mux.HandleFunc("GET /api/reminders/{id}", s.handleGet)
	mux.HandleFunc("PUT /api/reminders/{id}", s.handleUpdate)
	mux.HandleFunc("POST /api/reminders/{id}/cancel", s.handleCancel)
	mux.HandleFunc("POST /api/reminders/{id}/test", s.handleTest)
	mux.HandleFunc("POST /api/reminders/{id}/fire", s.handleFire)
	mux.HandleFunc("DELETE /api/reminders/{id}", s.handleDelete)
	mux.HandleFunc("POST /api/test", s.handleTestGlobal)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /version", s.info.Handler())
	webkit.Mount(mux)
	return logRequests(mux)
}

// apiReminder is a reminder plus the derived overdue flag for API responses.
type apiReminder struct {
	store.Reminder
	Overdue bool `json:"overdue"`
}

func (s *Server) toAPI(r store.Reminder) apiReminder {
	return apiReminder{Reminder: r, Overdue: store.Overdue(r, s.now())}
}

// handleIndex serves the static chrome-only shell; app.js fetches
// GET /api/reminders and builds the list in the browser.
func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(shellHTML)
}

// handleAppJS serves the client renderer; no-cache so a reminder rebuild's
// app.js is picked up on the next load.
func (s *Server) handleAppJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(appJS)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "" && !validStatus(status) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid status %q", status))
		return
	}
	rs := s.store.List(status)
	out := make([]apiReminder, 0, len(rs))
	for _, r := range rs {
		out = append(out, s.toAPI(r))
	}
	writeJSON(w, http.StatusOK, map[string]any{"reminders": out})
}

// createReq accepts an absolute due (RFC3339) or a relative "in" duration.
type createReq struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Due    string `json:"due"`
	In     string `json:"in"`
	Repeat string `json:"repeat"`
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return
	}
	due, err := s.resolveDue(req.Due, req.In)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rem, err := s.store.Create(store.CreateInput{
		Title: req.Title, Body: req.Body, Due: due, Repeat: req.Repeat,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	tags := map[string]string{"id": rem.ID, "due": rem.Due.Format(time.RFC3339)}
	if rem.Repeat != "" {
		tags["repeat"] = rem.Repeat
	}
	kitnotify.EmitEvent("reminder", "info", "reminder created: "+rem.Title, rem.Body, tags)
	writeJSON(w, http.StatusCreated, s.toAPI(rem))
}

// resolveDue turns either an RFC3339 due or a Go-duration "in" into an absolute
// time. Exactly one must be set.
func (s *Server) resolveDue(due, in string) (time.Time, error) {
	switch {
	case in != "":
		d, err := time.ParseDuration(in)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid 'in' duration %q: %w", in, err)
		}
		return s.now().Add(d), nil
	case due != "":
		t, err := time.Parse(time.RFC3339, due)
		if err != nil {
			return time.Time{}, fmt.Errorf(
				"invalid 'due' time %q: want RFC3339 like 2026-06-25T14:30:00Z",
				due,
			)
		}
		return t, nil
	default:
		return time.Time{}, errors.New("provide 'due' (RFC3339) or 'in' (duration)")
	}
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	rem, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toAPI(rem))
}

// updateReq carries optional edits; an omitted field is left unchanged.
type updateReq struct {
	Title  *string `json:"title"`
	Body   *string `json:"body"`
	Due    *string `json:"due"`
	Repeat *string `json:"repeat"`
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var req updateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return
	}
	var patch store.Patch
	patch.Title = req.Title
	patch.Body = req.Body
	patch.Repeat = req.Repeat
	if req.Due != nil {
		t, err := time.Parse(time.RFC3339, *req.Due)
		if err != nil {
			writeErr(
				w,
				http.StatusBadRequest,
				fmt.Errorf("invalid 'due' time %q: want RFC3339", *req.Due),
			)
			return
		}
		patch.Due = &t
	}
	rem, err := s.store.Update(r.PathValue("id"), patch)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toAPI(rem))
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	rem, err := s.store.Cancel(r.PathValue("id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	kitnotify.EmitEvent("reminder", "info", "reminder cancelled: "+rem.Title, "",
		map[string]string{"id": rem.ID})
	writeJSON(w, http.StatusOK, s.toAPI(rem))
}

// handleTest delivers a reminder's notification immediately without mutating it
// — a dry run to confirm the notification path works on this machine. It does
// not consume a one-shot or advance a recurring schedule, and emits no event.
func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	rem, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if err := s.notifier.Notify(rem.Title, rem.Body); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("notify failed: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, s.toAPI(rem))
}

// handleFire fires a reminder for real, right now — the same path the ticker
// takes when a reminder comes due: deliver, archive the event, then Trigger
// (a recurring reminder reschedules, a one-shot becomes "fired"). Unlike test,
// it bypasses the due-time check but still no-ops Trigger on a non-pending
// reminder, so the notification is sent but state is left alone.
func (s *Server) handleFire(w http.ResponseWriter, r *http.Request) {
	rem, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if err := s.notifier.Notify(rem.Title, rem.Body); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("notify failed: %w", err))
		return
	}
	kitnotify.EmitEvent("reminder", "info", "reminder fired: "+rem.Title, rem.Body,
		map[string]string{"id": rem.ID})
	fired, err := s.store.Trigger(rem.ID, s.now())
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toAPI(fired))
}

// handleTestGlobal sends a generic test notification with no reminder attached —
// a first-run check that notifications work at all (e.g. right after ralph up).
func (s *Server) handleTestGlobal(w http.ResponseWriter, _ *http.Request) {
	if err := s.notifier.Notify("Test", "Reminder notifications are working."); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("notify failed: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Delete(r.PathValue("id")); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validStatus(s string) bool {
	switch s {
	case store.StatusPending, store.StatusFired, store.StatusDone, store.StatusCancelled:
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

func writeStoreErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeErr(w, http.StatusBadRequest, err)
}

// logRequests logs one line per request so update problems are visible in
// t-man logs: t-man logs reminder --stderr.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf(
			"reminder: %s %s (%s)",
			r.Method,
			r.URL.Path,
			time.Since(start).Round(time.Millisecond),
		)
	})
}
