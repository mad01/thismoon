// Package servertest holds in-memory stand-ins for the store and the query
// service, seeded with one small fixture, so the server, client and MCP tests
// drive the HTTP surface without a backend on disk. The fakes implement the
// read side faithfully enough for those tests; every write method fails,
// because serve never writes through the API.
package servertest

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// Fixture ids and values the tests assert against.
const (
	// Repo and Worktree locate every edit of the newer session.
	Repo     = "example.com/owner/thismoon"
	Worktree = "/work/thismoon"
	// OtherRepo is the only repo the older session edited.
	OtherRepo = "example.com/owner/other"

	// OldPromptLine is the older session's cached first prompt line, its
	// title in a listing since it has no harness title.
	OldPromptLine = "fix the other repo"
	// OldTranscriptPath is the older session's transcript file.
	OldTranscriptPath = "/home/.transcripts/sess-old.jsonl"

	NewSession = "sess-new"
	OldSession = "sess-old"
	NewTurn    = "turn-new"
	OldTurn    = "turn-old"

	// EditA is the newest edit; it changed PathA and carries content.
	EditA = "toolu_a:0"
	EditB = "toolu_b:0"
	EditC = "toolu_c:0"
	PathA = "a.go"
	PathB = "b.go"

	// SnapshotID is the newer turn's one snapshot, and SnapshotDeltaID the
	// unattributed delta it found.
	SnapshotID      = NewTurn + ":snap:pre:wt"
	SnapshotDeltaID = SnapshotID + ":" + PathB

	// Prompt is the newer turn's prompt text; PromptSHA is its chunk key.
	Prompt = "add the server\nwith a second line"
	// After is EditA's after-content; AfterSHA is its chunk key.
	After = "package a\n"

	// Transcript is the older turn's transcript: a model call that asks for
	// EditC's tool call, its result, and a closing model call, all on
	// claude-opus-5-5. OldTurn runs from minute 1 to minute 3.
	Transcript = `{"type":"api-request","timestamp":"2026-10-01T09:01:00Z"}
{"type":"assistant","timestamp":"2026-10-01T09:01:30Z","message":{"id":"msg_old_1",` +
		`"model":"claude-opus-5-5","content":[{"type":"tool_use","id":"toolu_c","name":"Edit"}],` +
		`"usage":{"input_tokens":1000,"output_tokens":500}}}
{"type":"user","timestamp":"2026-10-01T09:02:30Z",` +
		`"message":{"content":[{"type":"tool_result","tool_use_id":"toolu_c"}]}}
{"type":"api-request","timestamp":"2026-10-01T09:02:31Z"}
{"type":"assistant","timestamp":"2026-10-01T09:02:50Z","message":{"id":"msg_old_2",` +
		`"model":"claude-opus-5-5","content":[{"type":"text","text":"done"}],` +
		`"usage":{"input_tokens":10,"output_tokens":20}}}
`
)

// Chunk keys of the fixture contents.
var (
	PromptSHA     = Key([]byte(Prompt))
	AfterSHA      = Key([]byte(After))
	TranscriptSHA = Key([]byte(Transcript))
)

