// Package timespan parses the time windows that since flags and parameters
// take across the fleet: a span before now such as 90m, 24h or 7d, or an
// RFC 3339 time. One grammar means an agent that learned `since: "24h"` on
// one tool can pass the same value to the next.
package timespan

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Grammar names what Since accepts, for help text and error messages.
const Grammar = "an RFC 3339 time or a positive span such as 90m, 24h or 7d"

// day is the length the d suffix stands for.
const day = 24 * time.Hour

// Parse reads a positive span: a Go duration ("90m", "24h") or whole days
// ("7d"), the unit people reach for first when asking about recent work.
func Parse(raw string) (time.Duration, error) {
	d, err := parse(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("must be a positive span such as 90m, 24h or 7d, got %q", raw)
	}
	return d, nil
}

// Since reads a lower time bound: an RFC 3339 time, or a positive span
// before now. Empty means no bound and returns the zero time. The error
// starts with "must be", so a caller can prefix the parameter name.
func Since(raw string, now time.Time) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	d, err := parse(raw)
	if err != nil || d <= 0 {
		return time.Time{}, fmt.Errorf("must be %s, got %q", Grammar, raw)
	}
	return now.Add(-d), nil
}

// parse is time.ParseDuration plus whole days.
func parse(raw string) (time.Duration, error) {
	days, ok := strings.CutSuffix(raw, "d")
	if !ok {
		return time.ParseDuration(raw)
	}
	n, err := strconv.Atoi(days)
	if err != nil {
		return 0, err
	}
	return time.Duration(n) * day, nil
}
