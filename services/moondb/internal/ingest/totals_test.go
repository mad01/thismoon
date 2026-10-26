package ingest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/ingest/faketest"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// transcript is a turn's lines: the generated title, a model call that
// asks for a tool, the tool's failed result, and the closing call, both
// calls on claude-opus-5-5.
const transcript = `{"type":"ai-title","aiTitle":"Generated title"}
{"type":"api-request","timestamp":"2026-10-25T12:01:00Z"}
{"type":"assistant","timestamp":"2026-10-25T12:01:30Z","message":{"id":"m1",` +
	`"model":"claude-opus-5-5","content":[{"type":"tool_use","id":"tu1","name":"Bash"}],` +
	`"usage":{"input_tokens":1000,"output_tokens":500}}}
{"type":"user","timestamp":"2026-10-25T12:02:30Z",` +
	`"message":{"content":[{"type":"tool_result","tool_use_id":"tu1","is_error":true}]}}
{"type":"assistant","timestamp":"2026-10-25T12:02:50Z","message":{"id":"m2",` +
	`"model":"claude-opus-5-5","content":[{"type":"text","text":"done"}],` +
	`"usage":{"input_tokens":10,"output_tokens":20}}}
`

// missingChunk is a chunk key no store holds.
var missingChunk = strings.Repeat("f", 64)

// closeWith is a turn-close carrying lines as its transcript.
func closeWith(c chunkSet, session, turn string, minute int, lines string) spool.Envelope {
	env := closeEnv(c, session, turn, minute, "done")
	env.TurnClose.TranscriptChunk = c.add(lines)
	return env
}

// untitled clears the hook's session title, as a payload without one does.
func untitled(env spool.Envelope) spool.Envelope {
	env.Session.Title = ""
	return env
}

// named is a transcript line holding the name the person gave the session.
func named(name string) string {
	return `{"type":"agent-name","agentName":"` + name + `"}` + "\n"
}

// failStore wraps the fake store: PutTurn and PutSession fail with the
// errors set, Ping fails with ping, and PutTurn of turn panicOn panics.
type failStore struct {
	*faketest.Store
	putTurn, putSession, ping error
	panicOn                   string
}

func (s *failStore) PutTurn(ctx context.Context, t store.Turn) error {
	if t.ID == s.panicOn {
		panic("index out of range")
	}
	if s.putTurn != nil {
		return s.putTurn
	}
	return s.Store.PutTurn(ctx, t)
}

func (s *failStore) PutSession(ctx context.Context, v store.Session) error {
	if s.putSession != nil {
		return s.putSession
	}
	return s.Store.PutSession(ctx, v)
}

func (s *failStore) Ping(context.Context) error { return s.ping }

// seedStore writes records straight into the fixture's fake store, as an
// older serve left them.
func (f *fixture) seedStore(sessions []store.Session, turns ...store.Turn) {
	f.t.Helper()
	ctx := context.Background()
	for _, s := range sessions {
		if err := f.fake.PutSession(ctx, s); err != nil {
			f.t.Fatal(err)
		}
	}
	for _, tr := range turns {
		if err := f.fake.PutTurn(ctx, tr); err != nil {
			f.t.Fatal(err)
		}
	}
}

// chunk puts data into the fixture's store and returns its key.
func (f *fixture) chunk(data string) string {
	f.t.Helper()
	key, err := f.fake.Chunks().Put(context.Background(), []byte(data))
	if err != nil {
		f.t.Fatal(err)
	}
	return key
}

func TestTurnCloseStoresTotals(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(untitled(openEnv(c, "s1", "p1", 1, "<teammate-message id=\"x\">\nrun the build\n")), c)
	f.write(untitled(closeWith(c, "s1", "p1", 3, transcript)), c)
	f.drain()

	tr := f.turn("p1")
	want := trace.Stored(
		trace.Build(store.TurnDetail{Turn: tr.Turn}, []byte(transcript), trace.Prices{}).Totals,
	)
	got := tr.Totals
	if got == nil {
		t.Fatal("closed turn has no totals")
	}
	if got.WallMS != want.WallMS || got.ModelCalls != 2 || got.ToolCalls != 1 ||
		got.ToolErrors != 1 || got.WallMS != got.ModelMS+got.ToolMS+got.HarnessMS+got.UserMS {
		t.Errorf("totals = %+v, want 2 model calls, 1 failed tool call, the kinds adding up", got)
	}
	wantUsage := []store.ModelUsage{{
		Model: "claude-opus-5-5", Tokens: store.Tokens{Input: 1010, Output: 520},
	}}
	if len(got.Usage) != 1 || got.Usage[0] != wantUsage[0] {
		t.Errorf("usage = %+v, want %+v", got.Usage, wantUsage)
	}
	s := f.session("s1")
	if s.Title != "Generated title" || s.TitleSource != store.TitleGenerated ||
		s.PromptLine != "run the build" {
		t.Errorf("session title %q from %q, prompt line %q; want the transcript's generated "+
			"title and the prompt without its tag", s.Title, s.TitleSource, s.PromptLine)
	}
}

