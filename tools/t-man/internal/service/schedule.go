package service

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CalendarEntry is one StartCalendarInterval dict: the job fires every
// minute whose wall-clock fields match every field that is set. A nil field
// is a wildcard, the same as a missing key in the plist. All set fields must
// match (unlike cron, Day and Weekday are not alternatives), and Weekday
// accepts 0 or 7 for Sunday as launchd does.
type CalendarEntry struct {
	Minute  *int `json:"minute,omitempty"`
	Hour    *int `json:"hour,omitempty"`
	Day     *int `json:"day,omitempty"`
	Weekday *int `json:"weekday,omitempty"`
	Month   *int `json:"month,omitempty"`
}

// calendarField is one settable CalendarEntry field with its launchd range.
type calendarField struct {
	name     string
	min, max int
	get      func(*CalendarEntry) **int
}

// calendarFields lists the fields in the order String renders them.
var calendarFields = []calendarField{
	{"minute", 0, 59, func(e *CalendarEntry) **int { return &e.Minute }},
	{"hour", 0, 23, func(e *CalendarEntry) **int { return &e.Hour }},
	{"day", 1, 31, func(e *CalendarEntry) **int { return &e.Day }},
	{"weekday", 0, 7, func(e *CalendarEntry) **int { return &e.Weekday }},
	{"month", 1, 12, func(e *CalendarEntry) **int { return &e.Month }},
}

// validate checks every set field against its launchd range and rejects an
// entry that sets nothing, which would fire every minute.
func (e CalendarEntry) validate() error {
	set := 0
	for _, f := range calendarFields {
		v := *f.get(&e)
		if v == nil {
			continue
		}
		set++
		if *v < f.min || *v > f.max {
			return fmt.Errorf("calendar %s must be %d-%d: %d", f.name, f.min, f.max, *v)
		}
	}
	if set == 0 {
		return fmt.Errorf("calendar entry sets no field (it would fire every minute)")
	}
	return nil
}

// String renders the entry in --calendar syntax: "minute=30,hour=7".
func (e CalendarEntry) String() string {
	var parts []string
	for _, f := range calendarFields {
		if v := *f.get(&e); v != nil {
			parts = append(parts, fmt.Sprintf("%s=%d", f.name, *v))
		}
	}
	return strings.Join(parts, ",")
}

// ParseCalendarFields parses one --calendar value, a comma-separated list of
// launchd field assignments such as "minute=0,hour=7,day=1".
func ParseCalendarFields(spec string) (CalendarEntry, error) {
	var entry CalendarEntry
	for _, pair := range strings.Split(spec, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok {
			return CalendarEntry{}, fmt.Errorf("calendar field %q is not key=value", pair)
		}
		if err := entry.set(strings.ToLower(strings.TrimSpace(key)), value); err != nil {
			return CalendarEntry{}, err
		}
	}
	if err := entry.validate(); err != nil {
		return CalendarEntry{}, err
	}
	return entry, nil
}

// set assigns one named field from its string value.
func (e *CalendarEntry) set(key, value string) error {
	for _, f := range calendarFields {
		if f.name != key {
			continue
		}
		slot := f.get(e)
		if *slot != nil {
			return fmt.Errorf("calendar field %s given twice", key)
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("calendar %s must be an integer: %q", key, value)
		}
		*slot = &n
		return nil
	}
	return fmt.Errorf("unknown calendar field %q (minute, hour, day, weekday, month)", key)
}

