package service

import (
	"reflect"
	"strings"
	"testing"
)

// ip returns a pointer to n for building CalendarEntry literals.
func ip(n int) *int { return &n }

// at builds a timed entry, optionally on one weekday (-1 for none).
func at(weekday, hour, minute int) CalendarEntry {
	e := CalendarEntry{Hour: ip(hour), Minute: ip(minute)}
	if weekday >= 0 {
		e.Weekday = ip(weekday)
	}
	return e
}

// timed builds one entry per weekday at the same time.
func timed(hour, minute int, weekdays ...int) []CalendarEntry {
	out := make([]CalendarEntry, 0, len(weekdays))
	for _, d := range weekdays {
		out = append(out, at(d, hour, minute))
	}
	return out
}

func TestParseClockSchedule(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    []CalendarEntry
		wantErr string
	}{
		{name: "daily time", spec: "07:30", want: []CalendarEntry{at(-1, 7, 30)}},
		{name: "daily prefix is the same", spec: "daily@07:30", want: []CalendarEntry{at(-1, 7, 30)}},
		{
			name: "comma list sorts by time",
			spec: "17:00, 07:30",
			want: []CalendarEntry{at(-1, 7, 30), at(-1, 17, 0)},
		},
		{
			name: "weekday form",
			spec: "mon@07:30,Friday@17:00",
			want: []CalendarEntry{at(1, 7, 30), at(5, 17, 0)},
		},
		{name: "sunday is zero", spec: "sun@00:00", want: []CalendarEntry{at(0, 0, 0)}},
		{name: "weekday range", spec: "mon-fri@07:30", want: timed(7, 30, 1, 2, 3, 4, 5)},
		{name: "weekday range wraps", spec: "fri-mon@09:00", want: timed(9, 0, 0, 1, 5, 6)},
		{name: "single-day range", spec: "wed-wed@07:30", want: timed(7, 30, 3)},
		{name: "weekdays set", spec: "Weekdays@07:30", want: timed(7, 30, 1, 2, 3, 4, 5)},
		{name: "weekend set", spec: "weekend@09:00", want: timed(9, 0, 0, 6)},
		{
			name: "duplicates collapse",
			spec: "mon@07:30,weekdays@07:30,mon-fri@07:30",
			want: timed(7, 30, 1, 2, 3, 4, 5),
		},
		{name: "missing minutes", spec: "7", wantErr: "HH:MM"},
		{name: "single digit hour", spec: "7:30", wantErr: "HH:MM"},
		{name: "signed hour", spec: "+7:30", wantErr: "HH:MM"},
		{name: "negative hour", spec: "-0:30", wantErr: "HH:MM"},
		{name: "signed minute", spec: "07:+5", wantErr: "HH:MM"},
		{name: "hour out of range", spec: "24:00", wantErr: "hour must be 0-23"},
		{name: "minute out of range", spec: "07:60", wantErr: "minute must be 0-59"},
		{name: "unknown weekday", spec: "someday@07:30", wantErr: "unknown weekday"},
		{name: "unknown set", spec: "workdays@07:30", wantErr: "unknown weekday"},
		{name: "unknown range end", spec: "mon-someday@07:30", wantErr: "unknown weekday"},
		{name: "empty range end", spec: "mon-@07:30", wantErr: "range is empty"},
		{name: "empty range start", spec: "-fri@07:30", wantErr: "range is empty"},
		{name: "empty item", spec: "07:30,", wantErr: "HH:MM"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseClockSchedule(tt.spec)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseClockSchedule(%q) error = %v, want containing %q",
						tt.spec, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseClockSchedule(%q) error = %v", tt.spec, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseClockSchedule(%q) = %v, want %v", tt.spec, got, tt.want)
			}
		})
	}
}

