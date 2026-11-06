package trace

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// piStart is the start of the pi fixture turns.
var piStart = time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)

func piTurn(start time.Time, d time.Duration) store.TurnDetail {
	end := start.Add(d)
	return store.TurnDetail{Turn: store.Turn{
		ID: "turn-pi", Status: store.TurnClosed, StartedAt: start, EndedAt: &end,
	}}
}

func TestPiCorpusParses(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "pi", "*.jsonl"))
	if err != nil || len(files) == 0 {
		t.Fatalf("pi corpus: %v, %d files", err, len(files))
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if !isPi(data) {
				t.Fatal("isPi = false for a pi session file")
			}
			_, counts := parseLines(data, false)
			if counts.Skipped != 0 || counts.Read == 0 {
				t.Errorf("lines = %+v; want every line read and none skipped", counts)
			}
		})
	}
}

func TestIsPiLeavesClaudeTranscripts(t *testing.T) {
	for _, name := range []string{"plain.jsonl", "tools.jsonl", "agent.jsonl"} {
		if isPi(fixture(t, name)) {
			t.Errorf("isPi(%s) = true for a Claude Code transcript", name)
		}
	}
}

func TestBuildPiTurn(t *testing.T) {
	tt := Build(piTurn(piStart, 9*time.Second), fixture(t, "pi/tooluse.jsonl"), Prices{})
	got := tt.Totals
	wantTokens := Tokens{
		Input: 1600, CacheWrite5m: 2200, CacheWrite1h: 1000, CacheRead: 8700, Output: 290, Thinking: 20,
	}
	if got.Tokens != wantTokens {
		t.Errorf("tokens = %+v; want %+v", got.Tokens, wantTokens)
	}
	// Model time runs from the line before each answer to the answer; the
	// tool results end tool time; pi writes no Stop line, so the last
	// second is harness time.
	want := Totals{
		WallMS: 9000, ModelMS: 6300, ToolMS: 1700, HarnessMS: 1000,
		ModelCalls: 3, ToolCalls: 3, ToolErrors: 1,
	}
	if got.WallMS != want.WallMS || got.ModelMS != want.ModelMS || got.ToolMS != want.ToolMS ||
		got.HarnessMS != want.HarnessMS || got.UserMS != 0 {
		t.Errorf("time split = %+v; want %+v", got, want)
	}
	if got.ModelCalls != want.ModelCalls || got.ToolCalls != want.ToolCalls ||
		got.ToolErrors != want.ToolErrors {
		t.Errorf("calls = %d model, %d tool, %d errors; want %d, %d, %d",
			got.ModelCalls, got.ToolCalls, got.ToolErrors,
			want.ModelCalls, want.ToolCalls, want.ToolErrors)
	}
	r := moondb.DefaultPrices["claude-haiku-5-5"].Rates
	wantCost := (1600*r.Input + 2200*r.CacheWrite5m + 1000*r.CacheWrite1h +
		8700*r.CacheRead + 290*r.Output) / tokensPerMillion
	if got.Cost == nil || math.Abs(*got.Cost-wantCost) > 1e-12 {
		t.Errorf("cost = %v; want %v at the claude-haiku-5-5 row", got.Cost, wantCost)
	}
	if len(got.Unpriced) != 0 {
		t.Errorf("unpriced = %v; want none", got.Unpriced)
	}
	var models, failed []string
	for _, s := range tt.Spans {
		switch {
		case s.Kind == KindModel:
			models = append(models, s.ID)
		case s.Kind == KindTool && s.IsError:
			failed = append(failed, s.ID)
		}
	}
	if !slices.Equal(models, []string{"a0000001", "a0000002", "a0000003"}) {
		t.Errorf("model spans = %v; want one per assistant entry", models)
	}
	if !slices.Equal(failed, []string{"call_3"}) {
		t.Errorf("failed tool spans = %v; want call_3", failed)
	}
}

func TestBuildPiSummaryEntries(t *testing.T) {
	start := piStart.Add(time.Hour)
	tt := Build(piTurn(start, 10*time.Second), fixture(t, "pi/summary.jsonl"), Prices{})
	got := tt.Totals
	// The answer, the cache warm-up and the compaction each billed a call;
	// the compaction names no model and takes the one model_change chose.
	if got.ModelCalls != 3 {
		t.Errorf("model calls = %d; want 3", got.ModelCalls)
	}
	want := Tokens{Input: 1200, CacheRead: 2000, Output: 130}
	if got.Tokens != want {
		t.Errorf("tokens = %+v; want %+v", got.Tokens, want)
	}
	if len(got.Usage) != 1 || got.Usage[0].Model != "anthropic/claude-haiku-5.5" {
		t.Errorf("usage = %+v; want one entry for anthropic/claude-haiku-5.5", got.Usage)
	}
	if got.Cost == nil || len(got.Unpriced) != 0 {
		t.Errorf("cost = %v, unpriced %v; want every call priced", got.Cost, got.Unpriced)
	}
}

func TestBuildPiCutHead(t *testing.T) {
	// The size cap keeps a chunk's tail: a pi chunk that starts at a tool
	// result has lost the lines before it.
	lines := bytes.SplitAfter(fixture(t, "pi/tooluse.jsonl"), []byte("\n"))
	tt := Build(piTurn(piStart, 9*time.Second), bytes.Join(lines[2:], nil), Prices{})
	if !tt.Lines.CutHead {
		t.Errorf("lines = %+v; want CutHead for a chunk opening on a tool result", tt.Lines)
	}
}

func TestBuildPiTurnWithoutPromptLine(t *testing.T) {
	// pi writes the prompt before the extension opens the turn, so a live
	// turn's lines start at the first answer: the model worked from the
	// turn's start, and nothing was cut.
	lines := bytes.SplitAfter(fixture(t, "pi/tooluse.jsonl"), []byte("\n"))
	tt := Build(piTurn(piStart, 9*time.Second), bytes.Join(lines[1:], nil), Prices{})
	if tt.Lines.CutHead {
		t.Errorf("lines = %+v; want no CutHead for a chunk opening on the first answer", tt.Lines)
	}
	if got := tt.Totals; got.ModelMS != 6300 || got.HarnessMS != 1000 {
		t.Errorf("model %dms harness %dms; want 6300 and 1000, as with the prompt line",
			got.ModelMS, got.HarnessMS)
	}
	if first := tt.Spans[0]; first.Kind != KindModel || first.DurationMS != 2000 {
		t.Errorf("first span = %s %dms; want the first answer's 2000ms from the turn's start",
			first.Kind, first.DurationMS)
	}
}