// T0 anchors every fixture time.
var T0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// Key returns the chunk key of data: lowercase hex SHA-256.
func Key(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// errReadOnly is what every write method returns.
var errReadOnly = errors.New("servertest: the fixture store is read-only")

// Store is an in-memory store.Store over a fixed set of records.
type Store struct {
	sessions []store.Session // oldest first
	turns    []store.Turn
	edits    []store.Edit // oldest first
	snaps    []store.Snapshot
	deltas   []store.UnattributedDelta
	chunks   chunkMap

	mu      sync.Mutex
	pingErr error
}

// at is the fixture time minutes after T0.
func at(minutes int) time.Time { return T0.Add(time.Duration(minutes) * time.Minute) }

// NewTurnSkipped is the newer turn's skipped files: one Bash call with a
// count for every reason.
func NewTurnSkipped() []store.SkippedFiles {
	return []store.SkippedFiles{
		{Tool: "Bash", ToolUseID: "toolu_pull", At: at(62), Clean: 40, Timeout: 2, Failed: 1},
	}
}

// NewStore returns a Store seeded with the fixture: an older session whose
// closed turn made one edit in OtherRepo and kept its Transcript and the
// totals it gives, and a newer session whose one open turn made two edits
// in Repo, opened with a snapshot that found one unattributed delta.
func NewStore() *Store {
	loc := store.Location{Repo: Repo, Worktree: Worktree, Canonical: Worktree, Branch: "main"}
	other := store.Location{Repo: OtherRepo, Worktree: "/work/other", Canonical: "/work/other"}
	return &Store{
		sessions: []store.Session{
			{
				ID: OldSession, Harness: "test-harness", Cwd: "/work/other",
				PromptLine: OldPromptLine, TranscriptPath: OldTranscriptPath, StartedAt: at(0),
			},
			{
				ID: NewSession, Harness: "test-harness", Cwd: Worktree,
				Title: "build the server", StartedAt: at(60),
			},
		},
		turns: []store.Turn{
			oldTurn(),
			{
				ID: NewTurn, SessionID: NewSession, Seq: 1, PromptChunk: PromptSHA,
				Status: store.TurnOpen, StartedAt: at(61), Skipped: NewTurnSkipped(),
			},
		},
		edits: []store.Edit{
			fixtureEdit(EditC, OldSession, OldTurn, at(2), other, "c.go"),
			fixtureEdit(EditB, NewSession, NewTurn, at(62), loc, PathB),
			withContent(fixtureEdit(EditA, NewSession, NewTurn, at(63), loc, PathA)),
		},
		snaps: []store.Snapshot{{
			ID: SnapshotID, SessionID: NewSession, TurnID: NewTurn, Phase: store.SnapshotPre,
			At: at(61), Location: loc, Scope: store.SnapshotScope, FileCount: 2,
			Skipped: store.SnapshotSkipped{TooLarge: 1},
		}},
		deltas: []store.UnattributedDelta{{
			ID: SnapshotDeltaID, SessionID: NewSession, TurnID: NewTurn, At: at(61),
			Location: loc, Path: PathB, Cause: store.DeltaSnapshot,
		}},
		chunks: chunkMap{
			PromptSHA:     []byte(Prompt),
			AfterSHA:      []byte(After),
			TranscriptSHA: []byte(Transcript),
		},
	}
}

// oldTurn is the older session's closed turn with the totals its
// Transcript gives.
func oldTurn() store.Turn {
	old := store.Turn{
		ID:              OldTurn,
		SessionID:       OldSession,
		Seq:             1,
		Status:          store.TurnClosed,
		StartedAt:       at(1),
		EndedAt:         new(at(3)),
		TranscriptChunk: TranscriptSHA,
	}
	totals := trace.Stored(trace.Build(store.TurnDetail{Turn: old}, []byte(Transcript),
		trace.Prices{}).Totals)
	old.Totals = &totals
	return old
}

func fixtureEdit(id, session, turn string, at time.Time, l store.Location, path string) store.Edit {
	return store.Edit{
		ID: id, ToolUseID: id[:len(id)-2], SessionID: session, TurnID: turn,
		Tool: "Edit", At: at, Location: l, Path: path, Kind: store.EditUpdate,
		BeforeSource: store.BeforeUnknown,
	}
}

func withContent(e store.Edit) store.Edit {
	e.Kind = store.EditCreate
	e.BeforeSource = store.BeforeAbsent
	e.AfterChunk = AfterSHA
	e.AfterBlob = "blob-a"
	return e
}

// SetPingErr makes Ping fail with err, or succeed again when err is nil.
func (s *Store) SetPingErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pingErr = err
}

// PutSession fails: the fixture is read-only.
func (s *Store) PutSession(context.Context, store.Session) error { return errReadOnly }

// PutTurn fails: the fixture is read-only.
func (s *Store) PutTurn(context.Context, store.Turn) error { return errReadOnly }