func TestParseCalendarFields(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    []CalendarEntry
		wantLen int // when the exact entries are too many to list
		wantErr string
	}{
		{
			name: "every field that may share an entry",
			spec: "minute=0,hour=7,day=1,month=1",
			want: []CalendarEntry{{Minute: ip(0), Hour: ip(7), Day: ip(1), Month: ip(1)}},
		},
		{
			name:    "day with weekday is refused",
			spec:    "day=13,weekday=5,hour=13,minute=13",
			wantErr: "both day and weekday",
		},
		{
			name:    "day range with weekday range is refused",
			spec:    "day=1-7,weekday=1-5,hour=6,minute=0",
			wantErr: "both day and weekday",
		},
		{
			name: "mixed case and spaces",
			spec: " Hour = 7 , minute=30",
			want: []CalendarEntry{at(-1, 7, 30)},
		},
		{name: "hourly on the half", spec: "minute=30", want: []CalendarEntry{{Minute: ip(30)}}},
		{name: "weekday range", spec: "weekday=1-5,hour=7,minute=30", want: timed(7, 30, 1, 2, 3, 4, 5)},
		{name: "weekday range wraps", spec: "weekday=5-1,hour=9,minute=0", want: timed(9, 0, 0, 1, 5, 6)},
		{name: "weekday 7 folds onto 0", spec: "weekday=7,hour=9,minute=0", want: timed(9, 0, 0)},
		{
			name: "weekday 0-7 is the whole week",
			spec: "weekday=0-7,hour=9,minute=0",
			want: timed(9, 0, 0, 1, 2, 3, 4, 5, 6),
		},
		{
			name: "hour range",
			spec: "hour=9-11,minute=0",
			want: []CalendarEntry{at(-1, 9, 0), at(-1, 10, 0), at(-1, 11, 0)},
		},
		{name: "day range", spec: "day=1-7,hour=6,minute=0", wantLen: 7},
		{name: "cross product", spec: "hour=9-17,weekday=1-5,minute=0", wantLen: 45},
		{name: "minute range", spec: "minute=0-59,hour=12", wantLen: 60},
		{name: "not key=value", spec: "hour", wantErr: "not key=value"},
		{name: "unknown key", spec: "second=5", wantErr: "unknown calendar field"},
		{name: "not an integer", spec: "hour=seven", wantErr: "must be an integer"},
		{name: "signed value", spec: "hour=+5", wantErr: "must be an integer"},
		{name: "signed range start", spec: "weekday=+1-5", wantErr: "must be an integer"},
		{name: "signed range end", spec: "hour=9-+5", wantErr: "must be an integer"},
		{name: "repeated key", spec: "hour=7,hour=8", wantErr: "given twice"},
		{name: "day out of range", spec: "day=0", wantErr: "day must be 1-31"},
		{name: "month out of range", spec: "month=13", wantErr: "month must be 1-12"},
		{name: "weekday out of range", spec: "weekday=8", wantErr: "weekday must be 0-7"},
		{name: "range end out of range", spec: "hour=20-24", wantErr: "hour must be 0-23"},
		{name: "reversed range is not a wrap", spec: "hour=17-9", wantErr: "must run upward"},
		{name: "reversed day range", spec: "day=7-1", wantErr: "must run upward"},
		{name: "empty range end", spec: "hour=9-", wantErr: "range is empty"},
		{name: "empty range start", spec: "hour=-5", wantErr: "range is empty"},
		{name: "cap exceeded", spec: "minute=0-59,hour=0-23", wantErr: "more than the 200 allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCalendarFields(tt.spec)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseCalendarFields(%q) error = %v, want containing %q",
						tt.spec, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCalendarFields(%q) error = %v", tt.spec, err)
			}
			if tt.wantLen > 0 {
				if len(got) != tt.wantLen {
					t.Errorf("ParseCalendarFields(%q) = %d entries, want %d", tt.spec, len(got), tt.wantLen)
				}
				if !reflect.DeepEqual(got, NormalizeCalendar(got)) {
					t.Errorf("ParseCalendarFields(%q) is not normalized", tt.spec)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseCalendarFields(%q) = %v, want %v", tt.spec, got, tt.want)
			}
		})
	}
}

