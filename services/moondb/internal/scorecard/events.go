package scorecard

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/moondb"
)

// The events-service sources a scorecard reads: belt posts each hint it
// shows to belt, tagged hint, event and session_id (and agent_id for a
// subagent's call); its record hooks post lifecycle entries to belt-ledger,
// tagged event and session_id.
const (
	sourceHints  = "belt"
	sourceLedger = "belt-ledger"
)

// The belt ledger events a scorecard counts (tools/belt, internal/ledger).
const (
	ledgerSubagentStart    = "subagent-start"
	ledgerSubagentStop     = "subagent-stop"
	ledgerPermissionDenied = "permission-denied"
	ledgerPreCompact       = "pre-compact"
	ledgerStopFailure      = "stop-failure"
)

// HintEvent is one hint belt showed the session.
type HintEvent struct {
	Hint string
	At   time.Time
}

// hintTitle is a hint entry's title, for one without a hint tag.
var hintTitle = regexp.MustCompile(`^hinted .*\(([^()\s]+)\)$`)

// parseHint reads a belt entry as a hint; false for an entry that is not
// one, such as a guard's denial.
func parseHint(e Event) (HintEvent, bool) {
	hint := e.Tags["hint"]
	if hint == "" {
		if m := hintTitle.FindStringSubmatch(e.Title); m != nil {
			hint = m[1]
		}
	}
	if hint == "" {
		return HintEvent{}, false
	}
	return HintEvent{Hint: hint, At: e.Time}, true
}

// countLedger counts the ledger entries by event.
func countLedger(entries []Event) Ledger {
	var l Ledger
	for _, e := range entries {
		switch e.Tags["event"] {
		case ledgerSubagentStart:
			l.SubagentStart++
		case ledgerSubagentStop:
			l.SubagentStop++
		case ledgerPermissionDenied:
			l.PermissionDenied++
		case ledgerPreCompact:
			l.PreCompact++
		case ledgerStopFailure:
			l.StopFailure++
		}
	}
	return l
}

// EventsResult is what the events service said about one session: the
// hints it was shown and its ledger counts, both nil when the service did
// not answer.
type EventsResult struct {
	Source Source
	Hints  []HintEvent
	Ledger *Ledger
}

// ReadEvents reads a session's belt hints and ledger entries. start is the
// session's first turn start: the events service keeps the newest
// moondb.EventsPerSource entries per source, so when none older than start
// remains, the session's early entries may be gone and the source is
// partial. A nil p, or a service that does not answer, is unavailable.
func ReadEvents(ctx context.Context, p Peers, sessionID string, start time.Time) EventsResult {
	if p == nil {
		return EventsResult{Source: Source{
			Status: StatusUnavailable, Detail: "no events service configured",
		}}
	}
	hints, hintsWhole, err := sessionEntries(ctx, p, sourceHints, sessionID, start)
	if err != nil {
		return EventsResult{Source: Source{Status: StatusUnavailable, Detail: err.Error()}}
	}
	ledger, ledgerWhole, err := sessionEntries(ctx, p, sourceLedger, sessionID, start)
	if err != nil {
		return EventsResult{Source: Source{Status: StatusUnavailable, Detail: err.Error()}}
	}
	out := EventsResult{Source: Source{Status: StatusOK}, Hints: []HintEvent{}}
	for _, e := range hints {
		if h, ok := parseHint(e); ok {
			out.Hints = append(out.Hints, h)
		}
	}
	slices.SortStableFunc(out.Hints, func(a, b HintEvent) int { return a.At.Compare(b.At) })
	counts := countLedger(ledger)
	out.Ledger = &counts
	if missing := partialSources(hintsWhole, ledgerWhole); missing != "" {
		out.Source = Source{Status: StatusPartial, Detail: fmt.Sprintf(
			"the events service keeps the newest %d entries per source and has none older "+
				"than the session's start in %s, so its early entries may be gone",
			moondb.EventsPerSource, missing)}
	}
	return out
}

// partialSources names the sources whose retained entries may not reach
// back to the session's start.
func partialSources(hintsWhole, ledgerWhole bool) string {
	var names []string
	if !hintsWhole {
		names = append(names, sourceHints)
	}
	if !ledgerWhole {
		names = append(names, sourceLedger)
	}
	return strings.Join(names, " and ")
}

// sessionEntries lists one source's entries tagged with the session, and
// reports whether the source still holds an entry older than start.
func sessionEntries(ctx context.Context, p Peers, source, sessionID string, start time.Time,
) (entries []Event, whole bool, err error) {
	all, err := p.Events(ctx, EventQuery{
		Source: source, Q: sessionID, Limit: moondb.EventsPerSource,
	})
	if err != nil {
		return nil, false, err
	}
	for _, e := range all {
		if e.Tags["session_id"] == sessionID {
			entries = append(entries, e)
		}
	}
	if start.IsZero() {
		return entries, true, nil
	}
	older, err := p.Events(ctx, EventQuery{Source: source, Before: eventID(start), Limit: 1})
	if err != nil {
		return nil, false, err
	}
	return entries, len(older) > 0, nil
}

// eventID is the events-service id prefix of time t: its UTC unix
// nanoseconds, zero-padded to 20 digits. Every entry stamped before t has
// a smaller id.
func eventID(t time.Time) string {
	return fmt.Sprintf("%020d", t.UTC().UnixNano())
}
