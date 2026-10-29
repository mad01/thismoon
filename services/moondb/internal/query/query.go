// Package query answers the questions the store exists for: why a line
// looks the way it does, what happened to a path, what took time in a
// session and how it spent its context. It reads the store and git, and
// for a scorecard the events service; it never writes any of them.
package query

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/scorecard"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
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

// Service answers queries over one store. Use it by pointer: it holds the
// trace caches.
type Service struct {
	Store store.Store
	// Prices costs the model calls in a trace; the zero value prices with
	// moondb.DefaultPrices.
	Prices trace.Prices
	// Peers reads the events service for a session's scorecard; nil leaves
	// it unavailable.
	Peers scorecard.Peers

	cacheOnce   sync.Once
	cache       *traceCache[trace.TurnTrace]
	agentTraces *traceCache[trace.AgentTrace]
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
// session it belongs to, the subagent that made it when one did, and the
// commit it landed in when known. When the blob join fails, Candidates
// lists content matches best first and Edit is the top one.
type WhyResult struct {
	Path       string            `json:"path"`
	Line       int               `json:"line"`
	Text       string            `json:"text,omitempty"`
	Method     Method            `json:"method"`
	Commit     string            `json:"commit,omitempty"`
	Edit       *store.Edit       `json:"edit,omitempty"`
	Turn       *store.Turn       `json:"turn,omitempty"`
	Session    *store.Session    `json:"session,omitempty"`
	Agent      *WhyAgent         `json:"agent,omitempty"`
	CommitLink *store.CommitLink `json:"commit_link,omitempty"`
	Candidates []Candidate       `json:"candidates,omitempty"`
}

// WhyAgent is the subagent that made an edit: who it was and its spawn
// brief, the prompt it was spawned with, which names the task the edit
// served. Messages its parent sent it later are not in the brief;
// LaterMessages counts them. The turn's prompt is the person's message to
// the main conversation. Recorded is false when the store holds no Agent
// record, only the edit's agent id and type. BriefChunk is empty then, and
// on a record stored before briefs were kept: the brief is unknown.
type WhyAgent struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	BriefChunk  string `json:"brief_chunk,omitempty"`
	// LaterMessages counts the messages that reached the agent after its
	// spawn brief, which the brief does not hold.
	LaterMessages int  `json:"later_messages,omitempty"`
	Recorded      bool `json:"recorded"`
}

// Label names the agent for a person: its name, else its type.
func (a WhyAgent) Label() string { return cmp.Or(a.Name, a.Type, "subagent") }

// MissingBrief says why an agent of type typ has no spawn brief: one of
// type internal has no transcript to take one from, a subagent known only
// from its edits has no record, and any other was recorded before moondb
// kept briefs, which a forced import fills.
func MissingBrief(typ string, recorded bool) string {
	switch {
	case typ == store.InternalAgent:
		return "type internal, no transcript to take it from"
	case !recorded:
		return "no agent record, only its edits"
	}
	return "recorded before moondb kept briefs; moondb import --since " +
		moondb.DefaultImportSince + " --force fills it"
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
// line nothing matches, get MethodNone with Text set. A worktree moondb
// never recorded an edit in is refused before anything is read from it.
func (s *Service) Why(ctx context.Context, req WhyRequest) (WhyResult, error) {
	if err := req.validate(); err != nil {
		return WhyResult{}, err
	}
	if req.Worktree != "" {
		if err := s.knownWorktree(ctx, req.Worktree); err != nil {
			return WhyResult{}, err
		}
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

// attach fills res with the Edit that explains the line, its turn,
// session and subagent, and the commit link: the one for res.Commit at
// path when blame named the commit, else the one recorded for the Edit,
// whose commit then becomes res.Commit. A turn or session the store lacks
// is left out rather than failing the answer.
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
	if err := s.attachSession(ctx, res, e.SessionID); err != nil {
		return err
	}
	agent, err := s.whyAgent(ctx, e)
	res.Agent = agent
	return err
}

// attachSession sets res.Session to session id; left out when id is empty
// or the store lacks it.
func (s *Service) attachSession(ctx context.Context, res *WhyResult, id string) error {
	if id == "" {
		return nil
	}
	sd, err := s.Store.Session(ctx, id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("query: session %s: %w", id, err)
	}
	if err == nil {
		res.Session = &sd.Session
	}
	return nil
}

// whyAgent is the subagent that made e, from its Agent record or, without
// one, from the edit alone; nil for an edit of the main conversation.
func (s *Service) whyAgent(ctx context.Context, e store.Edit) (*WhyAgent, error) {
	if e.AgentID == "" {
		return nil, nil
	}
	a, err := s.Store.Agent(ctx, e.AgentID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return &WhyAgent{ID: e.AgentID, Type: cmp.Or(e.AgentType, "subagent")}, nil
	case err != nil:
		return nil, fmt.Errorf("query: agent %s: %w", e.AgentID, err)
	}
	agent := RecordedWhyAgent(a)
	return &agent, nil
}

// RecordedWhyAgent is the WhyAgent of an agent the store holds a record of.
func RecordedWhyAgent(a store.Agent) WhyAgent {
	return WhyAgent{
		ID: a.ID, Type: a.Type, Name: a.Name, Description: a.Description,
		BriefChunk: a.BriefChunk, LaterMessages: a.LaterMessages, Recorded: true,
	}
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
