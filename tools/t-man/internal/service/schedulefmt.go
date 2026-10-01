package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Indices into calendarFields, which the display folding addresses by slot.
const (
	slotMinute = iota
	slotHour
	slotDay
	slotWeekday
	slotMonth
)

// foldOrder is the sequence the slots are folded in: weekday first, so five
// daily entries read as mon-fri, then hour, day, month, and minute.
var foldOrder = []int{slotWeekday, slotHour, slotDay, slotMonth, slotMinute}

// ScheduleString describes the schedule for humans: "every 1h0m0s", or the
// calendar entries folded back into runs ("mon-fri 07:30", "06:00 day=1",
// "mon-fri hour=9-17,minute=0") joined by "; ". Empty when the definition
// is not scheduled.
func (d *Definition) ScheduleString() string {
	if d.IntervalSeconds > 0 {
		return "every " + d.Interval().String()
	}
	segments := foldCalendar(d.Calendar)
	parts := make([]string, 0, len(segments))
	for _, s := range segments {
		parts = append(parts, s.String())
	}
	return strings.Join(parts, "; ")
}

// segment is a run of calendar entries that agree on every slot except the
// folded ones. Each value is "" for unset, a number, or a folded run such
// as "1-5" or "1,3,5".
type segment struct {
	values [5]string
}

// foldCalendar turns entries into display segments by folding one slot at a
// time in foldOrder; each pass merges the segments that agree on every
// other slot.
func foldCalendar(entries []CalendarEntry) []segment {
	segments := make([]segment, 0, len(entries))
	for _, e := range entries {
		segments = append(segments, newSegment(e))
	}
	for _, slot := range foldOrder {
		segments = foldSlot(segments, slot)
	}
	return segments
}

// newSegment copies an entry's fields as strings, writing Sunday as 0 so a
// weekday of 7 (valid for launchd, never written by t-man) folds and names
// like any other Sunday.
func newSegment(e CalendarEntry) segment {
	var s segment
	for i, f := range calendarFields {
		if v := *f.get(&e); v != nil {
			s.values[i] = strconv.Itoa(*v)
		}
	}
	if e.Weekday != nil {
		s.values[slotWeekday] = strconv.Itoa(*e.Weekday % daysInWeek)
	}
	return s
}

// foldSlot merges segments that are identical except in slot, replacing the
// slot with the folded run of their values. A segment whose slot is unset
// passes through, and first-seen order is kept.
func foldSlot(segments []segment, slot int) []segment {
	var out []segment
	position := map[string]int{}
	collected := map[int][]int{}
	for _, s := range segments {
		n, err := strconv.Atoi(s.values[slot])
		if err != nil {
			out = append(out, s)
			continue
		}
		key := s.keyWithout(slot)
		pos, ok := position[key]
		if !ok {
			pos = len(out)
			position[key] = pos
			out = append(out, s)
		}
		collected[pos] = append(collected[pos], n)
	}
	for pos, vals := range collected {
		out[pos].values[slot] = foldRuns(vals, calendarFields[slot].wraps)
	}
	return out
}

// keyWithout identifies the segment by every slot but one.
func (s segment) keyWithout(slot int) string {
	k := s.values
	k[slot] = "*"
	return strings.Join(k[:], "|")
}

// foldRuns renders values as runs: "1-5", "9-17", "1,3,5". With wrap, a run
// ending on Saturday joins one starting on Sunday (6-0, 5-1) so a weekend
// or a fri-mon span reads as one range.
func foldRuns(vals []int, wrap bool) string {
	type run struct{ lo, hi int }
	var runs []run
	for _, v := range uniqueSorted(vals) {
		if n := len(runs); n > 0 && runs[n-1].hi+1 == v {
			runs[n-1].hi = v
			continue
		}
		runs = append(runs, run{v, v})
	}
	if last := len(runs) - 1; wrap && last > 0 && runs[0].lo == 0 && runs[last].hi == daysInWeek-1 {
		runs = append([]run{{runs[last].lo, runs[0].hi}}, runs[1:last]...)
	}
	parts := make([]string, 0, len(runs))
	for _, r := range runs {
		if r.lo == r.hi {
			parts = append(parts, strconv.Itoa(r.lo))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", r.lo, r.hi))
		}
	}
	return strings.Join(parts, ",")
}

// String renders a segment: the days, the time, then day and month.
func (s segment) String() string {
	var parts []string
	if days := s.dayPrefix(); days != "" {
		parts = append(parts, days)
	}
	if clock := s.clock(); clock != "" {
		parts = append(parts, clock)
	}
	for _, slot := range []int{slotDay, slotMonth} {
		if v := s.values[slot]; v != "" {
			parts = append(parts, calendarFields[slot].name+"="+v)
		}
	}
	return strings.Join(parts, " ")
}

// dayPrefix names the weekdays ("mon-fri", "sat-sun", "mon,wed,fri"),
// says "daily" for a timed entry no weekday, day, or month narrows, and is
// empty otherwise.
func (s segment) dayPrefix() string {
	if wd := s.values[slotWeekday]; wd != "" {
		return weekdayNames(wd)
	}
	if s.values[slotDay] == "" && s.values[slotMonth] == "" && s.values[slotHour] != "" {
		return "daily"
	}
	return ""
}

// clock renders hour and minute as HH:MM when both are single values, and
// as key=value fields when either is folded or unset.
func (s segment) clock() string {
	h, herr := strconv.Atoi(s.values[slotHour])
	m, merr := strconv.Atoi(s.values[slotMinute])
	if herr == nil && merr == nil {
		return fmt.Sprintf("%02d:%02d", h, m)
	}
	var parts []string
	for _, slot := range []int{slotHour, slotMinute} {
		if v := s.values[slot]; v != "" {
			parts = append(parts, calendarFields[slot].name+"="+v)
		}
	}
	return strings.Join(parts, ",")
}

// weekdayNames turns a folded weekday run ("1-5", "6-0", "1,3,5") into
// names; the whole week reads as daily.
func weekdayNames(folded string) string {
	if folded == "0-6" {
		return "daily"
	}
	parts := strings.Split(folded, ",")
	for i, p := range parts {
		if lo, hi, isRange := strings.Cut(p, "-"); isRange {
			parts[i] = dayName(lo) + "-" + dayName(hi)
		} else {
			parts[i] = dayName(p)
		}
	}
	return strings.Join(parts, ",")
}

// dayName renders a weekday number as its three-letter name; 7 is Sunday.
func dayName(n string) string {
	d, _ := strconv.Atoi(n)
	return strings.ToLower(time.Weekday(d % daysInWeek).String()[:3])
}
