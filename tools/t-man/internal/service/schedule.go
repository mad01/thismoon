package service

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MaxCalendarEntries caps how many StartCalendarInterval entries one job may
// carry. A range cross product above it is almost certainly a typo.
const MaxCalendarEntries = 200

// daysInWeek is the modulus for weekday wrap-around.
const daysInWeek = 7

// CalendarEntry is one StartCalendarInterval dict: the job fires every
// minute whose wall-clock fields match every field that is set. A nil field
// is a wildcard, the same as a missing key in the plist. Weekday accepts 0
// or 7 for Sunday as launchd does. launchd treats Day and Weekday as
// alternatives (either one matching fires the job), so an entry that sets
// both is refused; that keeps every remaining field a conjunction, which is
// what the next-run math assumes.
type CalendarEntry struct {
	Minute  *int `json:"minute,omitempty"`
	Hour    *int `json:"hour,omitempty"`
	Day     *int `json:"day,omitempty"`
	Weekday *int `json:"weekday,omitempty"`
	Month   *int `json:"month,omitempty"`
}

// calendarField is one settable CalendarEntry field with its launchd range.
// wraps marks the field whose ranges may run past their end and continue
// from the start (weekday: fri-mon).
type calendarField struct {
	name     string
	min, max int
	wraps    bool
	get      func(*CalendarEntry) **int
}

// calendarFields lists the fields in the order String renders them; the
// slot* constants in schedulefmt.go index into it.
var calendarFields = []calendarField{
	{"minute", 0, 59, false, func(e *CalendarEntry) **int { return &e.Minute }},
	{"hour", 0, 23, false, func(e *CalendarEntry) **int { return &e.Hour }},
	{"day", 1, 31, false, func(e *CalendarEntry) **int { return &e.Day }},
	{"weekday", 0, 7, true, func(e *CalendarEntry) **int { return &e.Weekday }},
	{"month", 1, 12, false, func(e *CalendarEntry) **int { return &e.Month }},
}

// lookupCalendarField finds a field by its lowercase name.
func lookupCalendarField(name string) (calendarField, bool) {
	for _, f := range calendarFields {
		if f.name == name {
			return f, true
		}
	}
	return calendarField{}, false
}

