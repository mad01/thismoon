package ingest

import (
	"context"
	"fmt"

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
// logged and leaves the turn without totals, for the backfill to try
// again: losing the close over it would lose more.
func (in *Ingester) withTotals(ctx context.Context, t *store.Turn) []byte {
	if t.Status == store.TurnOpen {
		return nil
	}
	lines, err := in.transcript(ctx, *t)
	if err != nil {
		in.logf("%v; leaving its totals to the backfill", err)
		return nil
	}
	t.Totals = in.totals(*t, lines)
	return lines
}

// fillSession sets the session's title and prompt line where they are
// empty; a value the session has is never replaced.
func (in *Ingester) fillSession(ctx context.Context, id, title, promptLine string) error {
	if title == "" && promptLine == "" {
		return nil
	}
	d, err := in.Store.Session(ctx, id)
	if err != nil {
		return fmt.Errorf("ingest: load session %s: %w", id, err)
	}
	s := d.Session
	changed := fill(&s.Title, title)
	changed = fill(&s.PromptLine, promptLine) || changed
	if !changed {
		return nil
	}
	return in.putSession(ctx, s)
}

// promptLine is the title line of a stored prompt; "" when the chunk is
// missing or holds no text. A label is never worth failing ingest over.
func (in *Ingester) promptLine(ctx context.Context, chunk string) string {
	if chunk == "" {
		return ""
	}
	data, err := in.Store.Chunks().Get(ctx, chunk)
	if err != nil {
		return ""
	}
	return titleLine(string(data))
}

// backfillTotals computes the totals of up to limit stored turns that have
// none or older ones, newest first, and fills the session titles and
// prompt lines those turns give. A turn it cannot compute is logged and
// added to skip, which later calls leave out, and the batch looks further
// for one in its place. It returns how many turns it computed.
func (in *Ingester) backfillTotals(
	ctx context.Context,
	limit int,
	skip map[string]bool,
) (int, error) {
	n := 0
	for n < limit && ctx.Err() == nil {
		turns, err := in.Store.TurnsWithoutTotals(ctx, trace.TotalsVersion, limit+len(skip))
		if err != nil {
			return n, fmt.Errorf("ingest: turns without totals: %w", err)
		}
		failed := false
		for _, t := range turns {
			if n == limit || ctx.Err() != nil {
				break
			}
			if skip[t.ID] {
				continue
			}
			if err := in.backfillTurn(ctx, t); err != nil {
				in.logf("ingest: totals backfill: %v; skipping the turn until serve restarts", err)
				skip[t.ID], failed = true, true
				continue
			}
			n++
		}
		if !failed {
			break // every candidate was computed or skipped before: none is left
		}
	}
	return n, nil
}

// backfillTurn computes one stored turn's totals and fills its session's
// title from its lines and, for the first turn, its prompt line.
func (in *Ingester) backfillTurn(ctx context.Context, t store.Turn) error {
	lines, err := in.transcript(ctx, t)
	if err != nil {
		return err
	}
	t.Totals = in.totals(t, lines)
	if err := in.putTurn(ctx, t); err != nil {
		return err
	}
	prompt := ""
	if t.Seq == 1 {
		prompt = in.promptLine(ctx, t.PromptChunk)
	}
	return in.fillSession(ctx, t.SessionID, transcriptTitle(lines), prompt)
}
