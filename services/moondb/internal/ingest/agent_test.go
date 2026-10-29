package ingest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// agentLines is an agent transcript with one model call under turn p1.
const agentLines = `{"type":"user","isSidechain":true,"promptId":"p1",` +
	`"timestamp":"2026-10-25T12:01:00Z","message":{"content":"go"}}
{"type":"assistant","isSidechain":true,"timestamp":"2026-10-25T12:01:30Z","message":` +
	`{"id":"am1","model":"claude-opus-5-5","stop_reason":"end_turn","content":[],` +
	`"usage":{"input_tokens":100,"output_tokens":10}}}
`

// agentEnv is agent's stop at minute after it read read lines.
func agentEnv(c chunkSet, session, agent string, minute, read int) spool.Envelope {
	env := base(spool.KindAgentClose, fmt.Sprintf("%s:close:%d", agent, read), session, "",
		minute)
	env.Agent = spool.AgentRef{ID: agent, Type: "worker"}
	env.AgentClose = &spool.AgentClose{
		Type: "general-purpose", Name: "worker", Model: "claude-opus-5-5",
		TurnIDs: []string{"p1"}, Cwd: worktree, StartedAt: at(1), EndedAt: at(minute),
		TranscriptChunk: c.add(agentLines),
		Lines:           store.AgentLines{Read: read, Kept: 2},
	}
	return env
}

func (f *fixture) agent(id string) store.Agent {
	f.t.Helper()
	a, err := f.st.Agent(context.Background(), id)
	if err != nil {
		f.t.Fatalf("agent %s: %v", id, err)
	}
	return a
}

// TestAgentCloseRecordsAgent: a stop records the agent with the totals of
// its transcript, creates its session, and replayed changes nothing.
func TestAgentCloseRecordsAgent(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	env := agentEnv(c, "s1", "a1", 3, 2)
	f.write(env, c)
	f.drain()
	a := f.agent("a1")
	if a.SessionID != "s1" || a.Name != "worker" || a.Type != "general-purpose" ||
		!a.EndedAt.Equal(at(3)) || a.TranscriptChunk == "" || a.Cwd != worktree {
		t.Errorf("agent = %+v", a)
	}
	if a.Totals == nil || a.Totals.ModelCalls != 1 || a.Totals.Version != trace.TotalsVersion {
		t.Errorf("totals = %+v, want the one model call at the current version", a.Totals)
	}
	if s := f.session("s1"); s.Cwd != worktree {
		t.Errorf("session = %+v, want it created from the envelope", s)
	}
	f.write(env, c)
	f.drain()
	if list, err := f.st.Agents(context.Background(), "s1"); err != nil || len(list) != 1 {
		t.Errorf("agents after a replay = %+v, %v; want the one", list, err)
	}
}

