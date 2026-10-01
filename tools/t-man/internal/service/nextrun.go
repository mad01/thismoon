package service

import "time"

// nextRunHorizonYears bounds the next-run search. Five years covers a
// February 29 entry; anything that never matches inside the horizon (such as
// day=31,month=2) is reported as having no next run.
const nextRunHorizonYears = 5

// NextRun returns the next time the job fires after now. For a calendar job
// it is the first matching minute; an interval job's phase depends on when
// launchd loaded it, so only the calendar form has a computable next run.
// ok is false for interval jobs, unscheduled services, and calendar entries
// that never match.
func (d *Definition) NextRun(now time.Time) (next time.Time, ok bool) {
	if len(d.Calendar) == 0 {
		return time.Time{}, false
	}
	return NextCalendarRun(d.Calendar, now)
}

// NextCalendarRun returns the earliest minute strictly after now at which
// any entry matches, in now's location. ok is false when no entry matches
// within the search horizon.
func NextCalendarRun(entries []CalendarEntry, now time.Time) (next time.Time, ok bool) {
	for _, e := range entries {
		t, found := e.next(now)
		if found && (!ok || t.Before(next)) {
			next, ok = t, true
		}
	}
	return next, ok
}

// next finds the first minute after now matching the entry. It walks the
// clock field by field, jumping each mismatched field to its next candidate
// (the next month, the next day, the wanted hour, the wanted minute) so the
// loop runs a handful of iterations rather than one per minute. Every branch
// moves t strictly forward, so the horizon is the only exit for an entry
// that never matches.
func (e CalendarEntry) next(now time.Time) (time.Time, bool) {
	t := now.Truncate(time.Minute).Add(time.Minute)
	horizon := now.AddDate(nextRunHorizonYears, 0, 0)
	for t.Before(horizon) {
		switch {
		case e.Month != nil && int(t.Month()) != *e.Month:
			t = startOfDay(t.Year(), t.Month()+1, 1, t.Location())
		case e.Day != nil && t.Day() != *e.Day:
			t = nextDay(t, *e.Day)
		case e.Weekday != nil && int(t.Weekday()) != *e.Weekday%7:
			t = startOfDay(t.Year(), t.Month(), t.Day()+1, t.Location())
		case e.Hour != nil && t.Hour() != *e.Hour:
			t = nextHour(t, *e.Hour)
		case e.Minute != nil && t.Minute() != *e.Minute:
			t = nextMinute(t, *e.Minute)
		default:
			return t, true
		}
	}
	return time.Time{}, false
}

// startOfDay is midnight on the given date; time.Date normalises overflow,
// so month 13 or day 32 roll into the next year or month.
func startOfDay(year int, month time.Month, day int, loc *time.Location) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}

// nextDay moves t to midnight of the wanted day of month: later this month
// when it is still ahead, otherwise the first of next month (the loop then
// checks the month again). A wanted day past the month's end rolls over and
// is retried in the following month.
func nextDay(t time.Time, day int) time.Time {
	if t.Day() < day {
		return startOfDay(t.Year(), t.Month(), day, t.Location())
	}
	return startOfDay(t.Year(), t.Month()+1, 1, t.Location())
}

// nextHour moves t to the wanted hour today when it is still ahead,
// otherwise to midnight tomorrow.
func nextHour(t time.Time, hour int) time.Time {
	if t.Hour() < hour {
		return time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, t.Location())
	}
	return startOfDay(t.Year(), t.Month(), t.Day()+1, t.Location())
}

// nextMinute moves t to the wanted minute of this hour when it is still
// ahead, otherwise to the top of the next hour.
func nextMinute(t time.Time, minute int) time.Time {
	if t.Minute() < minute {
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), minute, 0, 0, t.Location())
	}
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
}
