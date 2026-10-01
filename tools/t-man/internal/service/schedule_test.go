package service

import (
	"reflect"
	"strings"
	"testing"
)

// ip returns a pointer to n for building CalendarEntry literals.
func ip(n int) *int { return &n }

func TestParseClockSchedule(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    []CalendarEntry
		wantErr string
	}{
		{
			name: "daily time",
			spec: "07:30",
			want: []CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
		},
		{
			name: "comma list",
			spec: "07:30, 17:00",
			want: []CalendarEntry{
				{Hour: ip(7), Minute: ip(30)},
				{Hour: ip(17), Minute: ip(0)},
			},
		},
		{
			name: "weekday form",
			spec: "mon@07:30,Friday@17:00",
			want: []CalendarEntry{
				{Weekday: ip(1), Hour: ip(7), Minute: ip(30)},
				{Weekday: ip(5), Hour: ip(17), Minute: ip(0)},
			},
		},
		{
			name: "sunday is zero",
			spec: "sun@00:00",
			want: []CalendarEntry{{Weekday: ip(0), Hour: ip(0), Minute: ip(0)}},
		},
		{name: "missing minutes", spec: "7", wantErr: "HH:MM"},
		{name: "single digit hour", spec: "7:30", wantErr: "HH:MM"},
		{name: "hour out of range", spec: "24:00", wantErr: "hour must be 0-23"},
		{name: "minute out of range", spec: "07:60", wantErr: "minute must be 0-59"},
		{name: "unknown weekday", spec: "someday@07:30", wantErr: "unknown weekday"},
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
		want    CalendarEntry
		wantErr string
	}{
		{
			name: "all fields",
			spec: "minute=0,hour=7,day=1,weekday=1,month=1",
			want: CalendarEntry{Minute: ip(0), Hour: ip(7), Day: ip(1), Weekday: ip(1), Month: ip(1)},
		},
		{
			name: "mixed case and spaces",
			spec: " Hour = 7 , minute=30",
			want: CalendarEntry{Minute: ip(30), Hour: ip(7)},
		},
		{name: "hourly on the half", spec: "minute=30", want: CalendarEntry{Minute: ip(30)}},
		{name: "not key=value", spec: "hour", wantErr: "not key=value"},
		{name: "unknown key", spec: "second=5", wantErr: "unknown calendar field"},
		{name: "not an integer", spec: "hour=seven", wantErr: "must be an integer"},
		{name: "repeated key", spec: "hour=7,hour=8", wantErr: "given twice"},
		{name: "day out of range", spec: "day=0", wantErr: "day must be 1-31"},
		{name: "month out of range", spec: "month=13", wantErr: "month must be 1-12"},
		{name: "weekday out of range", spec: "weekday=8", wantErr: "weekday must be 0-7"},
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseCalendarFields(%q) = %v, want %v", tt.spec, got, tt.want)
			}
		})
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
		{CalendarEntry{Hour: ip(7), Minute: ip(30)}, "minute=30,hour=7"},
		{CalendarEntry{Weekday: ip(1), Hour: ip(7), Minute: ip(0)}, "minute=0,hour=7,weekday=1"},
		{CalendarEntry{Day: ip(1), Month: ip(1)}, "day=1,month=1"},
	}
	for _, tt := range tests {
		if got := tt.entry.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}

func TestDefinitionScheduleString(t *testing.T) {
	tests := []struct {
		name string
		def  Definition
		want string
	}{
		{name: "long-lived service", def: Definition{}, want: ""},
		{name: "interval", def: Definition{IntervalSeconds: 3600}, want: "every 1h0m0s"},
		{
			name: "two calendar entries",
			def: Definition{Calendar: []CalendarEntry{
				{Hour: ip(7), Minute: ip(30)},
				{Weekday: ip(5), Hour: ip(17), Minute: ip(0)},
			}},
			want: "minute=30,hour=7; minute=0,hour=17,weekday=5",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.def.ScheduleString(); got != tt.want {
				t.Errorf("ScheduleString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefinitionValidate_Schedule(t *testing.T) {
	tests := []struct {
		name    string
		def     Definition
		wantErr string
	}{
		{name: "no schedule", def: Definition{}},
		{
			name: "calendar job",
			def:  Definition{Calendar: []CalendarEntry{{Hour: ip(7), Minute: ip(30)}}},
		},
		{name: "interval job", def: Definition{IntervalSeconds: 60}},
		{
			name: "calendar job may run at load",
			def: Definition{
				Calendar:  []CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
				RunAtLoad: true,
			},
		},
		{
			name: "both forms rejected",
			def: Definition{
				Calendar:        []CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
				IntervalSeconds: 60,
			},
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
