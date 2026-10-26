package query

import (
	"context"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/scorecard"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// SessionScorecard computes how a session spent its context: its trace,
// the tool calls in its turns' and agents' transcript lines, and what the
// events service and keeper-of-facts (Peers) say about it. A peer that
// does not answer leaves its part null. Returns store.ErrNotFound for an
// unknown session.
func (s *Service) SessionScorecard(ctx context.Context, id string) (scorecard.Scorecard, error) {
	sess, st, agents, err := s.sessionTraces(ctx, id)
	if err != nil {
		return scorecard.Scorecard{}, err
	}
	calls, counts := s.sessionCalls(ctx, sess)
	counts.CutHeads = cutHeads(st)
	in := scorecard.Input{
		Trace:      st,
		AgentSpans: make(map[string][]trace.Span, len(agents)),
		Calls:      calls,
	}
	for _, at := range agents {
		in.AgentSpans[at.AgentID] = at.Spans
		if at.LinesCut > 0 || at.Partial {
			counts.CutAgents++
		}
	}
	in.Transcripts = counts.Source()
	in.Events = scorecard.ReadEvents(ctx, s.Peers, id, sessionStart(st))
	in.Pins, in.Kof = scorecard.ReadPins(ctx, s.Peers, in.Events.Hints)
	return scorecard.Build(in), nil
}

// sessionCalls parses the tool calls of every turn and agent of a session,
// and counts the ones whose transcript lines the store lacks. A chunk that
// cannot be read counts as missing: the scorecard says so instead of
// failing.
func (s *Service) sessionCalls(
	ctx context.Context,
	sess store.SessionDetail,
) ([]scorecard.Call, scorecard.TranscriptCounts) {
	counts := scorecard.TranscriptCounts{Turns: len(sess.Turns), Agents: len(sess.Agents)}
	var calls []scorecard.Call
	for _, t := range sess.Turns {
		lines, err := s.chunk(ctx, t.TranscriptChunk)
		if err != nil || lines == nil {
			counts.MissingTurns++
			continue
		}
		calls = append(calls, scorecard.ParseCalls(lines, "")...)
	}
	for _, a := range sess.Agents {
		lines, err := s.chunk(ctx, a.TranscriptChunk)
		if err != nil || lines == nil {
			counts.MissingAgents++
			continue
		}
		calls = append(calls, scorecard.ParseCalls(lines, a.ID)...)
	}
	return calls, counts
}

// cutHeads counts the turns whose transcript lines start inside the turn.
func cutHeads(st trace.SessionTrace) int {
	n := 0
	for _, tt := range st.Turns {
		if tt.Lines.CutHead {
			n++
		}
	}
	return n
}

// sessionStart is when the session's first turn started; zero without
// turns. A session trace lists its turns by start.
func sessionStart(st trace.SessionTrace) time.Time {
	if len(st.Turns) == 0 {
		return time.Time{}
	}
	return st.Turns[0].Span.Start
}
