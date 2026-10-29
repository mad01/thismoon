package ingest

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// agentClose records a subagent as it stopped: its Agent record, with the
// totals of its agent transcript, and its session when the envelope is the
// first of it. A teammate stops after every message it works through, and
// each stop reads a longer file, so the newest record wins whole: a live
// envelope replaces the stored agent when it read as many lines or more,
// an import only when it read more. The spawn brief is the agent's first
// user line, the same in every stop, so a record keeps the one it has when
// a replacing envelope carries none. The later messages and their count
// only grow from stop to stop, so the longer list and the larger count
// stay. An envelope that does not replace the record still fills a brief
// it lacks, or messages it counted but did not keep (fill). A main
// segment's stop (store.Agent.IsMainSegment), which a spool written before
// record skipped them can still hold, records nothing.
func (in *Ingester) agentClose(
	ctx context.Context,
	env spool.Envelope,
	missing map[string]bool,
) error {
	p := env.AgentClose
	if p == nil {
		return errNoPayload(env)
	}
	if env.Agent.ID == "" {
		return fmt.Errorf("ingest: %s envelope %s has no agent id", env.Kind, env.ID)
	}
	if newAgent(env, p.TranscriptChunk).IsMainSegment() {
		return nil
	}
	if err := in.upsertSession(ctx, env); err != nil {
		return err
	}
	cur, err := in.Store.Agent(ctx, env.Agent.ID)
	switch {
	case err == nil && !replaces(env, cur, p):
		return in.fill(ctx, cur, in.kept(env, missing, p.BriefChunk),
			in.keptMessages(env, missing, p.Messages), p.LaterMessages)
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return fmt.Errorf("ingest: load agent %s: %w", env.Agent.ID, err)
	}
	a := newAgent(env, in.kept(env, missing, p.TranscriptChunk))
	a.BriefChunk = cmp.Or(in.kept(env, missing, p.BriefChunk), cur.BriefChunk)
	a.Messages = longer(in.keptMessages(env, missing, p.Messages), cur.Messages)
	a.LaterMessages = max(a.LaterMessages, cur.LaterMessages)
	in.withAgentTotals(ctx, &a)
	if err := in.Store.PutAgent(ctx, a); err != nil {
		return fmt.Errorf("ingest: put agent %s: %w", a.ID, err)
	}
	return nil
}

// fill gives a stored agent what an envelope that does not replace it
// carries and the record lacks: the spawn brief when it has none, and the
// later messages when it kept fewer, with their count. An agent stored
// before briefs, or before later messages, were kept gets them from a
// forced import.
func (in *Ingester) fill(
	ctx context.Context,
	a store.Agent,
	brief string,
	msgs []store.AgentMessage,
	later int,
) error {
	needBrief := a.BriefChunk == "" && brief != ""
	needMessages := len(msgs) > len(a.Messages)
	if !needBrief && !needMessages {
		return nil
	}
	if needBrief {
		a.BriefChunk = brief
	}
	if needMessages {
		a.Messages = msgs
	}
	a.LaterMessages = max(a.LaterMessages, later)
	if err := in.Store.PutAgent(ctx, a); err != nil {
		return fmt.Errorf("ingest: put agent %s: %w", a.ID, err)
	}
	return nil
}

// keptMessages is msgs without the ones whose chunk is in neither spool
// nor store (kept), so a record never names text it cannot serve.
func (in *Ingester) keptMessages(
	env spool.Envelope,
	missing map[string]bool,
	msgs []store.AgentMessage,
) []store.AgentMessage {
	var out []store.AgentMessage
	for _, m := range msgs {
		if in.kept(env, missing, m.Chunk) != "" {
			out = append(out, m)
		}
	}
	return out
}

// longer is the longer of two message lists, a when they are as long.
func longer(a, b []store.AgentMessage) []store.AgentMessage {
	if len(b) > len(a) {
		return b
	}
	return a
}

// replaces reports whether the envelope's agent replaces the stored one.
func replaces(env spool.Envelope, cur store.Agent, p *spool.AgentClose) bool {
	if env.Backfill {
		return p.Lines.Read > cur.Lines.Read
	}
	return p.Lines.Read >= cur.Lines.Read
}

// newAgent is the Agent an agent-close envelope records, its transcript
// at chunk. Without a stop time it ends when the envelope was written.
func newAgent(env spool.Envelope, chunk string) store.Agent {
	p := env.AgentClose
	ended := p.EndedAt
	if ended.IsZero() {
		ended = env.At
	}
	return store.Agent{
		ID:              env.Agent.ID,
		SessionID:       env.Session.ID,
		Type:            cmp.Or(p.Type, env.Agent.Type),
		Name:            p.Name,
		Description:     p.Description,
		Model:           p.Model,
		TurnIDs:         p.TurnIDs,
		Cwd:             p.Cwd,
		Location:        p.Location,
		StartedAt:       p.StartedAt,
		EndedAt:         ended,
		TranscriptChunk: chunk,
		Lines:           p.Lines,
		LaterMessages:   p.LaterMessages,
	}
}

// withAgentTotals sets an agent's totals from its agent transcript. A
// transcript the store cannot read is logged and leaves the totals to the
// totals backfill.
func (in *Ingester) withAgentTotals(ctx context.Context, a *store.Agent) {
	lines, err := in.agentTranscript(ctx, *a)
	if err != nil {
		in.logf("%v; leaving its totals to the totals backfill", err)
		a.Totals = nil
		return
	}
	a.Totals = in.agentTotals(*a, lines)
}

// agentTotals is what the trace of a over lines adds up to, as the Agent
// keeps it. Edits play no part in totals.
func (in *Ingester) agentTotals(a store.Agent, lines []byte) *store.TurnTotals {
	tot := trace.Stored(trace.BuildAgent(a, lines, nil, in.Prices).Totals)
	return &tot
}

// agentTranscript reads an agent's transcript; nil when it has none.
func (in *Ingester) agentTranscript(ctx context.Context, a store.Agent) ([]byte, error) {
	if a.TranscriptChunk == "" {
		return nil, nil
	}
	lines, err := in.Store.Chunks().Get(ctx, a.TranscriptChunk)
	if err != nil {
		return nil, fmt.Errorf("ingest: transcript of agent %s: %w", a.ID, err)
	}
	return lines, nil
}

// backfillAgent computes one stored agent's totals and reports whether it
// did, as backfillTurn does for a turn.
func (in *Ingester) backfillAgent(ctx context.Context, a store.Agent) (bool, error) {
	lines, err := in.agentTranscript(ctx, a)
	if err != nil {
		if down := in.storeDown(ctx, err); down != nil {
			return false, down
		}
		in.logf("ingest: totals backfill: %v; skipping the agent until serve restarts", err)
		return false, nil
	}
	a.Totals = in.agentTotals(a, lines)
	if err := in.Store.PutAgent(ctx, a); err != nil {
		if down := in.storeDown(ctx, err); down != nil {
			return false, down
		}
		return false, fmt.Errorf("ingest: put agent %s: %w", a.ID, err)
	}
	return true, nil
}
