package store

import "testing"

func TestValidKind(t *testing.T) {
	cases := map[string]bool{
		KindCodeBehavior: true,
		KindDeadEnd:      true,
		KindPreference:   true,
		KindDecision:     true,
		KindMachineState: true,
		KindOpenThread:   true,
		"":               false,
		"guess":          false,
	}
	for k, want := range cases {
		if got := ValidKind(k); got != want {
			t.Errorf("ValidKind(%q) = %v, want %v", k, got, want)
		}
	}
}

func TestValidConfidence(t *testing.T) {
	cases := map[string]bool{
		ConfidenceVerified: true,
		ConfidenceDerived:  true,
		ConfidenceHint:     true,
		"":                 false,
		"sure":             false,
	}
	for c, want := range cases {
		if got := ValidConfidence(c); got != want {
			t.Errorf("ValidConfidence(%q) = %v, want %v", c, got, want)
		}
	}
}

func TestValidStatus(t *testing.T) {
	cases := map[string]bool{
		StatusFresh:     true,
		StatusStale:     true,
		StatusRetracted: true,
		"":              false,
		"pending":       false,
	}
	for s, want := range cases {
		if got := ValidStatus(s); got != want {
			t.Errorf("ValidStatus(%q) = %v, want %v", s, got, want)
		}
	}
}
