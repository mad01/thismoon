package store

import (
	"testing"
	"time"
)

func TestParseRepeat(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"daily", 24 * time.Hour, false},
		{"weekly", 7 * 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"0s", 0, true},  // non-positive
		{"-1h", 0, true}, // negative
		{"banana", 0, true},
	}
	for _, c := range cases {
		got, err := ParseRepeat(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseRepeat(%q): want error, got %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRepeat(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseRepeat(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNextDue(t *testing.T) {
	base := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)

	// Due far in the past with a daily repeat reschedules to the next future
	// occurrence in one call, not one-interval-past-due.
	now := base.Add(50 * time.Hour) // 2 days + 2h after base
	next, err := NextDue(base, "daily", now)
	if err != nil {
		t.Fatalf("NextDue: %v", err)
	}
	if !next.After(now) {
		t.Fatalf("NextDue = %v, want strictly after now %v", next, now)
	}
	want := base.Add(72 * time.Hour) // third daily occurrence
	if !next.Equal(want) {
		t.Errorf("NextDue = %v, want %v", next, want)
	}

	// A non-recurring spec is a programmer error here.
	if _, err := NextDue(base, "", now); err == nil {
		t.Error("NextDue with empty repeat: want error")
	}
}

func TestOverdue(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	past := Reminder{Status: StatusPending, Due: now.Add(-time.Hour)}
	future := Reminder{Status: StatusPending, Due: now.Add(time.Hour)}
	firedPast := Reminder{Status: StatusFired, Due: now.Add(-time.Hour)}

	if !Overdue(past, now) {
		t.Error("past pending reminder should be overdue")
	}
	if Overdue(future, now) {
		t.Error("future pending reminder should not be overdue")
	}
	if Overdue(firedPast, now) {
		t.Error("a fired reminder is never overdue")
	}
}
