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
// loop runs a handful of iterations rather than one per minute. A jump that
// does not land strictly after t (a wall time that a DST change made
// ambiguous or absent) is replaced by the next minute, so t only ever moves
// forward and the horizon is the only exit for an entry that never matches.
func (e CalendarEntry) next(now time.Time) (time.Time, bool) {
	t := now.Truncate(time.Minute).Add(time.Minute)
	horizon := now.AddDate(nextRunHorizonYears, 0, 0)
	for t.Before(horizon) {
		candidate, matched := e.advance(t)
		if matched {
			return t, true
		}
		if !candidate.After(t) {
			candidate = t.Add(time.Minute)
		}
		t = candidate
	}
	return time.Time{}, false
}

// advance reports matched when every set field agrees with t's wall clock;
// otherwise it returns the first mismatched field's next candidate time.
func (e CalendarEntry) advance(t time.Time) (candidate time.Time, matched bool) {
	switch {
	case e.Month != nil && int(t.Month()) != *e.Month:
		return startOfDay(t.Year(), t.Month()+1, 1, t.Location()), false
	case e.Day != nil && t.Day() != *e.Day:
		return nextDay(t, *e.Day), false
	case e.Weekday != nil && int(t.Weekday()) != *e.Weekday%7:
		return startOfDay(t.Year(), t.Month(), t.Day()+1, t.Location()), false
	case e.Hour != nil && t.Hour() != *e.Hour:
		return nextHour(t, *e.Hour), false
	case e.Minute != nil && t.Minute() != *e.Minute:
		return nextMinute(t, *e.Minute), false
	}
	return t, true
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
// otherwise to midnight tomorrow. Two candidates compete for the same-day
// jump, because each is wrong on one side of a DST change: the wall time
// rebuilt with time.Date is right across a spring-forward gap (adding
// elapsed hours would overshoot by one), while t plus the elapsed hours is
// right across a fall-back repeat (time.Date picks the later copy of a
// repeated hour). The earliest candidate after t that reads the wanted hour
// wins. When neither does, the gap swallowed the hour, and the earliest
// forward candidate is returned for the loop to move on from.
func nextHour(t time.Time, hour int) time.Time {
	if t.Hour() >= hour {
		return startOfDay(t.Year(), t.Month(), t.Day()+1, t.Location())
	}
	rebuilt := time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, t.Location())
	elapsed := t.Add(
		time.Duration(hour-t.Hour())*time.Hour - time.Duration(t.Minute())*time.Minute,
	)
	return pickHour(t, hour, rebuilt, elapsed)
}

// pickHour returns the earliest candidate after t whose wall clock reads
// hour, else the earliest candidate after t, else the zero time (which the
// caller's forward guard turns into the next minute).
func pickHour(t time.Time, hour int, candidates ...time.Time) time.Time {
	var best, forward time.Time
	for _, c := range candidates {
		if !c.After(t) {
			continue
		}
		if forward.IsZero() || c.Before(forward) {
			forward = c
		}
		if c.Hour() == hour && (best.IsZero() || c.Before(best)) {
			best = c
		}
	}
	if !best.IsZero() {
		return best
	}
	return forward
}

// nextMinute moves t to the wanted minute of this hour when it is still
// ahead, otherwise to the top of the next hour, by adding elapsed time for
// the same reason as nextHour.
func nextMinute(t time.Time, minute int) time.Time {
	if t.Minute() < minute {
		return t.Add(time.Duration(minute-t.Minute()) * time.Minute)
	}
	return t.Add(time.Duration(60-t.Minute()) * time.Minute)
}
