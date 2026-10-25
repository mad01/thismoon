package ingest

import (
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

func skippedEnv(session, turn, toolUseID string, minute, clean, timeout int) spool.Envelope {
	env := base(spool.KindSkipped, toolUseID+":skipped", session, turn, minute)
	env.Skipped = &store.SkippedFiles{
		Tool: "Bash", ToolUseID: toolUseID, Clean: clean, Timeout: timeout,
	}
	return env
}

func TestSkippedCountsLandOnTheTurn(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	// The first count arrives before its turn opens: the placeholder turn
	// keeps it, and the open and the close keep it too.
	f.write(skippedEnv("s1", "p1", "toolu_pull", 2, 40, 6), nil)
	f.write(openEnv(c, "s1", "p1", 1, "pull main"), c)
	f.write(skippedEnv("s1", "p1", "toolu_reset", 3, 2, 0), nil)
	f.write(closeEnv(c, "s1", "p1", 5, "done"), c)
	f.drain()
	// A replayed envelope adds nothing.
	f.write(skippedEnv("s1", "p1", "toolu_pull", 2, 40, 6), nil)
	f.drain()

	if n := f.pending(); n != 0 {
		t.Errorf("%d envelopes left in the spool, want 0", n)
	}
	tr := f.turn("p1").Turn
	want := []store.SkippedFiles{
		{Tool: "Bash", ToolUseID: "toolu_pull", At: at(2), Clean: 40, Timeout: 6},
		{Tool: "Bash", ToolUseID: "toolu_reset", At: at(3), Clean: 2},
	}
	if len(tr.Skipped) != len(want) {
		t.Fatalf("turn skipped = %+v, want %+v", tr.Skipped, want)
	}
	for i, w := range want {
		got := tr.Skipped[i]
		if !got.At.Equal(w.At) {
			t.Errorf("skipped %d at %v, want %v", i, got.At, w.At)
		}
		got.At = w.At
		if got != w {
			t.Errorf("skipped %d = %+v, want %+v", i, got, w)
		}
	}
	if tr.Status != store.TurnClosed || tr.PromptChunk == "" || !tr.StartedAt.Equal(at(1)) {
		t.Errorf("turn = %+v, want it opened at minute 1 with its prompt, then closed", tr)
	}
}

func TestSkippedWithoutTurnIsDropped(t *testing.T) {
	f := newFixture(t)
	f.write(skippedEnv("s1", "", "toolu_x", 2, 1, 0), nil)
	f.drain()
	if n := f.pending(); n != 0 || !f.logged("has no turn id") {
		t.Errorf("%d envelopes left, logs %q; want none left and the drop logged", n, f.logs)
	}
}
