package trace

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// line is one transcript line of an idle test: its time from t0, its kind
// and an id. A call line is model message msg_<id> asking for tool call
// toolu_<id>, a result line answers that call, and a reply line is model
// message msg_<id> ending with end_turn.
type line struct {
	at   time.Duration
	kind string // prompt, request, call, reply, result, stop, teammate
	id   string
}

// jsonl renders lines as a transcript; sidechain marks them an agent's.
func jsonl(lines []line, sidechain bool) []byte {
	var b strings.Builder
	for _, l := range lines {
		head := fmt.Sprintf(`"timestamp":%q`, t0.Add(l.at).Format(time.RFC3339Nano))
		if sidechain {
			head += `,"isSidechain":true,"agentId":"agent-i"`
		}
		switch l.kind {
		case "prompt", "teammate":
			fmt.Fprintf(
				&b,
				`{"type":"user",%s,"promptId":"p-idle","message":{"role":"user","content":"go"}}`,
				head,
			)
		case "request":
			fmt.Fprintf(&b, `{"type":"api-request",%s}`, head)
		case "call":
			fmt.Fprintf(
				&b,
				`{"type":"assistant",%s,"message":{"id":"msg_%s","model":"claude-opus-5-5",`+
					`"stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_%[2]s","name":"Bash",`+
					`"input":{}}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
				head,
				l.id,
			)
		case "reply":
			fmt.Fprintf(
				&b,
				`{"type":"assistant",%s,"message":{"id":"msg_%s","model":"claude-opus-5-5",`+
					`"stop_reason":"end_turn","content":[{"type":"text","text":"done"}],`+
					`"usage":{"input_tokens":1,"output_tokens":1}}}`,
				head,
				l.id,
			)
		case "result":
			fmt.Fprintf(&b, `{"type":"user",%s,"promptId":"p-idle","message":{"role":"user",`+
				`"content":[{"type":"tool_result","tool_use_id":"toolu_%s","content":"ok"}]}}`, head, l.id)
		case "stop":
			fmt.Fprintf(&b, `{"type":"system","subtype":"stop_hook_summary",%s}`, head)
		}
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// gapsOf lists a trace's gaps as kind and duration, "model 30m0s".
func gapsOf(gaps []Gap) []string {
	out := make([]string, 0, len(gaps))
	for _, g := range gaps {
		out = append(out, fmt.Sprintf("%s %s", g.Kind, g.End.Sub(g.Start)))
	}
	return out
}

// TestBuildIdle: a model, tool or harness stretch longer than
// moondb.IdleGapAfter keeps its first IdleGapAfter in its own kind and
// the rest is idle; a shorter one and user time stay whole.
func TestBuildIdle(t *testing.T) {
	const idle = moondb.IdleGapAfter
	tests := []struct {
		name     string
		lines    []line
		ended    time.Duration
		wantGaps []string
		want     Totals
	}{
		{
			// A laptop asleep between a tool result and the model's answer:
			// the 5 hours end at an assistant line, so version 1 gave them all
			// to the model.
			name: "a 5-hour wait for the model keeps 30 minutes of model time",
			lines: []line{
				{0, "prompt", ""},
				{time.Second, "call", "a"},
				{2 * time.Second, "result", "a"},
				{2*time.Second + 5*time.Hour, "reply", "b"},
				{3*time.Second + 5*time.Hour, "stop", ""},
			},
			ended: 4*time.Second + 5*time.Hour,
			wantGaps: []string{
				"model 1s", "tool 1s", "model 30m0s", "idle 4h30m0s", "harness 1s", "user 1s",
			},
			want: Totals{
				ModelMS: (time.Second + idle).Milliseconds(), ToolMS: 1000, HarnessMS: 1000,
				UserMS: 1000, IdleMS: (4*time.Hour + 30*time.Minute).Milliseconds(),
			},
		},
		{
			name: "a 20-minute wait for the model stays model time",
			lines: []line{
				{0, "prompt", ""},
				{time.Second, "call", "a"},
				{2 * time.Second, "result", "a"},
				{2*time.Second + 20*time.Minute, "reply", "b"},
				{3*time.Second + 20*time.Minute, "stop", ""},
			},
			ended:    4*time.Second + 20*time.Minute,
			wantGaps: []string{"model 1s", "tool 1s", "model 20m0s", "harness 1s", "user 1s"},
			want: Totals{
				ModelMS: (time.Second + 20*time.Minute).Milliseconds(), ToolMS: 1000,
				HarnessMS: 1000, UserMS: 1000,
			},
		},
		{
			name: "a stretch of exactly IdleGapAfter has no idle time",
			lines: []line{
				{0, "prompt", ""},
				{time.Second, "call", "a"},
				{time.Second + idle, "result", "a"},
				{2*time.Second + idle, "reply", "b"},
				{3*time.Second + idle, "stop", ""},
			},
			ended:    3*time.Second + idle,
			wantGaps: []string{"model 1s", "tool 30m0s", "model 1s", "harness 1s"},
			want: Totals{
				ModelMS: 2000, ToolMS: idle.Milliseconds(), HarnessMS: 1000,
			},
		},
		{
			// A request that hung for 2 hours: the stretch ends at the API
			// request line, which is harness time.
			name: "a 2-hour harness stretch keeps 30 minutes of harness time",
			lines: []line{
				{0, "prompt", ""},
				{time.Second, "call", "a"},
				{2 * time.Second, "result", "a"},
				{2*time.Second + 2*time.Hour, "request", ""},
				{3*time.Second + 2*time.Hour, "reply", "b"},
				{4*time.Second + 2*time.Hour, "stop", ""},
			},
			ended: 4*time.Second + 2*time.Hour,
			wantGaps: []string{
				"model 1s", "tool 1s", "harness 30m0s", "idle 1h30m0s", "model 1s", "harness 1s",
			},
			want: Totals{
				ModelMS: 2000, ToolMS: 1000, HarnessMS: 1000 + idle.Milliseconds(),
				IdleMS: (90 * time.Minute).Milliseconds(),
			},
		},
		{
			name: "a 3-hour tool call keeps 30 minutes of tool time",
			lines: []line{
				{0, "prompt", ""},
				{time.Second, "call", "a"},
				{time.Second + 3*time.Hour, "result", "a"},
				{2*time.Second + 3*time.Hour, "reply", "b"},
				{3*time.Second + 3*time.Hour, "stop", ""},
			},
			ended:    3*time.Second + 3*time.Hour,
			wantGaps: []string{"model 1s", "tool 30m0s", "idle 2h30m0s", "model 1s", "harness 1s"},
			want: Totals{
				ModelMS: 2000, ToolMS: idle.Milliseconds(), HarnessMS: 1000,
				IdleMS: (150 * time.Minute).Milliseconds(),
			},
		},
		{
			// No Stop line, and the turn's recorded end comes an hour after
			// the last line: that tail is harness time, so it turns idle too.
			name: "an hour-long harness tail keeps 30 minutes of harness time",
			lines: []line{
				{0, "prompt", ""}, {time.Second, "reply", "a"},
			},
			ended:    time.Second + time.Hour,
			wantGaps: []string{"model 1s", "harness 30m0s", "idle 30m0s"},
			want: Totals{
				ModelMS: 1000, HarnessMS: idle.Milliseconds(), IdleMS: idle.Milliseconds(),
			},
		},
		{
			// A teammate message 2 hours after the model finished: waiting
			// for input is user time however long it is.
			name: "a 2-hour wait for input inside the turn stays user time",
			lines: []line{
				{0, "prompt", ""},
				{time.Second, "reply", "a"},
				{2 * time.Second, "stop", ""},
				{2*time.Second + 2*time.Hour, "teammate", ""},
				{3*time.Second + 2*time.Hour, "reply", "b"},
				{4*time.Second + 2*time.Hour, "stop", ""},
			},
			ended: 4*time.Second + 2*time.Hour,
			wantGaps: []string{
				"model 1s", "harness 1s", "user 2h0m0s", "model 1s", "harness 1s",
			},
			want: Totals{
				ModelMS: 2000, HarnessMS: 2000, UserMS: (2 * time.Hour).Milliseconds(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := store.TurnDetail{Turn: store.Turn{
				ID: "turn-idle", Status: store.TurnClosed, StartedAt: t0, EndedAt: after(tt.ended),
			}}
			got := Build(d, jsonl(tt.lines, false), Prices{})
			if g := gapsOf(got.Gaps); !slices.Equal(g, tt.wantGaps) {
				t.Errorf("gaps = %v\nwant   %v", g, tt.wantGaps)
			}
			tot := got.Totals
			want := tt.want
			want.WallMS = tt.ended.Milliseconds()
			if tot.WallMS != want.WallMS || tot.ModelMS != want.ModelMS ||
				tot.ToolMS != want.ToolMS || tot.HarnessMS != want.HarnessMS ||
				tot.UserMS != want.UserMS || tot.IdleMS != want.IdleMS {
				t.Errorf(
					"time = wall %d, model %d, tool %d, harness %d, user %d, idle %d\n"+
						"want   wall %d, model %d, tool %d, harness %d, user %d, idle %d",
					tot.WallMS,
					tot.ModelMS,
					tot.ToolMS,
					tot.HarnessMS,
					tot.UserMS,
					tot.IdleMS,
					want.WallMS,
					want.ModelMS,
					want.ToolMS,
					want.HarnessMS,
					want.UserMS,
					want.IdleMS,
				)
			}
			checkGaps(t, got)
			if s := Stored(tot); s.IdleMS != tot.IdleMS || s.Version != TotalsVersion {
				t.Errorf("Stored = idle %d at version %d, want idle %d at %d",
					s.IdleMS, s.Version, tot.IdleMS, TotalsVersion)
			}
		})
	}
}

// TestBuildAgentIdle: an agent trace takes the idle rule from the same
// partition: a subagent whose model answer came 5 hours after its tool
// result has 30 minutes of model time and the rest idle.
func TestBuildAgentIdle(t *testing.T) {
	lines := jsonl([]line{
		{0, "prompt", ""},
		{time.Second, "call", "a"},
		{2 * time.Second, "result", "a"},
		{2*time.Second + 5*time.Hour, "reply", "b"},
	}, true)
	a := store.Agent{
		ID: "agent-i", SessionID: "s", StartedAt: t0, EndedAt: t0.Add(2*time.Second + 5*time.Hour),
	}
	got := BuildAgent(a, lines, nil, Prices{})
	want := []string{"model 1s", "tool 1s", "model 30m0s", "idle 4h30m0s"}
	if g := gapsOf(got.Gaps); !slices.Equal(g, want) {
		t.Errorf("gaps = %v\nwant   %v", g, want)
	}
	tot := got.Totals
	if tot.IdleMS != (4*time.Hour+30*time.Minute).Milliseconds() ||
		tot.ModelMS != (time.Second+moondb.IdleGapAfter).Milliseconds() {
		t.Errorf("model %dms, idle %dms; want 30m01s of model time and 4h30m idle",
			tot.ModelMS, tot.IdleMS)
	}
	if sum := tot.ModelMS + tot.ToolMS + tot.HarnessMS + tot.UserMS + tot.IdleMS; sum != tot.WallMS {
		t.Errorf("the time split sums to %dms; wall is %dms", sum, tot.WallMS)
	}
}

// TestBuildSessionIdle: a session's idle time is its turns' idle time; the
// gaps between turns stay user time.
func TestBuildSessionIdle(t *testing.T) {
	turn := func(seq int, start, end time.Duration, idleMS int64) TurnTrace {
		wall := (end - start).Milliseconds()
		return TurnTrace{
			TurnID: fmt.Sprint("turn-", seq), Seq: seq,
			Span:   Span{Kind: KindTurn, Start: t0.Add(start), End: t0.Add(end)},
			Totals: Totals{WallMS: wall, ModelMS: wall - idleMS, IdleMS: idleMS},
		}
	}
	got := BuildSession("sess", []TurnTrace{
		turn(1, 0, 6*time.Hour, 5*3600*1000),
		turn(2, 10*time.Hour, 11*time.Hour, 30*60*1000),
	}, nil)
	tot := got.Totals
	if tot.IdleMS != 5*3600*1000+30*60*1000 || tot.UserMS != 4*3600*1000 {
		t.Errorf("idle %dms, user %dms; want the turns' 5h30m idle and the 4h between them as user",
			tot.IdleMS, tot.UserMS)
	}
	if sum := tot.ModelMS + tot.ToolMS + tot.HarnessMS + tot.UserMS + tot.IdleMS; sum != tot.WallMS {
		t.Errorf("the time split sums to %dms; wall is %dms", sum, tot.WallMS)
	}
}
