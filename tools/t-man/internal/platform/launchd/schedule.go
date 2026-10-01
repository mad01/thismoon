package launchd

import (
	"fmt"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

// CalendarInterval is one StartCalendarInterval dict as launchd reads it. A
// nil field is omitted from the plist, which launchd treats as a wildcard.
type CalendarInterval struct {
	Minute  *int `plist:"Minute,omitempty"`
	Hour    *int `plist:"Hour,omitempty"`
	Day     *int `plist:"Day,omitempty"`
	Weekday *int `plist:"Weekday,omitempty"`
	Month   *int `plist:"Month,omitempty"`
}

// CalendarIntervals is StartCalendarInterval as t-man reads and writes it.
// launchd accepts a single dict or an array of dicts, so decoding takes
// either; encoding always produces the array form.
type CalendarIntervals []CalendarInterval

// UnmarshalPlist implements plist.Unmarshaler: an array of dicts decodes as
// is, and a single dict becomes a one-element list.
func (c *CalendarIntervals) UnmarshalPlist(unmarshal func(any) error) error {
	var many []CalendarInterval
	if err := unmarshal(&many); err == nil {
		*c = many
		return nil
	}
	var one CalendarInterval
	if err := unmarshal(&one); err != nil {
		return fmt.Errorf("StartCalendarInterval is neither a dict nor an array of dicts: %w", err)
	}
	*c = CalendarIntervals{one}
	return nil
}

// calendarToPlist maps definition entries onto the plist shape. A nil input
// stays nil so an unscheduled service renders no StartCalendarInterval key.
func calendarToPlist(entries []service.CalendarEntry) CalendarIntervals {
	if len(entries) == 0 {
		return nil
	}
	out := make(CalendarIntervals, len(entries))
	for i, e := range entries {
		out[i] = CalendarInterval{
			Minute: e.Minute, Hour: e.Hour, Day: e.Day, Weekday: e.Weekday, Month: e.Month,
		}
	}
	return out
}

// calendarFromPlist is the inverse of calendarToPlist, so a parsed plist
// hashes identically to the definition that produced it.
func calendarFromPlist(intervals CalendarIntervals) []service.CalendarEntry {
	if len(intervals) == 0 {
		return nil
	}
	out := make([]service.CalendarEntry, len(intervals))
	for i, c := range intervals {
		out[i] = service.CalendarEntry{
			Minute: c.Minute, Hour: c.Hour, Day: c.Day, Weekday: c.Weekday, Month: c.Month,
		}
	}
	return out
}

// RunState is what launchd reports about one managed service right now,
// mapped to the status vocabulary list and status print.
type RunState struct {
	// Status is "running" while a process is live. For a long-lived service
	// that is otherwise launchd's own state ("stopped", "error", "waiting",
	// "spawn scheduled"); for a scheduled job idle between runs it is
	// "scheduled", because not running is that job's healthy state.
	Status string
	// Running reports whether launchd has a live process for the service.
	Running bool
	// LastExitCode is launchd's last exit status verbatim; empty when
	// launchd did not report one.
	LastExitCode string
}

// runStateFor maps a raw launchctl status onto RunState. Only a scheduled
// job's idle state is rewritten; long-lived services keep launchd's words.
func runStateFor(def *service.Definition, st *ServiceStatus) *RunState {
	running := st.PID != "" && st.PID != "-"
	status := st.Status
	if def.Scheduled() && !running {
		status = "scheduled"
	}
	return &RunState{Status: status, Running: running, LastExitCode: st.LastExitCode}
}