// ParseClockSchedule parses one --schedule value: a comma-separated list of
// HH:MM times, each optionally prefixed with a weekday name and "@"
// ("07:30", "mon@07:30,fri@17:00"). A bare time fires daily.
func ParseClockSchedule(spec string) ([]CalendarEntry, error) {
	var entries []CalendarEntry
	for _, item := range strings.Split(spec, ",") {
		entry, err := parseClockItem(strings.TrimSpace(item))
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// parseClockItem parses "[weekday@]HH:MM" into a calendar entry.
func parseClockItem(item string) (CalendarEntry, error) {
	var entry CalendarEntry
	clock := item
	if day, rest, ok := strings.Cut(item, "@"); ok {
		weekday, err := parseWeekday(day)
		if err != nil {
			return CalendarEntry{}, err
		}
		entry.Weekday = &weekday
		clock = rest
	}
	hour, minute, err := parseClock(clock)
	if err != nil {
		return CalendarEntry{}, err
	}
	entry.Hour, entry.Minute = &hour, &minute
	return entry, entry.validate()
}

// parseClock splits "HH:MM" into its two numbers; ranges are checked by
// CalendarEntry.validate.
func parseClock(clock string) (hour, minute int, err error) {
	h, m, ok := strings.Cut(clock, ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, 0, fmt.Errorf("schedule time must be HH:MM: %q", clock)
	}
	if hour, err = strconv.Atoi(h); err != nil {
		return 0, 0, fmt.Errorf("schedule time must be HH:MM: %q", clock)
	}
	if minute, err = strconv.Atoi(m); err != nil {
		return 0, 0, fmt.Errorf("schedule time must be HH:MM: %q", clock)
	}
	return hour, minute, nil
}

// parseWeekday maps a weekday name or its three-letter form to launchd's
// numbering (Sunday is 0).
func parseWeekday(name string) (int, error) {
	want := strings.ToLower(strings.TrimSpace(name))
	for d := time.Sunday; d <= time.Saturday; d++ {
		full := strings.ToLower(d.String())
		if want == full || (len(want) == 3 && want == full[:3]) {
			return int(d), nil
		}
	}
	return 0, fmt.Errorf("unknown weekday %q (sun, mon, tue, wed, thu, fri, sat)", name)
}

// ParseInterval parses one --every value, a Go duration ("1h", "30m"), into
// the whole number of seconds StartInterval takes.
func ParseInterval(spec string) (int, error) {
	d, err := time.ParseDuration(strings.TrimSpace(spec))
	if err != nil {
		return 0, fmt.Errorf("interval must be a duration such as 1h or 30m: %q", spec)
	}
	if d < time.Second || d%time.Second != 0 {
		return 0, fmt.Errorf("interval must be a whole number of seconds, at least 1s: %s", d)
	}
	return int(d / time.Second), nil
}

// Scheduled reports whether the definition is a scheduled job (a calendar
// or an interval trigger) rather than a long-lived service.
func (d *Definition) Scheduled() bool {
	return len(d.Calendar) > 0 || d.IntervalSeconds > 0
}

// Interval returns the StartInterval period, zero for a calendar job or a
// long-lived service.
func (d *Definition) Interval() time.Duration {
	return time.Duration(d.IntervalSeconds) * time.Second
}

// ScheduleString describes the schedule for humans: "every 1h0m0s" or the
// calendar entries in --calendar syntax joined by "; ". Empty when the
// definition is not scheduled.
func (d *Definition) ScheduleString() string {
	if d.IntervalSeconds > 0 {
		return "every " + d.Interval().String()
	}
	parts := make([]string, 0, len(d.Calendar))
	for _, e := range d.Calendar {
		parts = append(parts, e.String())
	}
	return strings.Join(parts, "; ")
}

// validateSchedule enforces the schedule invariants: one trigger kind at a
// time, launchd ranges on every calendar field, and no KeepAlive, which
// would relaunch the job the moment it exits.
func (d *Definition) validateSchedule() error {
	if len(d.Calendar) > 0 && d.IntervalSeconds > 0 {
		return fmt.Errorf("schedule: set calendar or interval_seconds, not both")
	}
	if d.IntervalSeconds < 0 {
		return fmt.Errorf("schedule: interval_seconds must be positive: %d", d.IntervalSeconds)
	}
	for _, e := range d.Calendar {
		if err := e.validate(); err != nil {
			return fmt.Errorf("schedule: %w", err)
		}
	}
	if d.Scheduled() && d.KeepAlive {
		return fmt.Errorf(
			"schedule: a scheduled job cannot set keep_alive " +
				"(launchd would restart it as soon as it exits)",
		)
	}
	return nil
}

// LastLogWrite returns the newest modification time of the job's stdout and
// stderr files. For a scheduled job that is the best local proxy for its
// last run: launchd keeps no run timestamp, and a run that writes nothing
// leaves it unchanged. ok is false when neither file exists.
func (d *Definition) LastLogWrite() (last time.Time, ok bool) {
	times := make([]time.Time, 0, 2)
	for _, path := range []string{d.StandardOutPath, d.StandardErrPath} {
		if path == "" {
			continue
		}
		if info, err := os.Stat(path); err == nil {
			times = append(times, info.ModTime())
		}
	}
	if len(times) == 0 {
		return time.Time{}, false
	}
	sort.Slice(times, func(i, j int) bool { return times[i].After(times[j]) })
	return times[0], true
}
