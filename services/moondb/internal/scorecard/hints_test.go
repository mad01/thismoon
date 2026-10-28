package scorecard

import "testing"

func TestHintStats(t *testing.T) {
	if hintStats(nil) != nil {
		t.Error("no events answer: want null hints")
	}
	h := hintStats([]HintEvent{
		{Hint: "prefer-csl"}, {Hint: "em-dash"}, {Hint: "prefer-csl"}, {Hint: "agent-memory"},
	})
	want := []HintCount{{"prefer-csl", 2}, {"agent-memory", 1}, {"em-dash", 1}}
	if h.Shown != 4 || len(h.ByHint) != 3 {
		t.Fatalf("hints = %+v", h)
	}
	for i := range want {
		if h.ByHint[i] != want[i] {
			t.Errorf("by hint = %+v, want %+v", h.ByHint, want)
		}
	}
}