// validate checks every set field against its launchd range, rejects an
// entry that sets nothing (it would fire every minute), and rejects one that
// sets both day and weekday, which launchd would fire on either match.
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
	if e.Day != nil && e.Weekday != nil {
		return fmt.Errorf(
			"calendar entry sets both day and weekday: launchd fires when either one " +
				"matches, so give them as two entries, one with day and one with weekday",
		)
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

// key orders an entry for NormalizeCalendar: weekday, hour, minute, day,
// month, with an unset field sorting first as -1.
func (e CalendarEntry) key() [5]int {
	return [5]int{deref(e.Weekday), deref(e.Hour), deref(e.Minute), deref(e.Day), deref(e.Month)}
}

func deref(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

// NormalizeCalendar returns the entries sorted by key, with Sunday written
// as 0 and duplicates dropped, so the same schedule written two ways
// (mon-fri, weekdays, five separate days) hashes the same. Empty in, nil out.
func NormalizeCalendar(entries []CalendarEntry) []CalendarEntry {
	if len(entries) == 0 {
		return nil
	}
	seen := make(map[[5]int]bool, len(entries))
	out := make([]CalendarEntry, 0, len(entries))
	for _, e := range entries {
		if e.Weekday != nil && *e.Weekday == daysInWeek {
			sunday := 0
			e.Weekday = &sunday
		}
		k := e.key()
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return lessKey(out[i].key(), out[j].key()) })
	return out
}

func lessKey(a, b [5]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// ParseCalendarFields parses one --calendar value, a comma-separated list of
// launchd field assignments such as "minute=0,hour=7,day=1". A value may be
// a range ("hour=9-17", "weekday=1-5"); ranged fields expand to their cross
// product, capped at MaxCalendarEntries. A weekday range may wrap
// ("weekday=5-1" is fri, sat, sun, mon); every other field must run upward.
// day and weekday cannot appear together (see CalendarEntry).
func ParseCalendarFields(spec string) ([]CalendarEntry, error) {
	values, err := parseFieldValues(spec)
	if err != nil {
		return nil, err
	}
	entries, err := expandCalendar(values)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if err := e.validate(); err != nil {
			return nil, err
		}
	}
	return NormalizeCalendar(entries), nil
}

// parseFieldValues splits a --calendar value into each named field's values.
func parseFieldValues(spec string) (map[string][]int, error) {
	values := make(map[string][]int)
	for _, pair := range strings.Split(spec, ",") {
		key, raw, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok {
			return nil, fmt.Errorf("calendar field %q is not key=value", pair)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		field, ok := lookupCalendarField(key)
		if !ok {
			return nil, fmt.Errorf(
				"unknown calendar field %q (minute, hour, day, weekday, month)", key,
			)
		}
		if _, dup := values[key]; dup {
			return nil, fmt.Errorf("calendar field %s given twice", key)
		}
		vals, err := field.parseValues(raw)
		if err != nil {
			return nil, err
		}
		values[key] = vals
	}
	return values, nil
}

// parseValues parses a single value or an a-b range for the field.
func (f calendarField) parseValues(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	lo, hi, isRange := strings.Cut(raw, "-")
	if !isRange {
		n, err := f.parseValue(raw)
		if err != nil {
			return nil, err
		}
		return []int{n}, nil
	}
	if lo == "" || hi == "" {
		return nil, fmt.Errorf("calendar %s range is empty: %q", f.name, raw)
	}
	a, err := f.parseValue(lo)
	if err != nil {
		return nil, err
	}
	b, err := f.parseValue(hi)
	if err != nil {
		return nil, err
	}
	return f.span(a, b)
}

// parseValue parses one integer and checks it against the field's range.
func (f calendarField) parseValue(raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("calendar %s must be an integer: %q", f.name, raw)
	}
	if n < f.min || n > f.max {
		return 0, fmt.Errorf("calendar %s must be %d-%d: %d", f.name, f.min, f.max, n)
	}
	return n, nil
}

// span lists the values from a to b inclusive. Only the wrapping field may
// run downward.
func (f calendarField) span(a, b int) ([]int, error) {
	if f.wraps {
		return weekdaySpan(a, b), nil
	}
	if a > b {
		return nil, fmt.Errorf("calendar %s range must run upward: %d-%d", f.name, a, b)
	}
	return seq(a, b), nil
}

// weekdaySpan lists the days from a to b, continuing past Saturday into
// Sunday when b comes before a (5-1 is fri, sat, sun, mon), with 7 folded
// onto 0 and the result sorted and unique.
func weekdaySpan(a, b int) []int {
	var raw []int
	if a <= b {
		raw = seq(a, b)
	} else {
		raw = append(seq(a, daysInWeek-1), seq(0, b)...)
	}
	for i := range raw {
		raw[i] %= daysInWeek
	}
	return uniqueSorted(raw)
}

// seq lists the integers from a to b inclusive; empty when a > b.
func seq(a, b int) []int {
	if a > b {
		return nil
	}
	out := make([]int, 0, b-a+1)
	for v := a; v <= b; v++ {
		out = append(out, v)
	}
	return out
}

func uniqueSorted(vals []int) []int {
	sort.Ints(vals)
	out := vals[:0]
	for i, v := range vals {
		if i == 0 || v != vals[i-1] {
			out = append(out, v)
		}
	}
	return out
}

// expandCalendar builds the cross product of every field's values, refusing
// one larger than MaxCalendarEntries before allocating it.
func expandCalendar(values map[string][]int) ([]CalendarEntry, error) {
	total := 1
	for _, vals := range values {
		total *= len(vals)
	}
	if total > MaxCalendarEntries {
		return nil, fmt.Errorf(
			"calendar expands to %d entries, more than the %d allowed", total, MaxCalendarEntries,
		)
	}
	entries := []CalendarEntry{{}}
	for _, f := range calendarFields {
		vals, ok := values[f.name]
		if !ok {
			continue
		}
		next := make([]CalendarEntry, 0, len(entries)*len(vals))
		for _, e := range entries {
			for _, v := range vals {
				value := v
				withField := e
				*f.get(&withField) = &value
				next = append(next, withField)
			}
		}
		entries = next
	}
	return entries, nil
}

// ParseClockSchedule parses one --schedule value: a comma-separated list of
// HH:MM times, each optionally prefixed with days and "@". The prefix is a
// weekday name (mon@07:30), a range that may wrap (mon-fri@07:30,
// fri-mon@09:00), or one of the sets weekdays, weekend, and daily. A bare
// time fires daily. Every day becomes its own entry, since launchd takes one
// Weekday per dict; the result is normalized.
func ParseClockSchedule(spec string) ([]CalendarEntry, error) {
	var entries []CalendarEntry
	for _, item := range strings.Split(spec, ",") {
		expanded, err := parseClockItem(strings.TrimSpace(item))
		if err != nil {
			return nil, err
		}
		entries = append(entries, expanded...)
	}
	entries = NormalizeCalendar(entries)
	if len(entries) > MaxCalendarEntries {
		return nil, fmt.Errorf(
			"schedule expands to %d entries, more than the %d allowed",
			len(entries), MaxCalendarEntries,
		)
	}
	return entries, nil
}

// parseClockItem parses "[days@]HH:MM" into one entry per day.
func parseClockItem(item string) ([]CalendarEntry, error) {
	days := []*int{nil}
	clock := item
	if prefix, rest, ok := strings.Cut(item, "@"); ok {
		parsed, err := parseDaySpec(prefix)
		if err != nil {
			return nil, err
		}
		days, clock = parsed, rest
	}
	hour, minute, err := parseClock(clock)
	if err != nil {
		return nil, err
	}
	entries := make([]CalendarEntry, 0, len(days))
	for _, day := range days {
		h, m := hour, minute
		e := CalendarEntry{Weekday: day, Hour: &h, Minute: &m}
		if err := e.validate(); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// daySets are the named weekday sets a --schedule prefix accepts, besides
// "daily" which means no weekday at all.
var daySets = map[string][]int{
	"weekdays": {1, 2, 3, 4, 5},
	"weekend":  {0, 6},
}

// parseDaySpec expands a --schedule prefix into weekday numbers: a set
// name, a range (wrapping allowed), or one day. "daily" yields a single
// nil, the wildcard.
func parseDaySpec(spec string) ([]*int, error) {
	name := strings.ToLower(strings.TrimSpace(spec))
	if name == "daily" {
		return []*int{nil}, nil
	}
	if set, ok := daySets[name]; ok {
		return intPtrs(set), nil
	}
	if lo, hi, isRange := strings.Cut(name, "-"); isRange {
		if lo == "" || hi == "" {
			return nil, fmt.Errorf("weekday range is empty: %q", spec)
		}
		a, err := parseWeekday(lo)
		if err != nil {
			return nil, err
		}
		b, err := parseWeekday(hi)
		if err != nil {
			return nil, err
		}
		return intPtrs(weekdaySpan(a, b)), nil
	}
	d, err := parseWeekday(name)
	if err != nil {
		return nil, err
	}
	return intPtrs([]int{d}), nil
}

func intPtrs(vals []int) []*int {
	out := make([]*int, len(vals))
	for i, v := range vals {
		value := v
		out[i] = &value
	}
	return out
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
	return 0, fmt.Errorf(
		"unknown weekday %q (sun-sat, a range such as mon-fri, or weekdays, weekend, daily)",
		name,
	)
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

// validateSchedule enforces the schedule invariants: one trigger kind at a
// time, launchd ranges on every calendar field, the entry cap, and no
// KeepAlive, which would relaunch the job the moment it exits.
func (d *Definition) validateSchedule() error {
	if len(d.Calendar) > 0 && d.IntervalSeconds > 0 {
		return fmt.Errorf("schedule: set calendar or interval_seconds, not both")
	}
	if d.IntervalSeconds < 0 {
		return fmt.Errorf("schedule: interval_seconds must be positive: %d", d.IntervalSeconds)
	}
	if len(d.Calendar) > MaxCalendarEntries {
		return fmt.Errorf(
			"schedule: %d calendar entries, more than the %d allowed",
			len(d.Calendar), MaxCalendarEntries,
		)
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
