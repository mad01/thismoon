package query

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// promptLineRunes caps the prompt line a session trace carries per turn.
const promptLineRunes = 120

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

// SessionTrace computes the trace of every turn of a session, each with its
// prompt's first line, and the user time between them. Returns
// store.ErrNotFound for an unknown session.
func (s *Service) SessionTrace(ctx context.Context, id string) (trace.SessionTrace, error) {
	sess, err := s.Store.Session(ctx, id)
	if err != nil {
		return trace.SessionTrace{}, err
	}
	turns := make([]trace.TurnTrace, 0, len(sess.Turns))
	for _, t := range sess.Turns {
		d, err := s.Store.Turn(ctx, t.ID)
		if err != nil {
			return trace.SessionTrace{}, err
		}
		tt, err := s.TurnTrace(ctx, d)
		if err != nil {
			return trace.SessionTrace{}, err
		}
		tt.Prompt = s.promptLine(ctx, d.PromptChunk)
		turns = append(turns, tt)
	}
	return trace.BuildSession(id, turns), nil
}

// UsageCost prices stored usage with the Service's table: the USD of the
// priced part, nil when no part has a price, and the unpriced models.
func (s *Service) UsageCost(usage []store.ModelUsage) (cost *float64, unpriced []string) {
	return s.Prices.UsageCost(usage)
}

// promptLine is the first non-blank line of a prompt, cut to
// promptLineRunes; "" when the chunk is missing. A label is never worth
// failing the trace over.
func (s *Service) promptLine(ctx context.Context, chunk string) string {
	if chunk == "" {
		return ""
	}
	data, err := s.Store.Chunks().Get(ctx, chunk)
	if err != nil {
		return ""
	}
	for line := range strings.Lines(string(data)) {
		if line = strings.TrimSpace(line); line != "" {
			return cutRunes(line, promptLineRunes)
		}
	}
	return ""
}

// cutRunes cuts s to n runes, marking a cut with an ellipsis.
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// traceKey names everything a turn's trace is built from but the price
// table, which is fixed for the life of the Service.
func traceKey(d store.TurnDetail) string {
	ended := int64(0)
	if d.EndedAt != nil {
		ended = d.EndedAt.UnixNano()
	}
	return fmt.Sprintf("%s|%d|%d|%d", d.TranscriptChunk, len(d.Edits),
		d.StartedAt.UnixNano(), ended)
}

// traces returns the Service's trace cache, made on first use so a zero
// Service works.
func (s *Service) traces() *traceCache {
	s.cacheOnce.Do(func() { s.cache = newTraceCache(moondb.TraceCacheTurns) })
	return s.cache
}

// traceCache keeps the most recently read turn traces, dropping the least
// recently read one past its size. Eviction scans every entry, which at a
// few hundred entries costs less than the parse it saves.
type traceCache struct {
	mu    sync.Mutex
	size  int
	clock uint64
	items map[string]*cached
}

type cached struct {
	tt   trace.TurnTrace
	used uint64
}

func newTraceCache(size int) *traceCache {
	return &traceCache{size: size, items: map[string]*cached{}}
}

func (c *traceCache) get(key string) (trace.TurnTrace, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return trace.TurnTrace{}, false
	}
	c.clock++
	e.used = c.clock
	return e.tt, true
}

func (c *traceCache) put(key string, tt trace.TurnTrace) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clock++
	if _, ok := c.items[key]; !ok && len(c.items) >= c.size {
		c.evict()
	}
	c.items[key] = &cached{tt: tt, used: c.clock}
}

// evict drops the least recently read entry; the caller holds mu.
func (c *traceCache) evict() {
	oldest, at := "", uint64(0)
	for key, e := range c.items {
		if oldest == "" || e.used < at {
			oldest, at = key, e.used
		}
	}
	delete(c.items, oldest)
}

// count reports how many traces the cache holds.
func (c *traceCache) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