// TestSessionTitleRanks: a name the person gave outranks a generated title,
// the hook's session title counts as generated, within one rank the first
// title seen stays, and an import follows the same rule.
func TestSessionTitleRanks(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	hook := func(env spool.Envelope, title string) spool.Envelope {
		env.Session.Title = title
		return env
	}
	f.write(hook(openEnv(c, "s1", "p1", 1, "first prompt"), "Hook title"), c)
	f.write(hook(closeWith(c, "s1", "p1", 3, transcript), "Later hook title"), c)
	f.drain()
	if s := f.session("s1"); s.Title != "Hook title" || s.TitleSource != store.TitleGenerated {
		t.Errorf("after p1: title %q from %q; want the first generated title kept",
			s.Title, s.TitleSource)
	}

	f.write(untitled(openEnv(c, "s1", "p2", 4, "second prompt")), c)
	f.write(untitled(closeWith(c, "s1", "p2", 6, transcript+named("Named by the person"))), c)
	f.write(untitled(openEnv(c, "s1", "p3", 7, "third prompt")), c)
	f.write(untitled(closeWith(c, "s1", "p3", 9, named("Renamed later"))), c)
	imported := untitled(closeEnv(c, "s1", "p3", 9, "done"))
	imported.ID, imported.Backfill = "p3:import", true
	imported.Session.Title, imported.Session.TitleSource = "Imported name", store.TitleNamed
	f.write(imported, c)
	f.drain()
	s := f.session("s1")
	if s.Title != "Named by the person" || s.TitleSource != store.TitleNamed ||
		s.PromptLine != "first prompt" {
		t.Errorf("session title %q from %q, prompt line %q; want the first name over the "+
			"generated title, and the first prompt kept", s.Title, s.TitleSource, s.PromptLine)
	}

	f.write(hook(openEnv(c, "s2", "q1", 1, "prompt"), "Hook title"), c)
	imported = untitled(openEnv(c, "s2", "q1", 1, "prompt"))
	imported.ID, imported.Backfill = "q1:import", true
	imported.Session.Title, imported.Session.TitleSource = "Imported name", store.TitleNamed
	f.write(imported, c)
	f.drain()
	if s := f.session("s2"); s.Title != "Imported name" || s.TitleSource != store.TitleNamed {
		t.Errorf("s2 title %q from %q; want the imported name over the hook's title",
			s.Title, s.TitleSource)
	}
}

// TestReplayedCloseFillsSession: a close whose session fill failed changes
// nothing on the turn when it is tried again, and still fills the session.
func TestReplayedCloseFillsSession(t *testing.T) {
	f := newFixture(t)
	fs := &failStore{Store: f.fake}
	f.in.Store = fs
	c := chunkSet{}
	f.write(untitled(openEnv(c, "s1", "p1", 1, "prompt")), c)
	f.drain()

	r := &runner{in: f.in, attempts: map[string]int{}, retryAt: map[string]time.Time{}}
	fs.putSession = errors.New("disk full")
	f.write(untitled(closeWith(c, "s1", "p1", 3, transcript)), c)
	r.drain(context.Background())
	if f.turn("p1").Totals == nil || f.session("s1").Title != "" || f.pending() != 1 {
		t.Fatalf("after the failed fill: want the turn closed, no title, the close kept")
	}

	fs.putSession = nil
	f.now = f.now.Add(retryDelay)
	r.drain(context.Background())
	if s := f.session("s1"); s.Title != "Generated title" || f.pending() != 0 {
		t.Errorf("after the replay: title %q, %d pending; want the fill made", s.Title, f.pending())
	}
}

