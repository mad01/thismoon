package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// DefaultLimit caps a listing (sessions, edits, history) when the request
// names no limit, so an unfiltered call cannot return the whole store.
const DefaultLimit = 50

// chunkCacheControl lets a browser keep a chunk for a year: the key is the
// SHA-256 of the bytes, so the bytes behind a key never change.
const chunkCacheControl = "max-age=31536000, immutable"

// chunkKey is the shape of a chunk key: lowercase hex SHA-256.
var chunkKey = regexp.MustCompile(`^[0-9a-f]{64}$`)

// TurnView is a turn as GET /api/turns/{id} answers it: the turn with
// everything recorded under it, plus its trace totals when its transcript
// lines put anything on the timeline, and the agents that worked in it.
// The totals add the work of the subagents with a record under the turn
// (trace.WithAgents) beside the main conversation's.
type TurnView struct {
	store.TurnDetail
	TraceTotals *trace.Totals `json:"trace_totals,omitempty"`
	Agents      []AgentRow    `json:"agents"`
}

func (s *Server) handleTurn(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.Turn(r.Context(), r.PathValue("id"))
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.turnView(r.Context(), d))
}

// turnView adds a turn's trace totals and its agents. A trace that cannot
// be computed is logged and left out, and the agents come from the edits
// alone: the turn itself is still the answer.
func (s *Server) turnView(ctx context.Context, d store.TurnDetail) TurnView {
	v := TurnView{TurnDetail: d}
	tt, err := s.query.TurnTrace(ctx, d)
	if err != nil {
		log.Printf("moondb: trace of turn %s: %v", d.ID, err)
		v.Agents = s.turnAgents(d, trace.TurnTrace{TurnID: d.ID}, nil)
		return v
	}
	runs, err := s.query.TurnAgents(ctx, d)
	if err != nil {
		log.Printf("moondb: agents of turn %s: %v", d.ID, err)
	}
	tt = trace.WithAgents(tt, runs)
	if tt.Lines.Events > 0 || len(runs) > 0 {
		v.TraceTotals = &tt.Totals
	}
	v.Agents = s.turnAgents(d, tt, runs)
	return v
}

// handleSessionTrace answers what took time in a session: every turn's
// spans and totals, the user time between turns, and the session totals.
func (s *Server) handleSessionTrace(w http.ResponseWriter, r *http.Request) {
	st, err := s.query.SessionTrace(r.Context(), r.PathValue("id"))
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleEdits(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, limit, err := s.parseWindow(q)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	edits, err := s.store.Edits(r.Context(), store.EditQuery{
		Repo:      q.Get("repo"),
		Path:      q.Get("path"),
		Worktree:  q.Get("worktree"),
		SessionID: q.Get("session"),
		TurnID:    q.Get("turn"),
		AfterBlob: q.Get("blob"),
		Since:     since,
		Limit:     limit,
	})
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"edits": edits})
}

func (s *Server) handleWhy(w http.ResponseWriter, r *http.Request) {
	req, err := parseWhy(r.URL.Query())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	res, err := s.query.Why(r.Context(), req)
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// parseWhy reads a WhyRequest from the query string: path and a 1-based line
// are required, worktree, repo and text are optional.
func parseWhy(q url.Values) (query.WhyRequest, error) {
	path := q.Get("path")
	if path == "" {
		return query.WhyRequest{}, errors.New("moondb: path is required")
	}
	line, err := strconv.Atoi(q.Get("line"))
	if err != nil || line < 1 {
		return query.WhyRequest{}, fmt.Errorf(
			"moondb: line must be a 1-based line number, got %q", q.Get("line"))
	}
	return query.WhyRequest{
		Worktree: q.Get("worktree"),
		Repo:     q.Get("repo"),
		Path:     path,
		Line:     line,
		Text:     q.Get("text"),
	}, nil
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	path := q.Get("path")
	if path == "" {
		writeErr(w, http.StatusBadRequest, errors.New("moondb: path is required"))
		return
	}
	limit, err := parseLimit(q.Get("limit"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	edits, err := s.query.History(r.Context(), query.HistoryRequest{
		Worktree: q.Get("worktree"),
		Repo:     q.Get("repo"),
		Path:     path,
		Limit:    limit,
	})
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	if edits == nil {
		edits = []store.Edit{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"edits": edits})
}

// handleAnnotate answers every line of one file on disk. worktree and path
// are required; repo is optional and narrows the content match. The query
// checks the rest: an absolute worktree with recorded edits and a regular
// file inside it.
func (s *Server) handleAnnotate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := query.AnnotateRequest{
		Worktree: q.Get("worktree"),
		Repo:     q.Get("repo"),
		Path:     q.Get("path"),
	}
	switch {
	case req.Path == "":
		writeErr(w, http.StatusBadRequest, errors.New("moondb: path is required"))
		return
	case req.Worktree == "":
		writeErr(w, http.StatusBadRequest, errors.New("moondb: worktree is required"))
		return
	}
	res, err := s.query.Annotate(r.Context(), req)
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleChunk serves a chunk's raw bytes. The key is validated before the
// store sees it, so a malformed key is a 400 and never a path lookup.
func (s *Server) handleChunk(w http.ResponseWriter, r *http.Request) {
	sha := r.PathValue("sha")
	if !chunkKey.MatchString(sha) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf(
			"moondb: chunk key %q is not a lowercase hex SHA-256 (64 characters)", sha))
		return
	}
	data, err := s.store.Chunks().Get(r.Context(), sha)
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Cache-Control", chunkCacheControl)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// parseWindow reads the since and limit parameters every listing takes.
func (s *Server) parseWindow(q url.Values) (since time.Time, limit int, err error) {
	since, err = ParseSince(q.Get("since"), s.now())
	if err != nil {
		return time.Time{}, 0, err
	}
	limit, err = parseLimit(q.Get("limit"))
	if err != nil {
		return time.Time{}, 0, err
	}
	return since, limit, nil
}

// parseLimit reads a positive limit, DefaultLimit when raw is empty.
func parseLimit(raw string) (int, error) {
	if raw == "" {
		return DefaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("moondb: limit must be a positive integer, got %q", raw)
	}
	return n, nil
}

// ParseSince reads a lower time bound: an RFC 3339 time, or a positive
// duration before now ("90m", "24h", or whole days as "7d"). Empty means no
// bound.
func ParseSince(raw string, now time.Time) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	d, err := parseAge(raw)
	if err != nil || d <= 0 {
		return time.Time{}, fmt.Errorf(
			"moondb: since must be an RFC 3339 time or a positive duration like 24h or 7d, got %q",
			raw)
	}
	return now.Add(-d), nil
}

// parseAge is time.ParseDuration plus whole days ("7d"), the unit people
// reach for first when asking about recent sessions.
func parseAge(raw string) (time.Duration, error) {
	days, ok := strings.CutSuffix(raw, "d")
	if !ok {
		return time.ParseDuration(raw)
	}
	n, err := strconv.Atoi(days)
	if err != nil {
		return 0, err
	}
	return time.Duration(n) * 24 * time.Hour, nil
}
