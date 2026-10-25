package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// DefaultLimit caps a listing (sessions, edits, history) when the request
// names no limit, so an unfiltered call cannot return the whole store.
const DefaultLimit = 50

// chunkCacheControl lets a browser keep a chunk for a year: the key is the
// SHA-256 of the bytes, so the bytes behind a key never change.
const chunkCacheControl = "max-age=31536000, immutable"

// chunkKey is the shape of a chunk key: lowercase hex SHA-256.
var chunkKey = regexp.MustCompile(`^[0-9a-f]{64}$`)

// SessionSummary is a session as the listing shows it: the session record
// plus what it touched. It costs one detail and one edit lookup per row,
// which the default limit bounds.
type SessionSummary struct {
	store.Session
	// Repos are the repository identities the session edited, sorted.
	Repos []string `json:"repos"`
	// Worktrees are the checkout roots the session edited, sorted.
	Worktrees []string `json:"worktrees"`
	TurnCount int      `json:"turn_count"`
	EditCount int      `json:"edit_count"`
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, limit, err := s.parseWindow(q)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	list, err := s.store.Sessions(ctx, store.SessionFilter{
		Repo: q.Get("repo"), Since: since, Limit: limit,
	})
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	sums := make([]SessionSummary, 0, len(list))
	for _, sess := range list {
		sum, err := s.summarize(ctx, sess.ID)
		if err != nil {
			writeBackendErr(w, err)
			return
		}
		sums = append(sums, sum)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sums})
}

// summarize loads one session's detail and edits into its listing row.
func (s *Server) summarize(ctx context.Context, id string) (SessionSummary, error) {
	d, err := s.store.Session(ctx, id)
	if err != nil {
		return SessionSummary{}, err
	}
	edits, err := s.store.Edits(ctx, store.EditQuery{SessionID: id})
	if err != nil {
		return SessionSummary{}, err
	}
	repos := []string{}
	for _, e := range edits {
		if e.Repo != "" && !slices.Contains(repos, e.Repo) {
			repos = append(repos, e.Repo)
		}
	}
	slices.Sort(repos)
	worktrees := d.Worktrees
	if worktrees == nil {
		worktrees = []string{}
	}
	return SessionSummary{
		Session:   d.Session,
		Repos:     repos,
		Worktrees: worktrees,
		TurnCount: len(d.Turns),
		EditCount: d.EditCount,
	}, nil
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.Session(r.Context(), r.PathValue("id"))
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleTurn(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.Turn(r.Context(), r.PathValue("id"))
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
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
	since, err = parseSince(q.Get("since"), s.now())
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

// parseSince reads a lower time bound: an RFC 3339 time, or a positive
// duration before now ("90m", "24h", or whole days as "7d"). Empty means no
// bound.
func parseSince(raw string, now time.Time) (time.Time, error) {
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
