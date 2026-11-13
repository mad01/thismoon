package cslask

import "testing"

// TestMaxLines pins the max_lines a call needs: none up to the default,
// the count rounded up to a hundred past it and never over the ceiling, and
// no call at all past the ceiling.
func TestMaxLines(t *testing.T) {
	tests := []struct {
		lines, want int
		ok          bool
	}{
		{0, 0, true},
		{843, 0, true},
		{DefaultMaxLines, 0, true},
		{DefaultMaxLines + 1, 6100, true},
		{7421, 7500, true},
		{7500, 7500, true},
		{19950, MaxLinesCeiling, true},
		{MaxLinesCeiling, MaxLinesCeiling, true},
		{MaxLinesCeiling + 1, 0, false},
		{25000, 0, false},
	}
	for _, tt := range tests {
		got, ok := MaxLines(tt.lines)
		if got != tt.want || ok != tt.ok {
			t.Errorf("MaxLines(%d) = %d, %v; want %d, %v", tt.lines, got, ok, tt.want, tt.ok)
		}
	}
}

// TestLimits pins the numbers csl_ask documents: a default of 6000 lines
// and a ceiling of 20000.
func TestLimits(t *testing.T) {
	if DefaultMaxLines != 6000 || MaxLinesCeiling != 20000 {
		t.Errorf(
			"DefaultMaxLines, MaxLinesCeiling = %d, %d; want 6000, 20000",
			DefaultMaxLines, MaxLinesCeiling,
		)
	}
}
