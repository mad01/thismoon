package store

import (
	"fmt"
	"time"
)

// Reminder is a single scheduled reminder. Times are stored in UTC.
type Reminder struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Body      string     `json:"body,omitempty"`
	Due       time.Time  `json:"due"`
	Repeat    string     `json:"repeat,omitempty"` // "" | "daily" | "weekly" | Go duration ("24h")
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	FiredAt   *time.Time `json:"fired_at,omitempty"`
}

// Reminder status values.
const (
	StatusPending   = "pending"   // armed, will fire at Due
	StatusFired     = "fired"     // one-shot that already fired (terminal)
	StatusDone      = "done"      // marked complete by the user (terminal)
	StatusCancelled = "cancelled" // soft-cancelled, never fires (terminal)
)

// Overdue reports whether a pending reminder's due time has passed. It is a
// derived display flag — the reminder's stored Status stays "pending" until the
// ticker actually fires it.
func Overdue(r Reminder, now time.Time) bool {
	return r.Status == StatusPending && !r.Due.After(now)
}

// ParseRepeat maps a repeat spec to a positive interval. An empty spec means
// no recurrence and returns (0, nil). "daily"/"weekly" are conveniences;
// anything else is parsed as a Go duration (e.g. "24h", "90m"). A non-empty
// spec that resolves to a non-positive interval is an error.
func ParseRepeat(repeat string) (time.Duration, error) {
	switch repeat {
	case "":
		return 0, nil
	case "daily":
		return 24 * time.Hour, nil
	case "weekly":
		return 7 * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(repeat)
	if err != nil {
		return 0, fmt.Errorf(
			"invalid repeat %q: want 'daily', 'weekly', or a Go duration like '24h'",
			repeat,
		)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid repeat %q: interval must be positive", repeat)
	}
	return d, nil
}

// NextDue advances due by the repeat interval until it is strictly after now,
// so a service that was down across several intervals reschedules to the next
// future occurrence rather than firing once per missed interval. It assumes a
// recurring reminder (repeat != ""); callers handle one-shots separately.
func NextDue(due time.Time, repeat string, now time.Time) (time.Time, error) {
	interval, err := ParseRepeat(repeat)
	if err != nil {
		return time.Time{}, err
	}
	if interval <= 0 {
		return time.Time{}, fmt.Errorf("NextDue called with non-recurring repeat %q", repeat)
	}
	next := due
	for !next.After(now) {
		next = next.Add(interval)
	}
	return next, nil
}
