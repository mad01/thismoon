// Package crashloop detects services stuck in a launchd respawn loop by
// watching the cumulative `runs` counter launchd keeps per job. A healthy
// KeepAlive service accumulates a handful of runs over days; a wedged one
// (crash loop, or the BTM "spawn scheduled" EX_CONFIG state) racks up one
// every ~25 seconds.
package crashloop

import (
	"time"
)

// Alert describes one service that crossed the restart threshold.
type Alert struct {
	Label    string
	Restarts int // restarts observed inside the window
	Window   time.Duration
}

type sample struct {
	at   time.Time
	runs int
}

// Tracker accumulates per-service run-counter samples and reports services
// whose restart rate crosses the threshold. Not safe for concurrent use; the
// monitor calls it from its single poll goroutine.
type Tracker struct {
	window    time.Duration
	threshold int
	cooldown  time.Duration

	samples map[string][]sample
	alerted map[string]time.Time
}

// New returns a Tracker that fires when a service restarts at least threshold
// times inside window, re-alerting for the same service at most once per
// cooldown.
func New(window time.Duration, threshold int, cooldown time.Duration) *Tracker {
	return &Tracker{
		window:    window,
		threshold: threshold,
		cooldown:  cooldown,
		samples:   map[string][]sample{},
		alerted:   map[string]time.Time{},
	}
}

// Observe records the current runs counter for a service and reports whether
// it just crossed the restart threshold. The counter resets when a job is
// re-registered with launchd (bootout/bootstrap), so a drop discards prior
// samples instead of producing a negative delta.
func (t *Tracker) Observe(label string, now time.Time, runs int) (Alert, bool) {
	s := t.samples[label]
	if n := len(s); n > 0 && runs < s[n-1].runs {
		s = nil // job re-registered; counter restarted
	}
	s = append(s, sample{at: now, runs: runs})

	cut := 0
	for cut < len(s)-1 && now.Sub(s[cut].at) > t.window {
		cut++
	}
	s = s[cut:]
	t.samples[label] = s

	restarts := runs - s[0].runs
	if restarts < t.threshold {
		return Alert{}, false
	}
	if last, ok := t.alerted[label]; ok && now.Sub(last) < t.cooldown {
		return Alert{}, false
	}
	t.alerted[label] = now
	return Alert{Label: label, Restarts: restarts, Window: t.window}, true
}

// Forget drops state for services no longer present in discovery so removed
// jobs don't accumulate forever.
func (t *Tracker) Forget(active map[string]bool) {
	for label := range t.samples {
		if !active[label] {
			delete(t.samples, label)
			delete(t.alerted, label)
		}
	}
}
