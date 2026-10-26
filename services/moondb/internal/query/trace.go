package query

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// TurnTrace computes one turn's trace from its transcript chunk. The result
// is cached by the chunk's key together with the turn's times and edit
// count: an edit can land after the turn closed (the Bash hook is async,
// an import fills a turn later) and must not be hidden by an old trace.
func (s *Service) TurnTrace(ctx context.Context, d store.TurnDetail) (trace.TurnTrace, error) {
	if d.TranscriptChunk == "" {
		return trace.Build(d, nil, s.Prices), nil
	}
	key := traceKey(d)
	if tt, ok := s.traces().get(key); ok {
		return tt, nil
	}
	lines, err := s.Store.Chunks().Get(ctx, d.TranscriptChunk)
	if err != nil {
		return trace.TurnTrace{}, fmt.Errorf("transcript of turn %s: %w", d.ID, err)
	}
	tt := trace.Build(d, lines, s.Prices)
	s.traces().put(key, tt)
	return tt, nil
}

// AgentTrace computes one agent's trace from its agent transcript, with
// the edits it made. The result is cached by the chunk's key together
// with the agent's times, label and edit count, which a later record of
// the same agent, or an edit landing late, changes.
func (s *Service) AgentTrace(
	ctx context.Context,
	a store.Agent,
	edits []store.Edit,
) (trace.AgentTrace, error) {
	if a.TranscriptChunk == "" {
		return trace.BuildAgent(a, nil, edits, s.Prices), nil
	}
	key := agentKey(a, len(edits))
	s.traces()
	if at, ok := s.agentTraces.get(key); ok {
		return at, nil
	}
	lines, err := s.Store.Chunks().Get(ctx, a.TranscriptChunk)
	if err != nil {
		return trace.AgentTrace{}, fmt.Errorf("transcript of agent %s: %w", a.ID, err)
	}
	at := trace.BuildAgent(a, lines, edits, s.Prices)
	s.agentTraces.put(key, at)
	return at, nil
}

// AgentTraceByID loads one agent and computes its trace with the edits it
// made in its session. Returns store.ErrNotFound for an unknown agent.
func (s *Service) AgentTraceByID(
	ctx context.Context,
	id string,
) (store.Agent, trace.AgentTrace, error) {
	a, err := s.Store.Agent(ctx, id)
	if err != nil {
		return store.Agent{}, trace.AgentTrace{}, err
	}
	edits, err := s.Store.Edits(ctx, store.EditQuery{SessionID: a.SessionID})
	if err != nil {
		return store.Agent{}, trace.AgentTrace{}, err
	}
	at, err := s.AgentTrace(ctx, a, AgentEdits(edits, a.ID))
	return a, at, err
}

// TurnAgents computes the run of every agent with a record that ran under
// the turn (store.Agent.RanIn), each with the edits it made in the
// session; none when no agent did.
func (s *Service) TurnAgents(ctx context.Context, d store.TurnDetail) ([]trace.AgentRun, error) {
	agents, err := s.Store.Agents(ctx, d.SessionID)
	if err != nil {
		return nil, err
	}
	agents = slices.DeleteFunc(agents, func(a store.Agent) bool { return !a.RanIn(d.ID) })
	if len(agents) == 0 {
		return nil, nil
	}
	edits, err := s.Store.Edits(ctx, store.EditQuery{SessionID: d.SessionID})
	if err != nil {
		return nil, err
	}
	runs := make([]trace.AgentRun, 0, len(agents))
	for _, a := range agents {
		at, err := s.AgentTrace(ctx, a, AgentEdits(edits, a.ID))
		if err != nil {
			return nil, err
		}
		runs = append(runs, at.AgentRun)
	}
	return runs, nil
}

// AgentEdits keeps the edits the agent id made, in the order given.
func AgentEdits(edits []store.Edit, agentID string) []store.Edit {
	var out []store.Edit
	for _, e := range edits {
		if e.AgentID == agentID {
			out = append(out, e)
		}
	}
	return out
}

