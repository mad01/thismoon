package config

import (
	"fmt"
	"strings"
	"time"
)

// Events-service sources the lifecycle commands default to. internal/ledger
// aliases the ledger one; both live here because the config file can
// replace them and this package resolves what it reads.
const (
	DefaultLedgerSource = "belt-ledger" // what `belt record` posts under
	DefaultWatchSource  = "belt-inbox"  // what `belt watch` polls for items
)

// Watch defaults and limits.
const (
	DefaultWatchPollInterval = 2 * time.Second
	DefaultWatchMaxWait      = 540 * time.Second
	// MinWatchPollInterval floors poll_interval_ms: below it a watcher on
	// an idle session turns into a busy loop against the events service.
	MinWatchPollInterval = 100 * time.Millisecond
	// MaxWatchPollInterval caps poll_interval_ms. No wait lasts longer than
	// MaxWatchMaxWait, so a slower poll would never poll a second time.
	MaxWatchPollInterval = time.Hour
	// MaxWatchMaxWait caps max_wait_s. A hook timeout can be raised past
	// the documented one, so the cap sits well above it; past an hour a
	// watcher is a process left behind, not a wait.
	MaxWatchMaxWait = time.Hour
	// WatchHookTimeout is the timeout the documented wiring gives the watch
	// hook, under Claude Code's 600 s default. An asyncRewake hook is
	// killed at its timeout, so a max wait at or past it never gets to exit
	// on its own; doctor warns about one.
	WatchHookTimeout = 590 * time.Second
)

// The watch limits as raw config counts. Every bound check compares the
// count as written before any conversion to time.Duration: a large enough
// count overflows that multiplication, wraps negative, and would slip past
// a check made on the converted value.
const (
	minPollMS = int(MinWatchPollInterval / time.Millisecond)
	maxPollMS = int(MaxWatchPollInterval / time.Millisecond)
	maxWaitS  = int(MaxWatchMaxWait / time.Second)
)

// maxSourceLen is the events service's cap on a source name.
const maxSourceLen = 64

// The ledger and watch values are resolved, never rejected. A rejected
// value is a config error, and a config error makes every guarded tool call
// deny (docs/adr/0008), which is far too much for a typo in a feature that
// may not even be on. Each accessor below returns the value in effect, and
// Adjustments names every value that differs from what the file says, for
// `belt doctor` to warn about.

// Ledger configures what `belt record` posts. With no ledger section every
// record event posts under DefaultLedgerSource: wiring the record hooks in
// the Claude settings is the opt-in, as it is for hints.
type Ledger struct {
	Enabled *bool  `toml:"enabled" yaml:"enabled,omitempty"`
	Source  string `toml:"source"  yaml:"source,omitempty"`
	// AgentTypes maps a subagent type to the model and effort its
	// definition declares, tagged on that type's subagent-start entries.
	// Unlike the other lifecycle keys it tolerates any shape, a wrong YAML
	// type included (see AgentTypes).
	AgentTypes AgentTypes `toml:"agent_types" yaml:"agent_types,omitempty"`
}

// On reports whether record events post at all; unset means yes.
func (l Ledger) On() bool { return l.Enabled == nil || *l.Enabled }

// SourceName is the events-service source entries post under: source in
// the form the events service stores it, or the default when unset or
// when nothing of it survives.
func (l Ledger) SourceName() string {
	return sourceOrDefault(l.Source, DefaultLedgerSource)
}

// Adjustments names the ledger values belt uses in a different form than
// the file states: a rewritten source, and every agent_types entry or
// value it dropped.
func (l Ledger) Adjustments() []string {
	out := sourceAdjustment("ledger.source", l.Source, l.SourceName())
	return append(out, l.AgentTypes.Adjustments()...)
}

// Watch configures `belt watch`. It is the one belt feature that is off
// until a config turns it on: a watcher holds a process per idle session
// for up to its max wait, which a machine should choose rather than
// inherit from a hook entry.
type Watch struct {
	Enabled        bool   `toml:"enabled"          yaml:"enabled"`
	PollIntervalMS int    `toml:"poll_interval_ms" yaml:"poll_interval_ms,omitempty"`
	MaxWaitS       int    `toml:"max_wait_s"       yaml:"max_wait_s,omitempty"`
	Source         string `toml:"source"           yaml:"source,omitempty"`
}