// TestAgentCloseReplaces: a live stop replaces the stored agent when it
// read as many lines or more, an import only when it read more.
func TestAgentCloseReplaces(t *testing.T) {
	cases := []struct {
		name     string
		read     int
		backfill bool
		want     bool
	}{
		{"live, more lines", 11, false, true},
		{"live, same lines", 10, false, true},
		{"live, fewer lines", 9, false, false},
		{"import, same lines", 10, true, false},
		{"import, more lines", 11, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			c := chunkSet{}
			f.write(agentEnv(c, "s1", "a1", 3, 10), c)
			f.drain()
			env := agentEnv(c, "s1", "a1", 5, tc.read)
			env.Backfill = tc.backfill
			env.AgentClose.Name = "renamed"
			f.write(env, c)
			f.drain()
			if got := f.agent("a1").Name == "renamed"; got != tc.want {
				t.Errorf("replaced = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAgentCloseBrief: a stop records the agent's spawn brief once and its
// count of later messages. An import that does not replace the record
// fills a missing brief with its count and leaves a stored one alone, and
// a replacing stop without a brief keeps the stored one.
func TestAgentCloseBrief(t *testing.T) {
	cases := []struct {
		name               string
		stored, next, want string
		read               int
		backfill           bool
		later, wantLater   int
	}{
		{
			name: "first stop", next: "fix the parser", want: "fix the parser",
			read: 10, later: 1, wantLater: 1,
		},
		{
			name: "import fills a record from before briefs", stored: "-",
			next: "fix the parser", want: "fix the parser", read: 10, backfill: true,
			later: 2, wantLater: 2,
		},
		{
			name: "import keeps the stored brief", stored: "fix the parser", next: "other",
			want: "fix the parser", read: 10, backfill: true, later: 2, wantLater: 0,
		},
		{
			name: "replacing stop without one keeps it", stored: "fix the parser",
			want: "fix the parser", read: 11, later: 3, wantLater: 3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			c := chunkSet{}
			if tc.stored != "" {
				first := agentEnv(c, "s1", "a1", 3, 10)
				if tc.stored != "-" {
					first.AgentClose.BriefChunk = c.add(tc.stored)
				}
				f.write(first, c)
				f.drain()
			}
			env := agentEnv(c, "s1", "a1", 5, tc.read)
			env.Backfill = tc.backfill
			if tc.next != "" {
				env.AgentClose.BriefChunk = c.add(tc.next)
			}
			env.AgentClose.LaterMessages = tc.later
			f.write(env, c)
			f.drain()
			got := f.agent("a1")
			if got.BriefChunk != spool.ChunkKey([]byte(tc.want)) {
				t.Errorf("brief chunk = %q, want the one of %q", got.BriefChunk, tc.want)
			}
			if got.LaterMessages != tc.wantLater {
				t.Errorf("later messages = %d, want %d", got.LaterMessages, tc.wantLater)
			}
		})
	}
}

// TestAgentCloseMessages: a stop records the agent's later messages. An
// import that does not replace the record fills messages it counted but
// did not keep, as a record from before they were kept, and leaves a
// fuller list alone. A replacing stop with fewer, as an older build's
// spool carries, keeps the stored ones, and a message whose chunk is gone
// is left out.
func TestAgentCloseMessages(t *testing.T) {
	two := []string{"Go with C.", "Tighten the scan."}
	cases := []struct {
		name        string
		stored      []string // nil: no record before the envelope
		storedLater int
		next        []string
		read        int
		backfill    bool
		gone        bool // the envelope's last message has no chunk
		want        []string
		wantLater   int
	}{
		{name: "first stop", next: two, read: 10, want: two, wantLater: 2},
		{
			name: "import fills a record from before messages", stored: []string{},
			storedLater: 2, next: two, read: 10, backfill: true, want: two, wantLater: 2,
		},
		{
			name: "import keeps a fuller list", stored: two, storedLater: 2,
			next: two[:1], read: 10, backfill: true, want: two, wantLater: 2,
		},
		{
			name: "replacing stop with fewer keeps them", stored: two, storedLater: 2,
			read: 11, want: two, wantLater: 2,
		},
		{
			name: "replacing stop with more", stored: two[:1], storedLater: 1,
			next: append(two, "Land."), read: 11, want: append(two, "Land."), wantLater: 3,
		},
		{name: "a gone chunk", next: two, read: 10, gone: true, want: two[:1], wantLater: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			c := chunkSet{}
			if tc.stored != nil {
				first := agentEnv(c, "s1", "a1", 3, 10)
				first.AgentClose.BriefChunk = c.add("build it")
				first.AgentClose.Messages = messages(c, tc.stored)
				first.AgentClose.LaterMessages = tc.storedLater
				f.write(first, c)
				f.drain()
			}
			env := agentEnv(c, "s1", "a1", 5, tc.read)
			env.Backfill = tc.backfill
			env.AgentClose.BriefChunk = c.add("build it")
			env.AgentClose.Messages = messages(c, tc.next)
			env.AgentClose.LaterMessages = len(tc.next)
			if tc.gone {
				delete(c, env.AgentClose.Messages[len(tc.next)-1].Chunk)
			}
			f.write(env, c)
			f.drain()
			got := f.agent("a1")
			if !slices.Equal(chunkKeys(got.Messages), chunkKeys(messages(chunkSet{}, tc.want))) {
				t.Errorf("messages = %+v, want the chunks of %q", got.Messages, tc.want)
			}
			if got.LaterMessages != tc.wantLater ||
				got.BriefChunk != spool.ChunkKey([]byte("build it")) {
				t.Errorf("later messages %d, brief %q; want %d and the brief kept",
					got.LaterMessages, got.BriefChunk, tc.wantLater)
			}
		})
	}
}

// messages are later messages with texts, a minute apart, their chunks
// added to c.
func messages(c chunkSet, texts []string) []store.AgentMessage {
	var out []store.AgentMessage
	for i, text := range texts {
		out = append(out, store.AgentMessage{At: at(2 + i), From: "team-lead", Chunk: c.add(text)})
	}
	return out
}

func chunkKeys(msgs []store.AgentMessage) []string {
	var out []string
	for _, m := range msgs {
		out = append(out, m.Chunk)
	}
	return out
}

// TestAgentCloseMissingChunk records the agent without its transcript and
// says so.
func TestAgentCloseMissingChunk(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	env := agentEnv(c, "s1", "a1", 3, 2)
	env.AgentClose.TranscriptChunk = missingChunk
	f.write(env, chunkSet{})
	f.drain()
	if a := f.agent("a1"); a.TranscriptChunk != "" || !f.logged("neither spool nor store") {
		t.Errorf("agent = %+v, logs %q; want no transcript and a warning", a, f.logs)
	}
}

// TestAgentCloseDropsMainSegment: a main segment's stop, as record spooled
// it before it skipped them (no agent_type, no line read, no transcript),
// leaves the spool and records neither an agent nor a session.
func TestAgentCloseDropsMainSegment(t *testing.T) {
	f := newFixture(t)
	env := base(spool.KindAgentClose, "a67e:close:0", "s1", "", 3)
	env.Agent = spool.AgentRef{ID: "a67e"}
	env.AgentClose = &spool.AgentClose{Type: store.InternalAgent, StartedAt: at(3), EndedAt: at(3)}
	f.write(env, chunkSet{})
	f.drain()
	ctx := context.Background()
	if a, err := f.st.Agent(ctx, "a67e"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("agent = %+v, %v; want none", a, err)
	}
	if _, err := f.st.Session(ctx, "s1"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("session s1: %v; want none", err)
	}
	if n := f.pending(); n != 0 {
		t.Errorf("%d envelopes pending, want the stop gone from the spool", n)
	}
}

// TestAgentCloseKeepsInternalAgentWithLines: an internal agent with lines
// read and an agent transcript is an Agent, not a main segment: its stop
// records it and its session.
func TestAgentCloseKeepsInternalAgentWithLines(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	env := agentEnv(c, "s1", "a68f", 3, 2)
	env.Agent.Type = ""
	env.AgentClose.Type, env.AgentClose.Name = store.InternalAgent, ""
	f.write(env, c)
	f.drain()
	if a := f.agent("a68f"); a.Type != store.InternalAgent || a.TranscriptChunk == "" ||
		a.Lines.Read != 2 {
		t.Errorf("agent = %+v, want the internal agent with its transcript", a)
	}
	if s := f.session("s1"); s.ID != "s1" {
		t.Errorf("session = %+v, want it created from the envelope", s)
	}
}

// TestAgentCloseNeedsAgentID: an agent-close without an agent id names
// nothing to record.
func TestAgentCloseNeedsAgentID(t *testing.T) {
	f := newFixture(t)
	env := agentEnv(chunkSet{}, "s1", "a1", 3, 2)
	env.Agent.ID = ""
	env.AgentClose.TranscriptChunk = ""
	if err := f.in.Ingest(context.Background(), env); err == nil {
		t.Error("Ingest took an agent-close with no agent id")
	}
	if _, err := f.st.Agent(context.Background(), ""); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("an agent was stored: %v", err)
	}
}

// TestBackfillAgentTotals: the totals backfill computes the agents' totals
// once the turns leave room in a batch, and skips an agent whose
// transcript is gone by a key no turn id can take.
func TestBackfillAgentTotals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	lines := f.chunk(agentLines)
	f.seedStore([]store.Session{{ID: "s1", StartedAt: at(0)}}, store.Turn{
		ID: "t1", SessionID: "s1", TranscriptChunk: f.chunk(transcript),
		Status: store.TurnClosed, StartedAt: at(1), EndedAt: new(at(3)),
	})
	for _, a := range []store.Agent{
		{ID: "a1", SessionID: "s1", StartedAt: at(1), EndedAt: at(2), TranscriptChunk: lines},
		{ID: "a2", SessionID: "s1", StartedAt: at(2), EndedAt: at(3), TranscriptChunk: missingChunk},
	} {
		if err := f.fake.PutAgent(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	skip := map[string]bool{}
	b, err := f.in.backfillTotals(ctx, 2, skip)
	if err != nil || b != (backfillBatch{tried: 2, computed: 2}) {
		t.Fatalf("first batch = %+v, %v; want t1 and a1 computed", b, err)
	}
	if tot := f.agent("a1").Totals; tot == nil || tot.ModelCalls != 1 {
		t.Errorf("a1 totals = %+v, want its model call", tot)
	}
	b, err = f.in.backfillTotals(ctx, moondb.TotalsBackfillTurns, skip)
	if err != nil || b != (backfillBatch{tried: 1}) || !skip[agentSkipKey("a2")] ||
		!f.logged("transcript of agent a2") {
		t.Fatalf("second batch = %+v, %v, skip %v; want a2 tried, skipped and logged", b, err, skip)
	}
}

// TestTurnOpenOrigin: a turn-open records what started the turn, and fills
// it on a turn an early edit left without one.
func TestTurnOpenOrigin(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	env := openEnv(c, "s1", "p1", 1, "<task-notification>\n</task-notification>")
	env.TurnOpen.Origin = store.OriginTask
	f.write(editEnv(c, "s1", "p2", "tu1", 2, "a.go", "x", ""), c)
	f.write(env, c)
	later := openEnv(c, "s1", "p2", 1, "hi")
	later.TurnOpen.Origin = store.OriginPerson
	f.write(later, c)
	f.drain()
	if o := f.turn("p1").Origin; o != store.OriginTask {
		t.Errorf("p1 origin = %q, want task", o)
	}
	if o := f.turn("p2").Origin; o != store.OriginPerson {
		t.Errorf("p2 origin = %q, want person filled on the placeholder", o)
	}
}