func TestNormalizeCalendar(t *testing.T) {
	in := []CalendarEntry{
		at(5, 17, 0),
		at(1, 7, 30),
		at(7, 9, 0), // Sunday written as 7
		at(0, 9, 0), // the same day written as 0
		at(-1, 7, 30),
		at(1, 7, 30), // duplicate
		{Day: ip(1), Hour: ip(6), Minute: ip(0)},
	}
	want := []CalendarEntry{
		{Day: ip(1), Hour: ip(6), Minute: ip(0)}, // no weekday sorts first, then by hour
		at(-1, 7, 30),
		at(0, 9, 0),
		at(1, 7, 30),
		at(5, 17, 0),
	}
	got := NormalizeCalendar(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeCalendar() =\n%v\nwant\n%v", got, want)
	}
	if NormalizeCalendar(nil) != nil || NormalizeCalendar([]CalendarEntry{}) != nil {
		t.Error("NormalizeCalendar() of nothing is not nil")
	}
}

func TestParseInterval(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    int
		wantErr bool
	}{
		{name: "one hour", spec: "1h", want: 3600},
		{name: "minutes and seconds", spec: "1m30s", want: 90},
		{name: "one second", spec: "1s", want: 1},
		{name: "zero", spec: "0s", wantErr: true},
		{name: "negative", spec: "-1h", wantErr: true},
		{name: "sub-second", spec: "1500ms", wantErr: true},
		{name: "not a duration", spec: "hourly", wantErr: true},
		{name: "bare number", spec: "60", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInterval(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseInterval(%q) error = %v, wantErr %v", tt.spec, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseInterval(%q) = %d, want %d", tt.spec, got, tt.want)
			}
		})
	}
}