// PollInterval is how often the watcher asks the events service for items:
// the default when unset or negative, clamped between the floor and the cap.
func (w Watch) PollInterval() time.Duration {
	switch {
	case w.PollIntervalMS <= 0:
		return DefaultWatchPollInterval
	case w.PollIntervalMS < minPollMS:
		return MinWatchPollInterval
	case w.PollIntervalMS > maxPollMS:
		return MaxWatchPollInterval
	}
	return time.Duration(w.PollIntervalMS) * time.Millisecond
}

// MaxWait is how long one watcher waits for an item before it exits: the
// default when unset or negative, never past the cap.
func (w Watch) MaxWait() time.Duration {
	switch {
	case w.MaxWaitS <= 0:
		return DefaultWatchMaxWait
	case w.MaxWaitS > maxWaitS:
		return MaxWatchMaxWait
	}
	return time.Duration(w.MaxWaitS) * time.Second
}

// SourceName is the events-service source the watcher polls, resolved
// like the ledger's.
func (w Watch) SourceName() string {
	return sourceOrDefault(w.Source, DefaultWatchSource)
}

// Adjustments names the watch values belt uses in a different form than
// the file states: a negative number, a poll faster than the floor, a wait
// past the cap, or a source the events service would rewrite.
func (w Watch) Adjustments() []string {
	out := w.pollAdjustment()
	out = append(out, w.waitAdjustment()...)
	return append(out, sourceAdjustment("watch.source", w.Source, w.SourceName())...)
}

func (w Watch) pollAdjustment() []string {
	switch {
	case w.PollIntervalMS < 0:
		return []string{fmt.Sprintf("watch.poll_interval_ms is %d, using the default %s",
			w.PollIntervalMS, w.PollInterval())}
	case w.PollIntervalMS > 0 && w.PollIntervalMS < minPollMS:
		return []string{fmt.Sprintf("watch.poll_interval_ms is %d, using the %s floor "+
			"(anything faster is a busy loop against the events service)",
			w.PollIntervalMS, w.PollInterval())}
	case w.PollIntervalMS > maxPollMS:
		return []string{fmt.Sprintf("watch.poll_interval_ms is %d, using the %s cap",
			w.PollIntervalMS, w.PollInterval())}
	}
	return nil
}

func (w Watch) waitAdjustment() []string {
	switch {
	case w.MaxWaitS < 0:
		return []string{fmt.Sprintf("watch.max_wait_s is %d, using the default %s",
			w.MaxWaitS, w.MaxWait())}
	case w.MaxWaitS > maxWaitS:
		return []string{fmt.Sprintf("watch.max_wait_s is %d, using the %s cap",
			w.MaxWaitS, w.MaxWait())}
	}
	return nil
}

// sourceAdjustment reports a set source that resolved to another name.
func sourceAdjustment(key, set, effective string) []string {
	if set == "" || set == effective {
		return nil
	}
	return []string{fmt.Sprintf(
		"%s is %q, using %q (the events service keeps lowercase letters, digits, - and _ only)",
		key, set, effective,
	)}
}

func sourceOrDefault(set, fallback string) string {
	if s := sanitizeSource(set); s != "" {
		return s
	}
	return fallback
}

// sanitizeSource rewrites a source name the way the events service does on
// the way in, so belt posts and polls under the name the service stores:
// lowercased, every run of other characters turned into one dash, dashes
// trimmed from both ends, and the length capped. It returns "" when
// nothing usable survives.
func sanitizeSource(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		c := '-'
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			c = r
		}
		if c == '-' && strings.HasSuffix(b.String(), "-") {
			continue
		}
		b.WriteRune(c)
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > maxSourceLen {
		out = strings.Trim(out[:maxSourceLen], "-")
	}
	return out
}
