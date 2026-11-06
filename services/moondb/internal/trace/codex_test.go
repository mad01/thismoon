package trace

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// The first turn of testdata/codex/twoturns.jsonl runs from its
// task_started line to its task_complete line.
var (
	codexStart = time.Date(2026, 11, 6, 11, 35, 25, 447e6, time.UTC)
	codexEnd   = time.Date(2026, 11, 6, 11, 35, 36, 764e6, time.UTC)
)

func codexTurn(start, end time.Time) store.TurnDetail {
	return store.TurnDetail{Turn: store.Turn{
		ID: "turn-codex", Status: store.TurnClosed, StartedAt: start, EndedAt: &end,
	}}
}

// codexLines is file lines from to through of a Codex fixture, counted
// from 1 as an editor shows them.
func codexLines(t *testing.T, name string, from, through int) []byte {
	t.Helper()
	lines := bytes.SplitAfter(fixture(t, filepath.Join("codex", name)), []byte("\n"))
	return bytes.Join(lines[from-1:through], nil)
}

func TestCodexCorpusParses(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "codex", "*.jsonl"))
	if err != nil || len(files) == 0 {
		t.Fatalf("codex corpus: %v, %d files", err, len(files))
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if got := formatOf(data); got != formatCodex {
				t.Fatalf("formatOf = %d; want formatCodex for a Codex rollout", got)
			}
			_, counts := parseLines(data, false)
			if counts.Skipped != 0 || counts.Read == 0 {
				t.Errorf("lines = %+v; want every line read and none skipped", counts)
			}
		})
	}
}

func TestBuildCodexTurn(t *testing.T) {
	tt := Build(codexTurn(codexStart, codexEnd), codexLines(t, "twoturns.jsonl", 2, 45), Prices{})
	got := tt.Totals
	// Input counts the cached and cache-written tokens too; what they
	// leave is the uncached input.
	wantTokens := Tokens{
		Input:        12,
		CacheWrite5m: 1929,
		CacheRead:    79824,
		Output:       1026,
		Thinking:     187,
	}
	if got.Tokens != wantTokens {
		t.Errorf("tokens = %+v; want %+v", got.Tokens, wantTokens)
	}
	// Each response runs from the line before its usage line to it; tool
	// outputs end tool time; the 11ms left are the context lines before
	// the prompt and the wait for task_complete.
	if got.WallMS != 11317 || got.ModelMS != 8637 || got.ToolMS != 2669 ||
		got.HarnessMS != 11 || got.UserMS != 0 {
		t.Errorf("time split = wall %d model %d tool %d harness %d user %d; "+
			"want 11317, 8637, 2669, 11, 0",
			got.WallMS, got.ModelMS, got.ToolMS, got.HarnessMS, got.UserMS)
	}
	// A nonzero exit in the item_completed line, a nonzero exit in the
	// output's header, and a tool Codex does not offer each fail a call.
	if got.ModelCalls != 5 || got.ToolCalls != 5 || got.ToolErrors != 3 {
		t.Errorf("calls = %d model, %d tool, %d errors; want 5, 5, 3",
			got.ModelCalls, got.ToolCalls, got.ToolErrors)
	}
	r := moondb.DefaultPrices["claude-haiku-5-5"].Rates
	wantCost := (12*r.Input + 1929*r.CacheWrite5m + 79824*r.CacheRead + 1026*r.Output) / tokensPerMillion
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
	wantModels := []string{
		"gen-1793964925-UDJcWfSJAke47ZsWNTfn", "gen-1793964927-s2RPJlmfmmzFgcG5FGZ4",
		"gen-1793964928-J2ry5BVLWXql3XrCzTgZ", "gen-1793964930-Wm97vC5N37cJCQH1y1pV",
		"gen-1793964935-MlTuMaH2UIWOcdmCP9F6",
	}
	if !slices.Equal(models, wantModels) {
		t.Errorf("model spans = %v; want one per usage line, by response id", models)
	}
	wantFailed := []string{
		"toolu_01B33Fre87aeUNo7D86U26Bf", "toolu_01TpUQgdE9d6vQgTCfZKi4Pb", "toolu_01HbeAEkXgaQ66NtcZ6PgDBE",
	}
	if !slices.Equal(failed, wantFailed) {
		t.Errorf("failed tool spans = %v; want %v", failed, wantFailed)
	}
}

func TestBuildCodexTurnWithoutPromptLine(t *testing.T) {
	// Codex can append the prompt before the hook opens the turn; then the
	// lines start at the first response, which the model worked on from
	// the turn's start, and nothing was cut.
	tt := Build(codexTurn(codexStart, codexEnd), codexLines(t, "twoturns.jsonl", 9, 45), Prices{})
	if tt.Lines.CutHead {
		t.Errorf("lines = %+v; want no CutHead for a chunk opening on the first response", tt.Lines)
	}
	if got := tt.Totals; got.ModelMS != 8644 || got.HarnessMS != 4 {
		t.Errorf("model %dms harness %dms; want 8644 and 4", got.ModelMS, got.HarnessMS)
	}
}

func TestBuildCodexCutHead(t *testing.T) {
	// A chunk the size cap cut can open on a tool output: the call it
	// answers is gone.
	tt := Build(codexTurn(codexStart, codexEnd), codexLines(t, "twoturns.jsonl", 13, 45), Prices{})
	if !tt.Lines.CutHead {
		t.Errorf("lines = %+v; want CutHead for a chunk opening on a tool output", tt.Lines)
	}
}

func TestBuildCodexResumedTurn(t *testing.T) {
	// The second turn resumes the thread: it names its model again in its
	// own turn_context and bills cache reads for the first turn's prefix.
	start := time.Date(2026, 11, 6, 11, 35, 37, 81e6, time.UTC)
	end := time.Date(2026, 11, 6, 11, 35, 41, 816e6, time.UTC)
	tt := Build(codexTurn(start, end), codexLines(t, "twoturns.jsonl", 48, 63), Prices{})
	got := tt.Totals
	want := Tokens{Input: 6, CacheWrite5m: 469, CacheRead: 34226, Output: 486, Thinking: 161}
	if got.Tokens != want {
		t.Errorf("tokens = %+v; want %+v", got.Tokens, want)
	}
	if got.ModelCalls != 2 || got.ToolCalls != 1 || got.ToolErrors != 0 {
		t.Errorf("calls = %d model, %d tool, %d errors; want 2, 1, 0",
			got.ModelCalls, got.ToolCalls, got.ToolErrors)
	}
	if got.Cost == nil || len(got.Unpriced) != 0 {
		t.Errorf("cost = %v, unpriced %v; want every call priced", got.Cost, got.Unpriced)
	}
}

func TestCodexOutputErrors(t *testing.T) {
	for _, tc := range []struct {
		output string
		want   bool
	}{
		{"Chunk ID: 1\nWall time: 0.1 seconds\nProcess exited with code 0\nOutput:\nok", false},
		{"Chunk ID: 1\nWall time: 0.1 seconds\nProcess exited with code 2\nOutput:\n", true},
		{"unsupported call: apply_patch", true},
		{"Success. Updated the following files:\nM greet.go", false},
		{"Output:\nProcess exited with code 1 is what the test prints", false},
	} {
		raw, _ := json.Marshal(tc.output)
		p := &codexParser{}
		if got := p.isError(codexPayload{CallID: "c", Output: raw}); got != tc.want {
			t.Errorf("isError(%q) = %v; want %v", tc.output, got, tc.want)
		}
	}
}
