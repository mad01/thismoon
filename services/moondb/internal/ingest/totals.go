package ingest

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// transcript reads a turn's transcript lines; nil when it kept none.
func (in *Ingester) transcript(ctx context.Context, t store.Turn) ([]byte, error) {
	if t.TranscriptChunk == "" {
		return nil, nil
	}
	lines, err := in.Store.Chunks().Get(ctx, t.TranscriptChunk)
	if err != nil {
		return nil, fmt.Errorf("ingest: transcript of turn %s: %w", t.ID, err)
	}
	return lines, nil
}

// totals is what the trace of t over lines adds up to, as the Turn keeps
// it. Edits play no part in totals, so the turn alone is enough.
func (in *Ingester) totals(t store.Turn, lines []byte) *store.TurnTotals {
	tot := trace.Stored(trace.Build(store.TurnDetail{Turn: t}, lines, in.Prices).Totals)
	return &tot
}

// withTotals sets a closed or failed turn's totals from its transcript
// lines and returns the lines. A transcript the store cannot read is
// logged and clears the turn's totals, which no longer match its times,
// for the totals backfill to compute again: losing the close over it would
// lose more.
func (in *Ingester) withTotals(ctx context.Context, t *store.Turn) []byte {
	if t.Status == store.TurnOpen {
		return nil
	}
	lines, err := in.transcript(ctx, *t)
	if err != nil {
		in.logf("%v; leaving its totals to the totals backfill", err)
		t.Totals = nil
		return nil
	}
	t.Totals = in.totals(*t, lines)
	return lines
}

// fillSession offers the session a turn's title and fills its prompt line
// when it has none. The title goes through store.OfferTitle. The prompt
// line comes from the first of the session's turns, in Seq order, that
// has a prompt, so every path fills it from the same turn. The session is
// written only when either changed.
func (in *Ingester) fillSession(ctx context.Context, id string, offer titleOffer) error {
	d, err := in.Store.Session(ctx, id)
	if err != nil {
		return fmt.Errorf("ingest: load session %s: %w", id, err)
	}
	s := d.Session
	changed := store.OfferTitle(&s.Title, &s.TitleSource, offer.title, offer.source)
	changed = in.fillPromptLine(ctx, &s, d.Turns) || changed
	if !changed {
		return nil
	}
	return in.putSession(ctx, s)
}

// fillPromptLine sets an empty s.PromptLine from the first of turns that
// has a prompt and reports whether it changed. A first prompt without text
// leaves it empty.
func (in *Ingester) fillPromptLine(ctx context.Context, s *store.Session, turns []store.Turn) bool {
	if s.PromptLine != "" {
		return false
	}
	for _, t := range turns {
		if t.PromptChunk != "" {
			return fill(&s.PromptLine, in.promptLine(ctx, t.PromptChunk))
		}
	}
	return false
}

// promptLine is a stored prompt's trace.PromptLine; "" when the chunk is
// missing or holds no text. A label is never worth failing ingest over.
func (in *Ingester) promptLine(ctx context.Context, chunk string) string {
	if chunk == "" {
		return ""
	}
	data, err := in.Store.Chunks().Get(ctx, chunk)
	if err != nil {
		return ""
	}
	return trace.PromptLine(string(data))
}

// backfillBatch is what one totals backfill batch did: the turns it tried
// and, of those, the ones it computed.
type backfillBatch struct {
	tried, computed int
}

// backfillTotals tries up to limit stored turns that have no totals or
// older ones, in Store.TurnsWithoutTotals order, computes their totals,
// and offers their sessions the titles their lines give. When the turns
// leave room under limit, it tries stored agents the same way, in
// Store.AgentsWithoutTotals order. limit caps the records tried, not the
// ones computed, so a batch stays short whatever fails. A record whose
// chunk cannot be read, or that panics, is logged and added to skip, which
// later calls leave out: a turn by its id, an agent by agentSkipKey. A
// store failure ends the batch and leaves the record for the next one.
func (in *Ingester) backfillTotals(
	ctx context.Context,
	limit int,
	skip map[string]bool,
) (backfillBatch, error) {
	turns, err := in.Store.TurnsWithoutTotals(ctx, trace.TotalsVersion, limit+len(skip))
	if err != nil {
		return backfillBatch{}, fmt.Errorf("ingest: turns without totals: %w", err)
	}
	b, stopped := backfillEach(ctx, in, turns, limit, skip, turnBackfill(in))
	if stopped || b.tried == limit || ctx.Err() != nil {
		return b, nil
	}
	agents, err := in.Store.AgentsWithoutTotals(ctx, trace.TotalsVersion, limit-b.tried+len(skip))
	if err != nil {
		return b, fmt.Errorf("ingest: agents without totals: %w", err)
	}
	a, _ := backfillEach(ctx, in, agents, limit-b.tried, skip, agentBackfill(in))
	return backfillBatch{tried: b.tried + a.tried, computed: b.computed + a.computed}, nil
}

// backfiller is how backfillEach works on one kind of record: the key it
// is skipped by, its name in a log line, and the work.
type backfiller[T any] struct {
	key  func(T) string
	name func(T) string
	run  func(context.Context, T) (bool, error)
}

