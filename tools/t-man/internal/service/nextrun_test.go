package service

import (
	"testing"
	"time"
	_ "time/tzdata" // the DST cases need real zone rules wherever the tests run
)

// stockholm is a fixed-offset zone so the tests pin that results come back
// in now's location rather than UTC.
var stockholm = time.FixedZone("CEST", 2*60*60)

func on(year int, month time.Month, day, hour, minute int) time.Time {
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
			now:     on(2026, time.October, 1, 6, 0),
			want:    on(2026, time.October, 1, 7, 30),
			wantOK:  true,
		},
		{
			name:    "already passed today wraps to tomorrow",
			entries: []CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
			now:     on(2026, time.October, 1, 9, 0),
			want:    on(2026, time.October, 2, 7, 30),
			wantOK:  true,
		},
		{
			name:    "exactly on the slot is strictly after",
			entries: []CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
			now:     on(2026, time.October, 1, 7, 30),
			want:    on(2026, time.October, 2, 7, 30),
			wantOK:  true,
		},
		{
			name:    "seconds into the slot minute still count as passed",
			entries: []CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
			now:     on(2026, time.October, 1, 7, 30).Add(20 * time.Second),
			want:    on(2026, time.October, 2, 7, 30),
			wantOK:  true,
		},
		{
			name:    "minute wildcard hour only fires at the top of the hour",
			entries: []CalendarEntry{{Hour: ip(7)}},
			now:     on(2026, time.October, 1, 7, 0),
			want:    on(2026, time.October, 1, 7, 1),
			wantOK:  true,
		},
		{
			name:    "minute only fires every hour",
			entries: []CalendarEntry{{Minute: ip(30)}},
			now:     on(2026, time.October, 1, 7, 45),
			want:    on(2026, time.October, 1, 8, 30),
			wantOK:  true,
		},
		{
			name:    "minute wraps across midnight",
			entries: []CalendarEntry{{Minute: ip(15)}},
			now:     on(2026, time.October, 1, 23, 50),
			want:    on(2026, time.October, 2, 0, 15),
			wantOK:  true,
		},
		{
			name:    "weekday later this week",
			entries: []CalendarEntry{{Weekday: ip(5), Hour: ip(17), Minute: ip(0)}},
			now:     on(2026, time.October, 1, 12, 0), // a Thursday
			want:    on(2026, time.October, 2, 17, 0),
			wantOK:  true,
		},
		{
			name:    "weekday wraps to next week",
			entries: []CalendarEntry{{Weekday: ip(1), Hour: ip(7), Minute: ip(0)}},
			now:     on(2026, time.October, 1, 12, 0), // Thursday
			want:    on(2026, time.October, 5, 7, 0),  // next Monday
			wantOK:  true,
		},
		{
			name:    "weekday seven is sunday",
			entries: []CalendarEntry{{Weekday: ip(7), Hour: ip(9), Minute: ip(0)}},
			now:     on(2026, time.October, 1, 12, 0),
			want:    on(2026, time.October, 4, 9, 0),
			wantOK:  true,
		},
		{
			name:    "day of month wraps into next month",
			entries: []CalendarEntry{{Day: ip(1), Hour: ip(0), Minute: ip(0)}},
			now:     on(2026, time.October, 1, 0, 0),
			want:    on(2026, time.November, 1, 0, 0),
			wantOK:  true,
		},
		{
			name:    "day 31 skips short months",
			entries: []CalendarEntry{{Day: ip(31), Hour: ip(12), Minute: ip(0)}},
			now:     on(2026, time.November, 1, 0, 0),
			want:    on(2026, time.December, 31, 12, 0),
			wantOK:  true,
		},
		{
			name:    "month wraps into next year",
			entries: []CalendarEntry{{Month: ip(1), Day: ip(1), Hour: ip(0), Minute: ip(0)}},
			now:     on(2026, time.October, 1, 0, 0),
			want:    on(2027, time.January, 1, 0, 0),
			wantOK:  true,
		},
		{
			name:    "february 29 waits for a leap year",
			entries: []CalendarEntry{{Month: ip(2), Day: ip(29), Hour: ip(8), Minute: ip(0)}},
			now:     on(2026, time.October, 1, 0, 0),
			want:    on(2028, time.February, 29, 8, 0),
			wantOK:  true,
		},
		{
			name:    "weekdays set evaluated on a saturday",
			entries: timed(7, 30, 1, 2, 3, 4, 5),
			now:     on(2026, time.October, 3, 10, 0), // Saturday
			want:    on(2026, time.October, 5, 7, 30), // Monday
			wantOK:  true,
		},
		{
			name:    "weekend set evaluated on a friday evening",
			entries: timed(9, 0, 0, 6),
			now:     on(2026, time.October, 2, 20, 0), // Friday
			want:    on(2026, time.October, 3, 9, 0),  // Saturday
			wantOK:  true,
		},
		{
			name: "earliest entry wins",
			entries: []CalendarEntry{
				{Hour: ip(17), Minute: ip(0)},
				{Hour: ip(7), Minute: ip(30)},
			},
			now:    on(2026, time.October, 1, 6, 0),
			want:   on(2026, time.October, 1, 7, 30),
			wantOK: true,
		},
		{
			name:    "never matches",
			entries: []CalendarEntry{{Month: ip(2), Day: ip(31)}},
			now:     on(2026, time.October, 1, 0, 0),
			wantOK:  false,
		},
		{
			name:   "no entries",
			now:    on(2026, time.October, 1, 0, 0),
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
	now := on(2026, time.October, 1, 6, 0)

	calendar := Definition{Calendar: []CalendarEntry{{Hour: ip(7), Minute: ip(30)}}}
	got, ok := calendar.NextRun(now)
	if !ok || !got.Equal(on(2026, time.October, 1, 7, 30)) {
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

// TestNextCalendarRun_DST pins the clock arithmetic across daylight-saving
// changes, where rebuilding a wall time with time.Date can pick the wrong
// offset. Instants are given in UTC so the ambiguous local hour is
// unambiguous in the fixture.
func TestNextCalendarRun_DST(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	stockholm, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	utc := func(month time.Month, day, hour, minute int) time.Time {
		return time.Date(2026, month, day, hour, minute, 0, 0, time.UTC)
	}
	utc2027 := func(month time.Month, day, hour, minute int) time.Time {
		return time.Date(2027, month, day, hour, minute, 0, 0, time.UTC)
	}

	tests := []struct {
		name  string
		entry CalendarEntry
		loc   *time.Location
		now   time.Time // a UTC instant, viewed in loc
		want  time.Time // a UTC instant
	}{
		{
			// 2026-11-01: CDT ends at 06:00 UTC (02:00 CDT becomes 01:00 CST).
			// 07:10 UTC is 01:10 CST, the second pass through 01:xx; the next
			// 01:30 on the wall is 01:30 CST, not the 01:30 CDT already gone.
			name:  "chicago fall back, second pass stays after now",
			entry: CalendarEntry{Minute: ip(30)},
			loc:   chicago,
			now:   utc(time.November, 1, 7, 10),
			want:  utc(time.November, 1, 7, 30),
		},
		{
			name:  "chicago fall back, first pass is the ordinary case",
			entry: CalendarEntry{Minute: ip(30)},
			loc:   chicago,
			now:   utc(time.November, 1, 6, 10), // 01:10 CDT
			want:  utc(time.November, 1, 6, 30), // 01:30 CDT
		},
		{
			// 2026-10-25: CEST ends at 01:00 UTC (03:00 CEST becomes 02:00 CET).
			// 00:10 UTC is 02:10 CEST, the first pass; 02:55 comes 45 minutes
			// later as CEST, not an hour and 45 minutes later as CET.
			name:  "stockholm fall back, first pass is not an hour late",
			entry: CalendarEntry{Minute: ip(55)},
			loc:   stockholm,
			now:   utc(time.October, 25, 0, 10),
			want:  utc(time.October, 25, 0, 55),
		},
		{
			// 03:00 CEST never shows on the wall: the clock goes from 02:59
			// CEST to 02:00 CET. The next 03:00 is 03:00 CET at 02:00 UTC.
			name:  "stockholm fall back, hour jump lands on the real 03:00",
			entry: CalendarEntry{Hour: ip(3), Minute: ip(0)},
			loc:   stockholm,
			now:   utc(time.October, 24, 22, 10), // 00:10 CEST
			want:  utc(time.October, 25, 2, 0),
		},
		{
			// 2026-03-29: CET ends at 01:00 UTC (02:00 CET becomes 03:00 CEST).
			// 02:30 does not exist; the next :30 on the wall is 03:30 CEST.
			name:  "stockholm spring forward skips the missing half hour",
			entry: CalendarEntry{Minute: ip(30)},
			loc:   stockholm,
			now:   utc(time.March, 29, 0, 50), // 01:50 CET
			want:  utc(time.March, 29, 1, 30), // 03:30 CEST
		},
		{
			// A daily 02:30 slot on the spring-forward day does not exist; the
			// next one is the following day's.
			name:  "stockholm spring forward, missing slot waits a day",
			entry: CalendarEntry{Hour: ip(2), Minute: ip(30)},
			loc:   stockholm,
			now:   utc(time.March, 29, 0, 10), // 01:10 CET
			want:  utc(time.March, 30, 0, 30), // 02:30 CEST on the 30th
		},
		{
			// An hour after the spring-forward gap must not overshoot into
			// the next day: 04:00 CEST is 02:00 UTC, three wall hours but
			// only two real hours after 01:10 CET.
			name:  "stockholm spring forward, hour after the gap is today",
			entry: CalendarEntry{Hour: ip(4), Minute: ip(0)},
			loc:   stockholm,
			now:   utc(time.March, 29, 0, 10), // 01:10 CET
			want:  utc(time.March, 29, 2, 0),  // 04:00 CEST
		},
		{
			// 2027-03-28: CET ends at 01:00 UTC. A daily 07:30 viewed the
			// evening before must land on the 28th at 07:30 CEST (05:30 UTC),
			// not skip to the 29th.
			name:  "stockholm spring forward, daily slot the morning after",
			entry: CalendarEntry{Hour: ip(7), Minute: ip(30)},
			loc:   stockholm,
			now:   utc2027(time.March, 27, 19, 0), // 20:00 CET on the 27th
			want:  utc2027(time.March, 28, 5, 30),
		},
		{
			// 2027-03-14: EST ends at 07:00 UTC (02:00 EST becomes 03:00 EDT).
			// A daily 09:00 viewed Saturday evening lands on Sunday 09:00 EDT
			// (13:00 UTC), not Monday.
			name:  "new york spring forward, daily slot the morning after",
			entry: CalendarEntry{Hour: ip(9), Minute: ip(0)},
			loc:   newYork,
			now:   utc2027(time.March, 13, 23, 0), // 18:00 EST on the 13th
			want:  utc2027(time.March, 14, 13, 0),
		},
		{
			// Fall back: 02:00 shows twice. From 00:10 CEST the next 02:00 on
			// the wall is the first copy, 02:00 CEST (00:00 UTC); time.Date
			// alone would pick 02:00 CET an hour later.
			name:  "stockholm fall back, hour jump takes the first copy",
			entry: CalendarEntry{Hour: ip(2), Minute: ip(0)},
			loc:   stockholm,
			now:   utc(time.October, 24, 22, 10), // 00:10 CEST
			want:  utc(time.October, 25, 0, 0),
		},
		{
			// Chicago fall back: 02:00 CDT never shows (01:59 CDT becomes
			// 01:00 CST). From 01:10 CDT the next 02:00 is 02:00 CST, 08:00
			// UTC; elapsed time alone would land on 01:00 CST.
			name:  "chicago fall back, hour jump skips the hidden hour",
			entry: CalendarEntry{Hour: ip(2), Minute: ip(0)},
			loc:   chicago,
			now:   utc(time.November, 1, 6, 10), // 01:10 CDT
			want:  utc(time.November, 1, 8, 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := tt.now.In(tt.loc)
			got, ok := NextCalendarRun([]CalendarEntry{tt.entry}, now)
			if !ok {
				t.Fatalf("NextCalendarRun() found nothing")
			}
			if !got.After(now) {
				t.Errorf("NextCalendarRun() = %v is not after now %v", got, now)
			}
			if !got.Equal(tt.want) {
				t.Errorf("NextCalendarRun() = %v (%v UTC), want %v UTC",
					got, got.UTC().Format("15:04"), tt.want.Format("15:04"))
			}
		})
	}
}
