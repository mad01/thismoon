package ingest

import (
	"context"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb"
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
	if s.Title != "Generated title" || s.PromptLine != "run the build" {
		t.Errorf("session title %q, prompt line %q; want the transcript's title and the prompt "+
			"without its tag", s.Title, s.PromptLine)
	}
}

func TestSessionFillsNeverReplace(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	named := transcript + `{"type":"agent-name","agentName":"Named by the person"}` + "\n"
	f.write(untitled(openEnv(c, "s1", "p1", 1, "first prompt")), c)
	f.write(untitled(closeWith(c, "s1", "p1", 3, named)), c)
	f.write(untitled(openEnv(c, "s1", "p2", 4, "second prompt")), c)
	f.write(untitled(closeWith(c, "s1", "p2", 6, transcript)), c)
	f.drain()

	s := f.session("s1")
	if s.Title != "Named by the person" || s.PromptLine != "first prompt" {
		t.Errorf("session title %q, prompt line %q; want the person's name over the "+
			"generated title, and the first prompt kept", s.Title, s.PromptLine)
	}
}

func TestBackfillTotals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	lines, err := f.st.Chunks().Put(ctx, []byte(transcript))
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := f.st.Chunks().Put(ctx, []byte("old prompt\nsecond line"))
	if err != nil {
		t.Fatal(err)
	}
	stale := &store.TurnTotals{Version: trace.TotalsVersion - 1}
	seed := []store.Turn{
		{
			ID: "t1", SessionID: "s1", Seq: 1, PromptChunk: prompt, TranscriptChunk: lines,
			Status: store.TurnClosed, StartedAt: at(1), EndedAt: new(at(3)),
		},
		{
			ID: "t2", SessionID: "s1", Seq: 2, Status: store.TurnFailed, StartedAt: at(4),
			EndedAt: new(at(5)), Totals: stale,
		},
		{ID: "t3", SessionID: "s1", Seq: 3, Status: store.TurnOpen, StartedAt: at(6)},
		{
			ID: "t4", SessionID: "s1", Seq: 4, Status: store.TurnClosed, StartedAt: at(7),
			TranscriptChunk: strings.Repeat("f", 64),
		},
	}
	if err := f.st.PutSession(ctx, store.Session{ID: "s1", StartedAt: at(0)}); err != nil {
		t.Fatal(err)
	}
	for _, tr := range seed {
		if err := f.st.PutTurn(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	skip := map[string]bool{}
	n, err := f.in.backfillTotals(ctx, 1, skip)
	if err != nil || n != 1 {
		t.Fatalf("first batch = %d, %v; want 1 turn", n, err)
	}
	if !skip["t4"] || !f.logged("t4") {
		t.Errorf("unreadable t4: skip %v; want it skipped and logged", skip)
	}
	if tot := f.turn("t2").Totals; tot == nil || tot.Version != trace.TotalsVersion {
		t.Errorf("t2 totals = %+v, want them computed again at the current version", tot)
	}
	n, err = f.in.backfillTotals(ctx, moondb.TotalsBackfillTurns, skip)
	if err != nil || n != 1 {
		t.Fatalf("second batch = %d, %v; want t1 alone", n, err)
	}
	if tot := f.turn("t1").Totals; tot == nil || tot.ModelCalls != 2 {
		t.Errorf("t1 totals = %+v, want its 2 model calls", tot)
	}
	if tot := f.turn("t3").Totals; tot != nil {
		t.Errorf("open t3 got totals %+v, want none until it closes", tot)
	}
	s := f.session("s1")
	if s.Title != "Generated title" || s.PromptLine != "old prompt" {
		t.Errorf(
			"session title %q, prompt line %q; want them filled from t1",
			s.Title,
			s.PromptLine,
		)
	}
	if n, err := f.in.backfillTotals(ctx, moondb.TotalsBackfillTurns, skip); err != nil || n != 0 {
		t.Errorf("third batch = %d, %v; want nothing left", n, err)
	}
}

func TestTitleLine(t *testing.T) {
	long := strings.Repeat("ab ", 60)
	cases := []struct{ name, prompt, want string }{
		{"first line", "fix the bug\nand more", "fix the bug"},
		{"blank lines first", "\n  \n  fix it  \n", "fix it"},
		{
			"tag line skipped",
			"<teammate-message teammate_id=\"lead\" summary=\"x\">\nbuild it\n",
			"build it",
		},
		{"inline tags dropped", "<command-name>/review</command-name> now", "/review now"},
		{"spaces collapsed", "a \t  b", "a b"},
		{"only tags", "<a>\n</a>\n", ""},
		{
			"long",
			long,
			string([]rune(strings.TrimSpace(long))[:moondb.SessionPromptLineRunes]) + "…",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := titleLine(c.prompt); got != c.want {
				t.Errorf("titleLine(%q) = %q, want %q", c.prompt, got, c.want)
			}
		})
	}
}

func TestTranscriptTitle(t *testing.T) {
	cases := []struct{ name, lines, want string }{
		{"none", `{"type":"user"}`, ""},
		{"generated", `{"type":"ai-title","aiTitle":"one"}` + "\n" +
			`{"type":"ai-title","aiTitle":"two"}`, "two"},
		{"name wins", `{"type":"agent-name","agentName":"mine"}` + "\n" +
			`{"type":"ai-title","aiTitle":"generated"}`, "mine"},
		{"broken line", `{"type":"ai-title","aiTitle":` + "\n" +
			`{"type":"ai-title","aiTitle":"kept"}`, "kept"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := transcriptTitle([]byte(c.lines)); got != c.want {
				t.Errorf("transcriptTitle = %q, want %q", got, c.want)
			}
		})
	}
}
