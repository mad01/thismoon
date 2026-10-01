package service

import (
	"testing"
	"time"
)

// stockholm is a fixed-offset zone so the tests pin that results come back
// in now's location rather than UTC.
var stockholm = time.FixedZone("CEST", 2*60*60)

func at(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, stockholm)
}

func TestNextCalendarRun(t *testing.T) {
	tests := []struct {
		name    string
		entries []CalendarEntry
		now     time.Time
		want    time.Time
		wantOK  bool
	}{
		{
			name:    "later today",
			entries: []CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
			now:     at(2026, time.October, 1, 6, 0),
			want:    at(2026, time.October, 1, 7, 30),
			wantOK:  true,
		},
		{
			name:    "already passed today wraps to tomorrow",
			entries: []CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
			now:     at(2026, time.October, 1, 9, 0),
			want:    at(2026, time.October, 2, 7, 30),
			wantOK:  true,
		},
		{
			name:    "exactly on the slot is strictly after",
			entries: []CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
			now:     at(2026, time.October, 1, 7, 30),
			want:    at(2026, time.October, 2, 7, 30),
			wantOK:  true,
		},
		{
			name:    "seconds into the slot minute still count as passed",
			entries: []CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
			now:     at(2026, time.October, 1, 7, 30).Add(20 * time.Second),
			want:    at(2026, time.October, 2, 7, 30),
			wantOK:  true,
		},
		{
			name:    "minute wildcard hour only fires at the top of the hour",
			entries: []CalendarEntry{{Hour: ip(7)}},
			now:     at(2026, time.October, 1, 7, 0),
			want:    at(2026, time.October, 1, 7, 1),
			wantOK:  true,
		},
		{
			name:    "minute only fires every hour",
			entries: []CalendarEntry{{Minute: ip(30)}},
			now:     at(2026, time.October, 1, 7, 45),
			want:    at(2026, time.October, 1, 8, 30),
			wantOK:  true,
		},
		{
			name:    "minute wraps across midnight",
			entries: []CalendarEntry{{Minute: ip(15)}},
			now:     at(2026, time.October, 1, 23, 50),
			want:    at(2026, time.October, 2, 0, 15),
			wantOK:  true,
		},
		{
			name:    "weekday later this week",
			entries: []CalendarEntry{{Weekday: ip(5), Hour: ip(17), Minute: ip(0)}},
			now:     at(2026, time.October, 1, 12, 0), // a Thursday
			want:    at(2026, time.October, 2, 17, 0),
			wantOK:  true,
		},
		{
			name:    "weekday wraps to next week",
			entries: []CalendarEntry{{Weekday: ip(1), Hour: ip(7), Minute: ip(0)}},
			now:     at(2026, time.October, 1, 12, 0), // Thursday
			want:    at(2026, time.October, 5, 7, 0),  // next Monday
			wantOK:  true,
		},
		{
			name:    "weekday seven is sunday",
			entries: []CalendarEntry{{Weekday: ip(7), Hour: ip(9), Minute: ip(0)}},
			now:     at(2026, time.October, 1, 12, 0),
			want:    at(2026, time.October, 4, 9, 0),
			wantOK:  true,
		},
		{
			name:    "day of month wraps into next month",
			entries: []CalendarEntry{{Day: ip(1), Hour: ip(0), Minute: ip(0)}},
			now:     at(2026, time.October, 1, 0, 0),
			want:    at(2026, time.November, 1, 0, 0),
			wantOK:  true,
		},
		{
			name:    "day 31 skips short months",
			entries: []CalendarEntry{{Day: ip(31), Hour: ip(12), Minute: ip(0)}},
			now:     at(2026, time.November, 1, 0, 0),
			want:    at(2026, time.December, 31, 12, 0),
			wantOK:  true,
		},
		{
			name:    "month wraps into next year",
			entries: []CalendarEntry{{Month: ip(1), Day: ip(1), Hour: ip(0), Minute: ip(0)}},
			now:     at(2026, time.October, 1, 0, 0),
			want:    at(2027, time.January, 1, 0, 0),
			wantOK:  true,
		},
		{
			name:    "february 29 waits for a leap year",
			entries: []CalendarEntry{{Month: ip(2), Day: ip(29), Hour: ip(8), Minute: ip(0)}},
			now:     at(2026, time.October, 1, 0, 0),
			want:    at(2028, time.February, 29, 8, 0),
			wantOK:  true,
		},
		{
			name:    "day and weekday must both match",
			entries: []CalendarEntry{{Day: ip(13), Weekday: ip(5), Hour: ip(13), Minute: ip(13)}},
			now:     at(2026, time.October, 1, 0, 0),
			want:    at(2026, time.November, 13, 13, 13), // first Friday the 13th after now
			wantOK:  true,
		},
		{
			name: "earliest entry wins",
			entries: []CalendarEntry{
				{Hour: ip(17), Minute: ip(0)},
				{Hour: ip(7), Minute: ip(30)},
			},
			now:    at(2026, time.October, 1, 6, 0),
			want:   at(2026, time.October, 1, 7, 30),
			wantOK: true,
		},
		{
			name:    "never matches",
			entries: []CalendarEntry{{Month: ip(2), Day: ip(31)}},
			now:     at(2026, time.October, 1, 0, 0),
			wantOK:  false,
		},
		{
			name:   "no entries",
			now:    at(2026, time.October, 1, 0, 0),
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NextCalendarRun(tt.entries, tt.now)
			if ok != tt.wantOK {
				t.Fatalf("NextCalendarRun() ok = %v, want %v (got %v)", ok, tt.wantOK, got)
			}
			if !ok {
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("NextCalendarRun() = %v, want %v", got, tt.want)
			}
			if got.Location() != tt.now.Location() {
				t.Errorf("NextCalendarRun() location = %v, want %v", got.Location(), tt.now.Location())
			}
		})
	}
}

func TestDefinitionNextRun(t *testing.T) {
	now := at(2026, time.October, 1, 6, 0)

	calendar := Definition{Calendar: []CalendarEntry{{Hour: ip(7), Minute: ip(30)}}}
	got, ok := calendar.NextRun(now)
	if !ok || !got.Equal(at(2026, time.October, 1, 7, 30)) {
		t.Errorf("calendar NextRun() = %v, %v", got, ok)
	}

	interval := Definition{IntervalSeconds: 3600}
	if _, ok := interval.NextRun(now); ok {
		t.Error("interval NextRun() reported a time; its phase is launchd's, not ours")
	}

	plain := Definition{}
	if _, ok := plain.NextRun(now); ok {
		t.Error("unscheduled NextRun() reported a time")
	}
}
