package service

import "testing"

// TestScheduleString_WeekdaySeven covers an entry that still carries
// launchd's other spelling of Sunday, as a hand-edited plist may.
func TestScheduleString_WeekdaySeven(t *testing.T) {
	tests := []struct {
		name     string
		calendar []CalendarEntry
		want     string
	}{
		{name: "seven is sunday", calendar: timed(9, 0, 7), want: "sun 09:00"},
		{name: "seven joins saturday", calendar: timed(9, 0, 6, 7), want: "sat-sun 09:00"},
		{name: "one through seven is the week", calendar: timed(9, 0, 1, 2, 3, 4, 5, 6, 7), want: "daily 09:00"},
		{name: "zero and seven are one day", calendar: timed(9, 0, 0, 7), want: "sun 09:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := Definition{Calendar: tt.calendar}
			if got := def.ScheduleString(); got != tt.want {
				t.Errorf("ScheduleString() = %q, want %q", got, tt.want)
			}
		})
	}
}