func turnBackfill(in *Ingester) backfiller[store.Turn] {
	return backfiller[store.Turn]{
		key:  func(t store.Turn) string { return t.ID },
		name: func(t store.Turn) string { return "turn " + t.ID },
		run:  in.backfillTurn,
	}
}

func agentBackfill(in *Ingester) backfiller[store.Agent] {
	return backfiller[store.Agent]{
		key:  func(a store.Agent) string { return agentSkipKey(a.ID) },
		name: func(a store.Agent) string { return "agent " + a.ID },
		run:  in.backfillAgent,
	}
}

// agentSkipKey is an agent's key in the backfill's skip set, apart from
// every turn id.
func agentSkipKey(id string) string { return "agent:" + id }

// backfillEach tries up to limit of items that skip leaves in, in order,
// and counts what it tried and computed. stopped is set when a store
// failure ended it.
func backfillEach[T any](
	ctx context.Context,
	in *Ingester,
	items []T,
	limit int,
	skip map[string]bool,
	bf backfiller[T],
) (b backfillBatch, stopped bool) {
	for _, item := range items {
		if b.tried == limit || ctx.Err() != nil {
			break
		}
		key := bf.key(item)
		if skip[key] {
			continue
		}
		b.tried++
		done, err := in.guardedBackfill(key, bf.name(item), skip, func() (bool, error) {
			return bf.run(ctx, item)
		})
		if err != nil {
			if ctx.Err() == nil {
				in.logf("ingest: totals backfill: %v; pausing until the next batch", err)
			}
			return b, true
		}
		if done {
			b.computed++
		}
	}
	return b, false
}

// guardedBackfill runs one record's backfill with a panic recovered, as
// the drain's guarded does for an envelope, so a transcript that trips a
// bug cannot crash serve a second after every restart. The record sits in
// skip by key while it runs, so a panic leaves it skipped until serve
// restarts; a run that computed the record or hit a store failure takes it
// out again. name says which record a log line is about.
func (in *Ingester) guardedBackfill(
	key, name string,
	skip map[string]bool,
	run func() (bool, error),
) (done bool, err error) {
	skip[key] = true
	defer func() {
		if p := recover(); p != nil {
			in.logf(
				"ingest: totals backfill: %s panicked: %v; skipping it until serve restarts\n%s",
				name,
				p,
				debug.Stack(),
			)
			done, err = false, nil
		}
	}()
	done, err = run()
	if done || err != nil {
		delete(skip, key)
	}
	return done, err
}

// backfillTurn computes one stored turn's totals and offers its session
// the title its lines give, and reports whether it computed them. It does
// not for a turn whose transcript chunk the store cannot read while it
// answers a Ping: that is logged, and the turn stays skipped. The error is
// a store failure: a failed turn write, or a failed chunk read while Ping
// fails too. A failed session fill after the turn's write is logged on its
// own, and the turn counts as computed.
func (in *Ingester) backfillTurn(ctx context.Context, t store.Turn) (bool, error) {
	lines, err := in.transcript(ctx, t)
	if err != nil {
		if down := in.storeDown(ctx, err); down != nil {
			return false, down
		}
		in.logf("ingest: totals backfill: %v; skipping the turn until serve restarts", err)
		return false, nil
	}
	t.Totals = in.totals(t, lines)
	if err := in.putTurn(ctx, t); err != nil {
		if down := in.storeDown(ctx, err); down != nil {
			return false, down
		}
		return false, err
	}
	if err := in.fillSession(ctx, t.SessionID, transcriptTitle(lines)); err != nil {
		in.logf("ingest: totals backfill: %v; turn %s keeps its totals", err, t.ID)
	}
	return true, nil
}

// storeDown is err as the store's failure when Ping fails too, else nil:
// the drain tells a store failure from an envelope's the same way.
func (in *Ingester) storeDown(ctx context.Context, err error) error {
	if perr := in.Store.Ping(ctx); perr != nil {
		return fmt.Errorf("store unavailable: %w (%v)", perr, err)
	}
	return nil
}

// sessionsWithoutPromptLine lists the sessions that have no prompt line,
// for the totals backfill to fill.
func (in *Ingester) sessionsWithoutPromptLine(ctx context.Context) ([]string, error) {
	list, err := in.Store.Sessions(ctx, store.SessionFilter{})
	if err != nil {
		return nil, fmt.Errorf("ingest: list sessions: %w", err)
	}
	var ids []string
	for _, s := range list {
		if s.PromptLine == "" {
			ids = append(ids, s.ID)
		}
	}
	return ids, nil
}

// fillPromptLines fills the prompt line of each session in ids that has
// none from its first prompt (fillSession), whether or not its turns need
// totals. It returns how many of ids it went through: all of them, or
// fewer with the store failure that stopped it.
func (in *Ingester) fillPromptLines(ctx context.Context, ids []string) (int, error) {
	for i, id := range ids {
		if ctx.Err() != nil {
			return i, nil
		}
		err := in.fillSession(ctx, id, titleOffer{})
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return i, err
		}
	}
	return len(ids), nil
}