// TestReopenUnreadableTranscriptClearsTotals: a turn-open that moves a
// closed turn's start back recomputes its totals, and a transcript that
// cannot be read then clears them for the totals backfill instead of
// keeping a wall time that no longer holds.
func TestReopenUnreadableTranscriptClearsTotals(t *testing.T) {
	f := newFixture(t)
	f.seedStore([]store.Session{{ID: "s1", StartedAt: at(5)}}, store.Turn{
		ID: "p1", SessionID: "s1", Status: store.TurnClosed, StartedAt: at(5),
		EndedAt: new(at(8)), TranscriptChunk: missingChunk,
		Totals: &store.TurnTotals{Version: trace.TotalsVersion, WallMS: 3 * 60_000},
	})
	c := chunkSet{}
	env := untitled(openEnv(c, "s1", "p1", 1, "earlier prompt"))
	env.Backfill = true
	f.write(env, c)
	f.drain()

	tr := f.turn("p1")
	if !tr.StartedAt.Equal(at(1)) || tr.Totals != nil ||
		!store.NeedsTotals(tr.Turn, trace.TotalsVersion) {
		t.Errorf("turn start %v, totals %+v; want the start moved back and the totals "+
			"cleared for the totals backfill", tr.StartedAt, tr.Totals)
	}
	if !f.logged("leaving its totals to the totals backfill") {
		t.Errorf("logs = %q, want the unreadable transcript logged", f.logs)
	}
}

func TestBackfillTotals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	lines, prompt := f.chunk(transcript), f.chunk("old prompt\nsecond line")
	stale := &store.TurnTotals{Version: trace.TotalsVersion - 1}
	f.seedStore([]store.Session{{ID: "s1", StartedAt: at(0)}},
		store.Turn{
			ID: "t1", SessionID: "s1", PromptChunk: prompt, TranscriptChunk: lines,
			Status: store.TurnClosed, StartedAt: at(1), EndedAt: new(at(3)),
		},
		store.Turn{
			ID: "t2", SessionID: "s1", Status: store.TurnFailed, StartedAt: at(4),
			EndedAt: new(at(5)), Totals: stale,
		},
		store.Turn{ID: "t3", SessionID: "s1", Status: store.TurnOpen, StartedAt: at(6)},
		store.Turn{
			ID: "t4", SessionID: "s1", Status: store.TurnClosed, StartedAt: at(7),
			TranscriptChunk: missingChunk,
		},
	)

	skip := map[string]bool{}
	b, err := f.in.backfillTotals(ctx, 2, skip)
	if err != nil || b != (backfillBatch{tried: 2, computed: 2}) {
		t.Fatalf("first batch = %+v, %v; want t1 and t2 tried and computed", b, err)
	}
	if tot := f.turn("t1").Totals; tot == nil || tot.ModelCalls != 2 {
		t.Errorf("t1 totals = %+v, want its 2 model calls", tot)
	}
	if tot := f.turn("t2").Totals; tot == nil || tot.Version != trace.TotalsVersion {
		t.Errorf("t2 totals = %+v, want them computed again at the current version", tot)
	}
	s := f.session("s1")
	if s.Title != "Generated title" || s.PromptLine != "old prompt" {
		t.Errorf(
			"session title %q, prompt line %q; want them filled from t1",
			s.Title,
			s.PromptLine,
		)
	}

	b, err = f.in.backfillTotals(ctx, moondb.TotalsBackfillTurns, skip)
	if err != nil || b != (backfillBatch{tried: 1}) || !skip["t4"] || !f.logged("t4") {
		t.Fatalf("second batch = %+v, %v, skip %v; want unreadable t4 tried, skipped and logged",
			b, err, skip)
	}
	if tot := f.turn("t3").Totals; tot != nil {
		t.Errorf("open t3 got totals %+v, want none until it closes", tot)
	}
	if b, err := f.in.backfillTotals(ctx, moondb.TotalsBackfillTurns, skip); err != nil ||
		b.tried != 0 {
		t.Errorf("third batch = %+v, %v; want nothing left to try", b, err)
	}
}

