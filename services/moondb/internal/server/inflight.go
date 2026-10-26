package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// InflightView is a GET /api/inflight answer: the files of one checkout
// with in-flight edits by sessions other than the asking one
// (store.InFlight), newest edit first.
type InflightView struct {
	Worktree string               `json:"worktree"`
	Files    []store.InFlightFile `json:"files"`
}

// inflightRequest is a parsed GET /api/inflight query.
type inflightRequest struct {
	worktree string
	// exclude is the asking session, whose own edits never count.
	exclude string
	since   time.Time
	limit   int
}

func (s *Server) handleInflight(w http.ResponseWriter, r *http.Request) {
	req, err := parseInflight(r.URL.Query(), s.now())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.inflight(r.Context(), req)
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// parseInflight reads the query: worktree is required and absolute; since
// defaults to moondb.InflightWindow before now and limit to
// moondb.InflightLimit.
func parseInflight(q url.Values, now time.Time) (inflightRequest, error) {
	req := inflightRequest{
		worktree: q.Get("worktree"),
		exclude:  q.Get("exclude_session"),
		since:    now.Add(-moondb.InflightWindow),
		limit:    moondb.InflightLimit,
	}
	if req.worktree == "" {
		return inflightRequest{}, errors.New("moondb: worktree is required")
	}
	if !filepath.IsAbs(req.worktree) {
		return inflightRequest{}, fmt.Errorf(
			"moondb: worktree must be an absolute checkout root, got %q", req.worktree)
	}
	req.worktree = filepath.Clean(req.worktree)
	var err error
	if raw := q.Get("since"); raw != "" {
		if req.since, err = ParseSince(raw, now); err != nil {
			return inflightRequest{}, err
		}
	}
	if raw := q.Get("limit"); raw != "" {
		if req.limit, err = parseLimit(raw); err != nil {
			return inflightRequest{}, err
		}
	}
	return req, nil
}

// inflight reads the checkout's edits in the window and the commit links
// of their turns, keeps the files store.InFlight keeps, and names the
// sessions behind them.
func (s *Server) inflight(ctx context.Context, req inflightRequest) (InflightView, error) {
	edits, err := s.store.Edits(ctx, store.EditQuery{Worktree: req.worktree, Since: req.since})
	if err != nil {
		return InflightView{}, err
	}
	links, err := s.commitLinks(ctx, edits)
	if err != nil {
		return InflightView{}, err
	}
	files := store.InFlight(edits, links,
		store.InFlightQuery{Worktree: req.worktree, Exclude: req.exclude})
	if len(files) > req.limit {
		files = files[:req.limit]
	}
	if err := s.titleSessions(ctx, files); err != nil {
		return InflightView{}, err
	}
	return InflightView{Worktree: req.worktree, Files: files}, nil
}

// commitLinks reads the commit links of the edits' turns. A link is filed
// under its edit's turn, so one turn read covers every edit of that turn.
// The store answers a turn with no record as not found, so the links of
// such a turn stay unread and its edits read as in flight.
func (s *Server) commitLinks(ctx context.Context, edits []store.Edit) ([]store.CommitLink, error) {
	var links []store.CommitLink
	read := map[string]bool{}
	for _, e := range edits {
		if e.TurnID == "" || read[e.TurnID] {
			continue
		}
		read[e.TurnID] = true
		d, err := s.store.Turn(ctx, e.TurnID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		links = append(links, d.CommitLinks...)
	}
	return links, nil
}

// titleSessions names each session the way the listing does, reading each
// Session record once. A session without a record is named by its id.
func (s *Server) titleSessions(ctx context.Context, files []store.InFlightFile) error {
	titles := map[string]string{}
	for i := range files {
		for j := range files[i].Sessions {
			id := files[i].Sessions[j].ID
			title, ok := titles[id]
			if !ok {
				d, err := s.store.Session(ctx, id)
				switch {
				case errors.Is(err, store.ErrNotFound):
					d.ID = id
				case err != nil:
					return err
				}
				title = sessionTitle(d.Session)
				titles[id] = title
			}
			files[i].Sessions[j].Title = title
		}
	}
	return nil
}