// AppendEdit fails: the fixture is read-only.
func (s *Store) AppendEdit(context.Context, store.Edit) error { return errReadOnly }

// AppendDelta fails: the fixture is read-only.
func (s *Store) AppendDelta(context.Context, store.UnattributedDelta) error {
	return errReadOnly
}

// AppendSnapshot fails: the fixture is read-only.
func (s *Store) AppendSnapshot(context.Context, store.Snapshot) error { return errReadOnly }

// AppendCommitLink fails: the fixture is read-only.
func (s *Store) AppendCommitLink(context.Context, store.CommitLink) error { return errReadOnly }

// Sessions lists sessions newest first, narrowed by f, each summarized.
func (s *Store) Sessions(_ context.Context, f store.SessionFilter) ([]store.SessionSummary, error) {
	out := []store.SessionSummary{}
	for _, sess := range slices.Backward(s.sessions) {
		sum := s.summary(sess)
		if !f.Keeps(sum) {
			continue
		}
		out = append(out, sum)
		if f.Limit > 0 && len(out) == f.Limit {
			break
		}
	}
	return out, nil
}

// summary adds up one session's fixture records.
func (s *Store) summary(sess store.Session) store.SessionSummary {
	return store.Summarize(sess, s.sessionTurns(sess.ID), func(yield func(*store.Edit) bool) {
		for i := range s.edits {
			if s.edits[i].SessionID == sess.ID && !yield(&s.edits[i]) {
				return
			}
		}
	})
}

// sessionTurns lists one session's turns in Seq order.
func (s *Store) sessionTurns(id string) []store.Turn {
	out := []store.Turn{}
	for _, t := range s.turns {
		if t.SessionID == id {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b store.Turn) int { return cmp.Compare(a.Seq, b.Seq) })
	return out
}

// Session loads one session with its summary and turns.
func (s *Store) Session(_ context.Context, id string) (store.SessionDetail, error) {
	i := slices.IndexFunc(s.sessions, func(x store.Session) bool { return x.ID == id })
	if i < 0 {
		return store.SessionDetail{}, fmt.Errorf("session %s: %w", id, store.ErrNotFound)
	}
	return store.SessionDetail{
		SessionSummary: s.summary(s.sessions[i]),
		Turns:          s.sessionTurns(id),
	}, nil
}