// TestBackfillRecomputesVersionOneTotals: totals of version 1 gave a stretch
// with no line to the event that ends it, however long. The totals
// backfill computes them again for a turn and an agent, and the stretch
// past moondb.IdleGapAfter turns idle.
func TestBackfillRecomputesVersionOneTotals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	// The model answers 5 hours after the request: version 1 called it all
	// model time.
	const idleLines = `{"type":"user","isSidechain":true,"promptId":"p1",` +
		`"timestamp":"2026-10-25T12:01:00Z","message":{"content":"go"}}
{"type":"api-request","isSidechain":true,"timestamp":"2026-10-25T12:01:00Z"}
{"type":"assistant","isSidechain":true,"timestamp":"2026-10-25T17:01:00Z","message":` +
		`{"id":"m1","model":"claude-opus-5-5","stop_reason":"end_turn","content":[],` +
		`"usage":{"input_tokens":1,"output_tokens":1}}}
`
	turnLines := strings.ReplaceAll(idleLines, `"isSidechain":true,`, "")
	v1 := &store.TurnTotals{Version: 1, WallMS: 5 * 3_600_000, ModelMS: 5 * 3_600_000}
	f.seedStore([]store.Session{{ID: "s1", StartedAt: at(0)}}, store.Turn{
		ID: "t1", SessionID: "s1", TranscriptChunk: f.chunk(turnLines),
		Status: store.TurnClosed, StartedAt: at(1), EndedAt: new(at(301)), Totals: v1,
	})
	if err := f.fake.PutAgent(ctx, store.Agent{
		ID: "a1", SessionID: "s1", StartedAt: at(1), EndedAt: at(301),
		TranscriptChunk: f.chunk(idleLines), Totals: v1,
	}); err != nil {
		t.Fatal(err)
	}

	b, err := f.in.backfillTotals(ctx, moondb.TotalsBackfillTurns, map[string]bool{})
	if err != nil || b != (backfillBatch{tried: 2, computed: 2}) {
		t.Fatalf("batch = %+v, %v; want t1 and a1 computed again", b, err)
	}
	wantModel := moondb.IdleGapAfter.Milliseconds()
	wantIdle := 5*3_600_000 - wantModel
	for name, tot := range map[string]*store.TurnTotals{
		"t1": f.turn("t1").Totals, "a1": f.agent("a1").Totals,
	} {
		if tot == nil || tot.Version != trace.TotalsVersion || tot.ModelMS != wantModel ||
			tot.IdleMS != wantIdle {
			t.Errorf("%s totals = %+v; want version %d, %dms model and %dms idle",
				name, tot, trace.TotalsVersion, wantModel, wantIdle)
		}
	}
}

// TestBackfillStoreFailures: a store failure ends the batch after the turn
// it hit and skips nothing, so the turn is tried again; a failed session
// fill after the turn's write leaves the turn computed.
func TestBackfillStoreFailures(t *testing.T) {
	down := errors.New("store down")
	tests := []struct {
		name  string
		fail  failStore
		chunk string
		want  backfillBatch
		log   string
	}{
		{"turn write", failStore{putTurn: down}, "", backfillBatch{tried: 1}, "pausing"},
		{
			"turn write while down",
			failStore{putTurn: down, ping: down},
			"",
			backfillBatch{tried: 1},
			"store unavailable",
		},
		{
			"chunk read while down",
			failStore{ping: down},
			missingChunk,
			backfillBatch{tried: 1},
			"store unavailable",
		},
		{
			"session fill",
			failStore{putSession: down},
			"",
			backfillBatch{tried: 3, computed: 3},
			"keeps its totals",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			lines := f.chunk(transcript)
			if tc.chunk != "" {
				lines = tc.chunk
			}
			var turns []store.Turn
			for i, id := range []string{"t1", "t2", "t3"} {
				turns = append(turns, store.Turn{
					ID: id, SessionID: "s1", TranscriptChunk: lines, Status: store.TurnClosed,
					StartedAt: at(i), EndedAt: new(at(i + 1)),
				})
			}
			f.seedStore([]store.Session{{ID: "s1", StartedAt: at(0)}}, turns...)
			fs := tc.fail
			fs.Store = f.fake
			f.in.Store = &fs
			skip := map[string]bool{}
			b, err := f.in.backfillTotals(context.Background(), moondb.TotalsBackfillTurns, skip)
			if err != nil || b != tc.want || len(skip) != 0 || !f.logged(tc.log) {
				t.Errorf("batch = %+v, %v, skip %v, logs %q; want %+v, nothing skipped, %q "+
					"logged", b, err, skip, f.logs, tc.want, tc.log)
			}
		})
	}
}