// SessionTrace computes the trace of every turn of a session, each with its
// prompt's label (trace.PromptLabel), the user time between them, and the
// run of every agent with a record, added to the turns it ran under.
// Returns store.ErrNotFound for an unknown session.
func (s *Service) SessionTrace(ctx context.Context, id string) (trace.SessionTrace, error) {
	sess, err := s.Store.Session(ctx, id)
	if err != nil {
		return trace.SessionTrace{}, err
	}
	turns := make([]trace.TurnTrace, 0, len(sess.Turns))
	var edits []store.Edit
	for _, t := range sess.Turns {
		d, err := s.Store.Turn(ctx, t.ID)
		if err != nil {
			return trace.SessionTrace{}, err
		}
		tt, err := s.TurnTrace(ctx, d)
		if err != nil {
			return trace.SessionTrace{}, err
		}
		tt.Prompt = s.promptLabel(ctx, d.PromptChunk)
		turns = append(turns, tt)
		edits = append(edits, d.Edits...)
	}
	agents := make([]trace.AgentTrace, 0, len(sess.Agents))
	for _, a := range sess.Agents {
		at, err := s.AgentTrace(ctx, a, AgentEdits(edits, a.ID))
		if err != nil {
			return trace.SessionTrace{}, err
		}
		agents = append(agents, at)
	}
	return trace.BuildSession(id, turns, agents), nil
}

// UsageCost prices stored usage with the Service's table: the USD of the
// priced part, nil when no part has a price, and the unpriced models.
func (s *Service) UsageCost(usage []store.ModelUsage) (cost *float64, unpriced []string) {
	return s.Prices.UsageCost(usage)
}

// promptLabel is a stored prompt's trace.PromptLabel; "" when the chunk is
// missing. A label is never worth failing the trace over.
func (s *Service) promptLabel(ctx context.Context, chunk string) string {
	if chunk == "" {
		return ""
	}
	data, err := s.Store.Chunks().Get(ctx, chunk)
	if err != nil {
		return ""
	}
	return trace.PromptLabel(string(data))
}

// traceKey names everything a turn's trace is built from but the price
// table, which is fixed for the life of the Service; agentKey likewise an
// agent's.
func traceKey(d store.TurnDetail) string {
	ended := int64(0)
	if d.EndedAt != nil {
		ended = d.EndedAt.UnixNano()
	}
	return fmt.Sprintf("%s|%d|%d|%d", d.TranscriptChunk, len(d.Edits),
		d.StartedAt.UnixNano(), ended)
}

func agentKey(a store.Agent, edits int) string {
	return fmt.Sprintf("%s|%d|%d|%d|%s|%d", a.TranscriptChunk, edits,
		a.StartedAt.UnixNano(), a.EndedAt.UnixNano(), a.Label(), a.Lines.Cut)
}

// traces returns the Service's turn trace cache, and makes both trace
// caches on first use so a zero Service works. Each holds
// moondb.TraceCacheTurns traces.
func (s *Service) traces() *traceCache[trace.TurnTrace] {
	s.cacheOnce.Do(func() {
		s.cache = newTraceCache[trace.TurnTrace](moondb.TraceCacheTurns)
		s.agentTraces = newTraceCache[trace.AgentTrace](moondb.TraceCacheTurns)
	})
	return s.cache
}

// traceCache keeps the most recently read traces, dropping the least
// recently read one past its size. Eviction scans every entry, which at a
// few hundred entries costs less than the parse it saves.
type traceCache[T any] struct {
	mu    sync.Mutex
	size  int
	clock uint64
	items map[string]*cached[T]
}

type cached[T any] struct {
	tt   T
	used uint64
}

func newTraceCache[T any](size int) *traceCache[T] {
	return &traceCache[T]{size: size, items: map[string]*cached[T]{}}
}

func (c *traceCache[T]) get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		var zero T
		return zero, false
	}
	c.clock++
	e.used = c.clock
	return e.tt, true
}

func (c *traceCache[T]) put(key string, tt T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clock++
	if _, ok := c.items[key]; !ok && len(c.items) >= c.size {
		c.evict()
	}
	c.items[key] = &cached[T]{tt: tt, used: c.clock}
}

// evict drops the least recently read entry; the caller holds mu.
func (c *traceCache[T]) evict() {
	oldest, at := "", uint64(0)
	for key, e := range c.items {
		if oldest == "" || e.used < at {
			oldest, at = key, e.used
		}
	}
	delete(c.items, oldest)
}

// count reports how many traces the cache holds.
func (c *traceCache[T]) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
