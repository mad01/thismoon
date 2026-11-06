package noteid

import (
	"testing"
	"time"
)

func TestNewIsValidAndSortsByTime(t *testing.T) {
	early := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	a, b := New(early), New(early.Add(time.Millisecond))
	for _, id := range []string{a, b} {
		if !Valid(id) {
			t.Errorf("New made an invalid id %q", id)
		}
	}
	if a >= b {
		t.Errorf("ids do not sort by time: %s >= %s", a, b)
	}
	if c, d := New(early), New(early); c == d {
		t.Error("two ids from the same millisecond are equal")
	}
}

func TestEncodeKnownValue(t *testing.T) {
	var max [16]byte
	for i := range max {
		max[i] = 0xff
	}
	if got := encode(max); got != "7ZZZZZZZZZZZZZZZZZZZZZZZZZ" {
		t.Errorf("encode(max) = %s", got)
	}
	if got := encode([16]byte{}); got != "00000000000000000000000000" {
		t.Errorf("encode(zero) = %s", got)
	}
}

func TestValid(t *testing.T) {
	tests := map[string]bool{
		"01JBZ6Y3W6S1Y1Q7A9M2N4P5RT":  true,
		"81JBZ6Y3W6S1Y1Q7A9M2N4P5RT":  false, // overflows 128 bits
		"01jbz6y3w6s1y1q7a9m2n4p5rt":  false,
		"01JBZ6Y3W6S1Y1Q7A9M2N4P5R":   false,
		"01JBZ6Y3W6S1Y1Q7A9M2N4P5RIX": false,
		"":                            false,
	}
	for id, want := range tests {
		if got := Valid(id); got != want {
			t.Errorf("Valid(%q) = %v, want %v", id, got, want)
		}
	}
}
