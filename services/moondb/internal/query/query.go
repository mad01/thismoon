// Package query answers the questions the store exists for: why a line
// looks the way it does, and what happened to a path. It reads the store
// and git; it never writes either.
package query

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// ErrNotImplemented marks a question this build cannot answer yet.
var ErrNotImplemented = errors.New("query: not implemented")

// ErrInvalidRequest wraps every error about the request itself rather than
// the store: a missing path, a line number below 1, a relative worktree.
// A server maps it to a client error.
var ErrInvalidRequest = errors.New("query: invalid request")

// defaultHistoryLimit is how many edits History returns when the request
// names no limit.
const defaultHistoryLimit = 100

// Service answers queries over one store.
type Service struct {
	Store store.Store
}

// Method says how a Why answer was found.
type Method string

const (
	// MethodBlame: git blame named the commit and its blob joined an Edit.
	MethodBlame Method = "blame"
	// MethodContent: the line's text matched a line an Edit added.
	MethodContent Method = "content"
	// MethodNone: nothing recorded explains the line.
	MethodNone Method = "none"
)

// WhyRequest asks about one line of one file. Worktree is the absolute
// root of the checkout the file is read from (git runs there); Repo narrows
// the content match when the worktree is gone. Path is worktree-relative.
// Line is 1-based. Text is the line's content when the caller has it and
// the file is not on disk.
type WhyRequest struct {
	Worktree string `json:"worktree,omitempty"`
	Repo     string `json:"repo,omitempty"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Text     string `json:"text,omitempty"`
}

// WhyResult is the answer: the Edit that produced the line, the turn and
// session it belongs to, and the commit it landed in when known. When the
// blob join fails, Candidates lists content matches best first and Edit is
// the top one.
type WhyResult struct {
	Path       string            `json:"path"`
	Line       int               `json:"line"`
	Text       string            `json:"text,omitempty"`
	Method     Method            `json:"method"`
	Commit     string            `json:"commit,omitempty"`
	Edit       *store.Edit       `json:"edit,omitempty"`
	Turn       *store.Turn       `json:"turn,omitempty"`
	Session    *store.Session    `json:"session,omitempty"`
	CommitLink *store.CommitLink `json:"commit_link,omitempty"`
	Candidates []Candidate       `json:"candidates,omitempty"`
}

// Candidate is one Edit that may explain a line, with how well it fits.
type Candidate struct {
	Edit   store.Edit `json:"edit"`
	Score  float64    `json:"score"`
	Reason string     `json:"reason"`
}

// HistoryRequest asks for the recorded edits of one path, newest first.
// Worktree narrows to one checkout; Repo alone spans every checkout of the
// repository.
type HistoryRequest struct {
	Worktree string `json:"worktree,omitempty"`
	Repo     string `json:"repo,omitempty"`
	Path     string `json:"path"`
	Limit    int    `json:"limit,omitempty"`
}

// Why answers a WhyRequest. A line committed in the worktree is traced by
// git blame: the blob the blamed commit holds for the file joins the Edit
// whose after-content it is (MethodBlame). Otherwise, or when no Edit holds
// that blob, the line's text is matched against the lines recorded edits
// of the path added, newest first (MethodContent). A line too common to
// tell edits apart, such as a brace, a lone return or an import, and a
// line nothing matches, get MethodNone with Text set.
func (s *Service) Why(ctx context.Context, req WhyRequest) (WhyResult, error) {
	if err := req.validate(); err != nil {
		return WhyResult{}, err
	}
	res := WhyResult{Path: req.Path, Line: req.Line, Text: req.Text, Method: MethodNone}
	found, err := s.whyByBlame(ctx, req, &res)
	if err != nil || found {
		return res, err
	}
	if res.Text == "" {
		res.Text = lineAt(req)
	}
	return res, s.whyByContent(ctx, req, &res)
}

// History lists the recorded edits of a path, newest first, at most
// Limit of them (100 when Limit is 0).
func (s *Service) History(ctx context.Context, req HistoryRequest) ([]store.Edit, error) {
	if req.Path == "" {
		return nil, fmt.Errorf("%w: history needs a path", ErrInvalidRequest)
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultHistoryLimit
	}
	edits, err := s.Store.Edits(ctx, store.EditQuery{
		Worktree: req.Worktree, Repo: req.Repo, Path: req.Path, Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("query: history of %s: %w", req.Path, err)
	}
	if edits == nil {
		edits = []store.Edit{}
	}
	return edits, nil
}

// validate checks what Why needs: a path, a line number or the line's
// text, and an absolute worktree when one is named.
func (r WhyRequest) validate() error {
	switch {
	case r.Path == "":
		return fmt.Errorf("%w: why needs a path", ErrInvalidRequest)
	case r.Line < 0, r.Line == 0 && r.Text == "":
		return fmt.Errorf("%w: why needs a line number from 1, or the line's text",
			ErrInvalidRequest)
	case r.Worktree != "" && !filepath.IsAbs(r.Worktree):
		return fmt.Errorf("%w: worktree %q is not absolute", ErrInvalidRequest, r.Worktree)
	}
	return nil
}

// attach fills res with the Edit that explains the line, its turn and
// session, and the commit link: the one for res.Commit at path when blame
// named the commit, else the one recorded for the Edit, whose commit then
// becomes res.Commit. A turn or session the store lacks is left out
// rather than failing the answer.
func (s *Service) attach(ctx context.Context, res *WhyResult, e store.Edit, path string) error {
	res.Edit = &e
	if e.TurnID != "" {
		td, err := s.Store.Turn(ctx, e.TurnID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("query: turn %s: %w", e.TurnID, err)
		}
		if err == nil {
			res.Turn = &td.Turn
			res.CommitLink = linkFor(td.CommitLinks, e.ID, res.Commit, path)
		}
	}
	if res.Commit == "" && res.CommitLink != nil {
		res.Commit = res.CommitLink.Commit
	}
	if e.SessionID == "" {
		return nil
	}
	sd, err := s.Store.Session(ctx, e.SessionID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("query: session %s: %w", e.SessionID, err)
	}
	if err == nil {
		res.Session = &sd.Session
	}
	return nil
}

// linkFor picks the commit link of commit at path, or, with no commit, the
// link recorded for the edit; nil when there is none.
func linkFor(links []store.CommitLink, editID, commit, path string) *store.CommitLink {
	for _, l := range links {
		if commit != "" && l.Commit == commit && l.Path == path ||
			commit == "" && l.EditID == editID {
			return &l
		}
	}
	return nil
}