func TestCalendarEntryString(t *testing.T) {
	tests := []struct {
		entry CalendarEntry
		want  string
	}{
		{at(-1, 7, 30), "minute=30,hour=7"},
		{at(1, 7, 0), "minute=0,hour=7,weekday=1"},
		{CalendarEntry{Day: ip(1), Month: ip(1)}, "day=1,month=1"},
	}
	for _, tt := range tests {
		if got := tt.entry.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}

// mustParseCalendar expands a --calendar value or fails the test.
func mustParseCalendar(t *testing.T, spec string) []CalendarEntry {
	t.Helper()
	entries, err := ParseCalendarFields(spec)
	if err != nil {
		t.Fatalf("ParseCalendarFields(%q) error = %v", spec, err)
	}
	return entries
}

func TestDefinitionScheduleString(t *testing.T) {
	tests := []struct {
		name     string
		calendar []CalendarEntry
		interval int
		want     string
	}{
		{name: "long-lived service", want: ""},
		{name: "interval", interval: 3600, want: "every 1h0m0s"},
		{name: "daily", calendar: []CalendarEntry{at(-1, 7, 30)}, want: "daily 07:30"},
		{name: "one weekday", calendar: timed(17, 0, 5), want: "fri 17:00"},
		{name: "weekdays fold to a range", calendar: timed(7, 30, 1, 2, 3, 4, 5), want: "mon-fri 07:30"},
		{name: "weekend folds across sunday", calendar: timed(9, 0, 0, 6), want: "sat-sun 09:00"},
		{name: "wrapping range", calendar: timed(9, 0, 0, 1, 5, 6), want: "fri-mon 09:00"},
		{name: "scattered days stay listed", calendar: timed(7, 30, 1, 3, 5), want: "mon,wed,fri 07:30"},
		{name: "run plus a day", calendar: timed(7, 30, 1, 2, 3, 6), want: "mon-wed,sat 07:30"},
		{name: "whole week is daily", calendar: timed(7, 30, 0, 1, 2, 3, 4, 5, 6), want: "daily 07:30"},
		{
			name:     "two times",
			calendar: []CalendarEntry{at(-1, 7, 30), at(5, 17, 0)},
			want:     "daily 07:30; fri 17:00",
		},
		{
			name:     "monthly",
			calendar: []CalendarEntry{{Day: ip(1), Hour: ip(6), Minute: ip(0)}},
			want:     "06:00 day=1",
		},
		{
			name:     "yearly",
			calendar: []CalendarEntry{{Day: ip(1), Month: ip(1), Hour: ip(0), Minute: ip(0)}},
			want:     "00:00 day=1 month=1",
		},
		{
			name:     "hour range cross weekdays",
			calendar: mustParseCalendar(t, "hour=9-17,weekday=1-5,minute=0"),
			want:     "mon-fri hour=9-17,minute=0",
		},
		{
			name:     "day range",
			calendar: mustParseCalendar(t, "day=1-7,hour=6,minute=0"),
			want:     "06:00 day=1-7",
		},
		{name: "minute only", calendar: []CalendarEntry{{Minute: ip(30)}}, want: "minute=30"},
		{
			name:     "six-day range",
			calendar: mustParseCalendar(t, "weekday=1-6,hour=8,minute=0"),
			want:     "mon-sat 08:00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := Definition{Calendar: tt.calendar, IntervalSeconds: tt.interval}
			if got := def.ScheduleString(); got != tt.want {
				t.Errorf("ScheduleString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefinitionValidate_Schedule(t *testing.T) {
	tooMany := make([]CalendarEntry, MaxCalendarEntries+1)
	for i := range tooMany {
		tooMany[i] = CalendarEntry{Minute: ip(i % 60), Hour: ip(i / 60)}
	}

	tests := []struct {
		name    string
		def     Definition
		wantErr string
	}{
		{name: "no schedule", def: Definition{}},
		{name: "calendar job", def: Definition{Calendar: []CalendarEntry{at(-1, 7, 30)}}},
		{name: "interval job", def: Definition{IntervalSeconds: 60}},
		{
			name: "calendar job may run at load",
			def:  Definition{Calendar: []CalendarEntry{at(-1, 7, 30)}, RunAtLoad: true},
		},
		{
			name:    "both forms rejected",
			def:     Definition{Calendar: []CalendarEntry{at(-1, 7, 30)}, IntervalSeconds: 60},
			wantErr: "not both",
		},
		{name: "negative interval", def: Definition{IntervalSeconds: -5}, wantErr: "positive"},
		{
			name:    "empty calendar entry",
			def:     Definition{Calendar: []CalendarEntry{{}}},
			wantErr: "sets no field",
		},
		{
			name:    "calendar field out of range",
			def:     Definition{Calendar: []CalendarEntry{{Hour: ip(25)}}},
			wantErr: "hour must be 0-23",
		},
		{
			name:    "calendar with keep alive",
			def:     Definition{Calendar: []CalendarEntry{{Minute: ip(0)}}, KeepAlive: true},
			wantErr: "keep_alive",
		},
		{
			name:    "interval with keep alive",
			def:     Definition{IntervalSeconds: 60, KeepAlive: true},
			wantErr: "keep_alive",
		},
		{name: "too many entries", def: Definition{Calendar: tooMany}, wantErr: "more than the 200"},
		{
			name:    "day with weekday",
			def:     Definition{Calendar: []CalendarEntry{{Day: ip(13), Weekday: ip(5), Hour: ip(13)}}},
			wantErr: "both day and weekday",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := tt.def
			def.Name = "job"
			def.Command = "/usr/bin/true"
			err := def.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestDefinitionScheduled(t *testing.T) {
	if (&Definition{}).Scheduled() {
		t.Error("empty definition reports Scheduled()")
	}
	if !(&Definition{IntervalSeconds: 1}).Scheduled() {
		t.Error("interval definition does not report Scheduled()")
	}
	if !(&Definition{Calendar: []CalendarEntry{{Minute: ip(0)}}}).Scheduled() {
		t.Error("calendar definition does not report Scheduled()")
	}
}