// TurnsWithoutTotals lists the closed or failed fixture turns that lack
// totals from version, in store.SortForTotals order.
func (s *Store) TurnsWithoutTotals(_ context.Context, version, limit int) ([]store.Turn, error) {
	out := []store.Turn{}
	for _, t := range s.turns {
		if store.NeedsTotals(t, version) {
			out = append(out, t)
		}
	}
	store.SortForTotals(out, func(id string) time.Time {
		i := slices.IndexFunc(s.sessions, func(x store.Session) bool { return x.ID == id })
		if i < 0 {
			return time.Time{}
		}
		return s.sessions[i].StartedAt
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Turn loads one turn with its edits, deltas and snapshots; the fixture
// records no commit links.
func (s *Store) Turn(ctx context.Context, id string) (store.TurnDetail, error) {
	i := slices.IndexFunc(s.turns, func(t store.Turn) bool { return t.ID == id })
	if i < 0 {
		return store.TurnDetail{}, fmt.Errorf("turn %s: %w", id, store.ErrNotFound)
	}
	edits, _ := s.Edits(ctx, store.EditQuery{TurnID: id})
	return store.TurnDetail{
		Turn:  s.turns[i],
		Edits: edits,
		Deltas: underTurn(s.deltas, id, func(d store.UnattributedDelta) string {
			return d.TurnID
		}),
		Snapshots:   underTurn(s.snaps, id, func(v store.Snapshot) string { return v.TurnID }),
		CommitLinks: []store.CommitLink{},
	}, nil
}

// underTurn returns the records of turn id, newest first; never nil.
func underTurn[T any](list []T, id string, turnOf func(T) string) []T {
	out := []T{}
	for _, v := range slices.Backward(list) {
		if turnOf(v) == id {
			out = append(out, v)
		}
	}
	return out
}

// Edits lists edits newest first, narrowed by q.
func (s *Store) Edits(_ context.Context, q store.EditQuery) ([]store.Edit, error) {
	out := []store.Edit{}
	for _, e := range slices.Backward(s.edits) {
		if e.At.Before(q.Since) || !editMatches(q, e) {
			continue
		}
		out = append(out, e)
		if q.Limit > 0 && len(out) == q.Limit {
			break
		}
	}
	return out, nil
}

func editMatches(q store.EditQuery, e store.Edit) bool {
	return (q.Repo == "" || e.Repo == q.Repo) &&
		(q.Path == "" || e.Path == q.Path) &&
		(q.Worktree == "" || e.Worktree == q.Worktree) &&
		(q.SessionID == "" || e.SessionID == q.SessionID) &&
		(q.TurnID == "" || e.TurnID == q.TurnID) &&
		(q.AfterBlob == "" || e.AfterBlob == q.AfterBlob)
}

// LatestSnapshot returns the newest snapshot q allows.
func (s *Store) LatestSnapshot(_ context.Context, q store.SnapshotQuery) (store.Snapshot, error) {
	for _, v := range slices.Backward(s.snaps) {
		if v.Worktree == q.Worktree && v.ID != q.ExcludeID && (q.At.IsZero() || !v.At.After(q.At)) {
			return v, nil
		}
	}
	return store.Snapshot{}, fmt.Errorf("snapshot of %s: %w", q.Worktree, store.ErrNotFound)
}

// LatestAfter returns the newest edit q allows with after-content.
func (s *Store) LatestAfter(ctx context.Context, q store.ChainQuery) (store.Edit, error) {
	edits, _ := s.Edits(ctx, store.EditQuery{Worktree: q.Worktree, Path: q.Path})
	for _, e := range edits {
		if e.AfterChunk != "" && e.ID != q.ExcludeID && (q.At.IsZero() || !e.At.After(q.At)) {
			return e, nil
		}
	}
	return store.Edit{}, fmt.Errorf("after-content of %s: %w", q.Path, store.ErrNotFound)
}

// OpenTurns lists the open turns with an edit in worktree.
func (s *Store) OpenTurns(ctx context.Context, worktree string) ([]store.Turn, error) {
	out := []store.Turn{}
	for _, t := range s.turns {
		edits, _ := s.Edits(ctx, store.EditQuery{TurnID: t.ID, Worktree: worktree})
		if t.Status == store.TurnOpen && len(edits) > 0 {
			out = append(out, t)
		}
	}
	return out, nil
}

// Chunks returns the fixture's content store.
func (s *Store) Chunks() store.Chunks { return s.chunks }

// Ping reports the error set by SetPingErr, nil by default.
func (s *Store) Ping(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pingErr
}

// Close does nothing.
func (s *Store) Close() error { return nil }

// chunkMap is a read-only store.Chunks over a fixed map.
type chunkMap map[string][]byte

func (c chunkMap) Put(context.Context, []byte) (string, error) { return "", errReadOnly }

func (c chunkMap) Get(_ context.Context, sha string) ([]byte, error) {
	data, ok := c[sha]
	if !ok {
		return nil, fmt.Errorf("chunk %s: %w", sha, store.ErrNotFound)
	}
	return slices.Clone(data), nil
}

func (c chunkMap) Has(_ context.Context, sha string) (bool, error) {
	_, ok := c[sha]
	return ok, nil
}

// Querier is a stand-in for query.Service over a Store: Why answers with the
// newest edit of the path, History lists the path's edits. It records the
// last request of each kind so tests can check what the server parsed.
// Traces are pure functions of the store, so it answers those with the
// real query.Service.
type Querier struct {
	store  *Store
	traces *query.Service

	mu           sync.Mutex
	err          error
	lastWhy      query.WhyRequest
	lastHistory  query.HistoryRequest
	lastAnnotate query.AnnotateRequest
}

// NewQuerier returns a Querier reading st.
func NewQuerier(st *Store) *Querier {
	return &Querier{store: st, traces: &query.Service{Store: st}}
}

// TurnTrace computes the turn's trace with the real query service.
func (q *Querier) TurnTrace(ctx context.Context, d store.TurnDetail) (trace.TurnTrace, error) {
	return q.traces.TurnTrace(ctx, d)
}

// SessionTrace computes the session's trace with the real query service.
func (q *Querier) SessionTrace(ctx context.Context, id string) (trace.SessionTrace, error) {
	return q.traces.SessionTrace(ctx, id)
}

// UsageCost prices usage with the real query service's default table.
func (q *Querier) UsageCost(usage []store.ModelUsage) (*float64, []string) {
	return q.traces.UsageCost(usage)
}

// SetErr makes Why, History and Annotate fail with err, or answer again
// when err is nil.
func (q *Querier) SetErr(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.err = err
}

// LastWhy returns the last request Why received.
func (q *Querier) LastWhy() query.WhyRequest {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.lastWhy
}

// LastHistory returns the last request History received.
func (q *Querier) LastHistory() query.HistoryRequest {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.lastHistory
}

// LastAnnotate returns the last request Annotate received.
func (q *Querier) LastAnnotate() query.AnnotateRequest {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.lastAnnotate
}

// Why names the newest edit of req.Path as a content match, or MethodNone.
func (q *Querier) Why(ctx context.Context, req query.WhyRequest) (query.WhyResult, error) {
	q.mu.Lock()
	q.lastWhy = req
	err := q.err
	q.mu.Unlock()
	if err != nil {
		return query.WhyResult{}, err
	}
	res := query.WhyResult{Path: req.Path, Line: req.Line, Text: req.Text, Method: query.MethodNone}
	edits, _ := q.store.Edits(ctx, store.EditQuery{
		Worktree: req.Worktree, Repo: req.Repo, Path: req.Path, Limit: 1,
	})
	if len(edits) == 0 {
		return res, nil
	}
	turn, _ := q.store.Turn(ctx, edits[0].TurnID)
	sess, _ := q.store.Session(ctx, edits[0].SessionID)
	res.Method = query.MethodContent
	res.Edit = &edits[0]
	res.Turn = &turn.Turn
	res.Session = &sess.Session
	res.Candidates = []query.Candidate{{Edit: edits[0], Score: 1, Reason: "fixture"}}
	return res, nil
}

// History lists req.Path's edits newest first.
func (q *Querier) History(ctx context.Context, req query.HistoryRequest) ([]store.Edit, error) {
	q.mu.Lock()
	q.lastHistory = req
	err := q.err
	q.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return q.store.Edits(ctx, store.EditQuery{
		Worktree: req.Worktree, Repo: req.Repo, Path: req.Path, Limit: req.Limit,
	})
}

// Annotate answers as if the file held After: its one line attributed to
// the newest edit of req.Path in the worktree, or MethodNone.
func (q *Querier) Annotate(
	ctx context.Context,
	req query.AnnotateRequest,
) (query.AnnotateResult, error) {
	q.mu.Lock()
	q.lastAnnotate = req
	err := q.err
	q.mu.Unlock()
	if err != nil {
		return query.AnnotateResult{}, err
	}
	line := query.AnnotatedLine{Line: 1, Text: "package a", Method: query.MethodNone}
	edits, _ := q.store.Edits(ctx, store.EditQuery{
		Worktree: req.Worktree, Repo: req.Repo, Path: req.Path, Limit: 1,
	})
	if len(edits) > 0 {
		e := edits[0]
		line.Method, line.EditID, line.TurnID = query.MethodContent, e.ID, e.TurnID
		line.SessionID, line.AgentType = e.SessionID, e.AgentType
	}
	return query.AnnotateResult{
		Worktree: req.Worktree,
		Path:     req.Path,
		Lines:    []query.AnnotatedLine{line},
	}, nil
}