// TestBackfillPanicSkipsTurn: a turn that panics is logged with the panic,
// stays skipped, and the batch goes on to the next turn.
func TestBackfillPanicSkipsTurn(t *testing.T) {
	f := newFixture(t)
	lines := f.chunk(transcript)
	f.seedStore([]store.Session{{ID: "s1", StartedAt: at(0)}},
		store.Turn{
			ID: "t1", SessionID: "s1", TranscriptChunk: lines, Status: store.TurnClosed,
			StartedAt: at(1), EndedAt: new(at(2)),
		},
		store.Turn{
			ID: "t2", SessionID: "s1", TranscriptChunk: lines, Status: store.TurnClosed,
			StartedAt: at(3), EndedAt: new(at(4)),
		},
	)
	f.in.Store = &failStore{Store: f.fake, panicOn: "t1"}
	ctx := context.Background()
	skip := map[string]bool{}
	b, err := f.in.backfillTotals(ctx, moondb.TotalsBackfillTurns, skip)
	if err != nil || b != (backfillBatch{tried: 2, computed: 1}) || !skip["t1"] ||
		!f.logged("turn t1 panicked: index out of range") {
		t.Fatalf("batch = %+v, %v, skip %v, logs %q; want t1 skipped with its panic logged "+
			"and t2 computed", b, err, skip, f.logs)
	}
	if f.turn("t2").Totals == nil {
		t.Error("t2 has no totals, want them computed after t1 panicked")
	}
	if b, err := f.in.backfillTotals(ctx, moondb.TotalsBackfillTurns, skip); err != nil ||
		b.tried != 0 {
		t.Errorf("next batch = %+v, %v; want the panicking turn left out", b, err)
	}
}

// TestPromptLineFills: a session whose first turn opened before prompt
// lines existed gets one from the totals backfill whether or not its turns
// need totals, and from a close of a later turn; a session whose first
// prompt has no text keeps none.
func TestPromptLineFills(t *testing.T) {
	f := newFixture(t)
	first, second := f.chunk("first prompt"), f.chunk("second prompt")
	current := &store.TurnTotals{Version: trace.TotalsVersion}
	f.seedStore(
		[]store.Session{
			{ID: "s1", StartedAt: at(0)},
			{ID: "s2", StartedAt: at(10)},
			{ID: "s3", StartedAt: at(20)},
		},
		store.Turn{
			ID: "a1", SessionID: "s1", PromptChunk: first, Status: store.TurnOpen,
			StartedAt: at(1),
		},
		store.Turn{
			ID: "b1", SessionID: "s2", PromptChunk: first, Status: store.TurnClosed,
			StartedAt: at(11), EndedAt: new(at(12)), Totals: current,
		},
		store.Turn{
			ID: "b2", SessionID: "s2", PromptChunk: second, Status: store.TurnOpen,
			StartedAt: at(13),
		},
		store.Turn{
			ID: "c1", SessionID: "s3", PromptChunk: f.chunk("<tag/>"),
			Status: store.TurnOpen, StartedAt: at(21),
		},
	)
	r := &runner{in: f.in, skip: map[string]bool{}}
	r.backfill(context.Background())
	for id, want := range map[string]string{"s1": "first prompt", "s2": "first prompt", "s3": ""} {
		if got := f.session(id).PromptLine; got != want {
			t.Errorf("%s prompt line = %q, want %q", id, got, want)
		}
	}
	if len(r.promptLines) != 0 || !r.listed {
		t.Errorf("queue %v, listed %v; want every session gone through once",
			r.promptLines, r.listed)
	}

	f.seedStore([]store.Session{{ID: "s4", StartedAt: at(30)}},
		store.Turn{
			ID: "d1", SessionID: "s4", PromptChunk: first, Status: store.TurnOpen,
			StartedAt: at(31),
		})
	c := chunkSet{}
	f.write(untitled(closeEnv(c, "s4", "d1", 32, "done")), c)
	f.drain()
	if got := f.session("s4").PromptLine; got != "first prompt" {
		t.Errorf("s4 prompt line after the close = %q, want its first prompt's", got)
	}
}

func TestTranscriptTitle(t *testing.T) {
	cases := []struct {
		name, lines string
		want        titleOffer
	}{
		{"none", `{"type":"user"}`, titleOffer{}},
		{
			"first generated stays", `{"type":"ai-title","aiTitle":"one"}` + "\n" +
				`{"type":"ai-title","aiTitle":"two"}`,
			titleOffer{"one", store.TitleGenerated},
		},
		{
			"name outranks a generated title", `{"type":"ai-title","aiTitle":"generated"}` +
				"\n" + named("mine") + named("later"),
			titleOffer{"mine", store.TitleNamed},
		},
		{
			"broken line", `{"type":"ai-title","aiTitle":` + "\n" +
				`{"type":"ai-title","aiTitle":" kept "}`,
			titleOffer{"kept", store.TitleGenerated},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := transcriptTitle([]byte(c.lines)); got != c.want {
				t.Errorf("transcriptTitle = %+v, want %+v", got, c.want)
			}
		})
	}
}
