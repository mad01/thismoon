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
	// AgentID is the subagent it was shown to, "" for the main
	// conversation.
	AgentID string
	// Assertions are the keeper-of-facts assertion ids it showed.
	Assertions []string
	// Pins are the files its pins tag names, by assertion id; nil when the
	// tag is absent. belt tags the first pin of each shown assertion as a
	// bare path, which every assertion of the fire then shares, so these
	// are the fallback for an assertion keeper-of-facts does not hold.
	Pins map[string][]Pin
}

var (
	// kofGetID is how a kof hint's text names an assertion.
	kofGetID = regexp.MustCompile(`kof_get id=([A-Za-z0-9._:-]+)`)
	// hintTitle is a hint entry's title, for one without a hint tag.
	hintTitle = regexp.MustCompile(`^hinted .*\(([^()\s]+)\)$`)
)

// parseHint reads a belt entry as a hint; false for an entry that is not
// one, such as a guard's denial. The assertion ids come from the
// assertions tag (comma-separated) when set, else from the kof_get ids in
// the text. The pins tag is comma-separated too, each entry <id>=<path>,
// or a bare path that every assertion of the hint shares.
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
	h := HintEvent{Hint: hint, At: e.Time, AgentID: e.Tags["agent_id"]}
	h.Assertions = splitList(e.Tags["assertions"])
	if len(h.Assertions) == 0 {
		for _, m := range kofGetID.FindAllStringSubmatch(e.Message, -1) {
			if !slices.Contains(h.Assertions, m[1]) {
				h.Assertions = append(h.Assertions, m[1])
			}
		}
	}
	if raw, ok := e.Tags["pins"]; ok {
		h.Pins = tagPins(raw, h.Assertions)
	}
	return h, true
}

// tagPins reads a pins tag into pins by assertion id.
func tagPins(raw string, ids []string) map[string][]Pin {
	out := make(map[string][]Pin, len(ids))
	for _, id := range ids {
		out[id] = []Pin{}
	}
	for _, entry := range splitList(raw) {
		id, file, ok := strings.Cut(entry, "=")
		if !ok {
			for _, each := range ids {
				out[each] = append(out[each], Pin{File: entry})
			}
			continue
		}
		out[id] = append(out[id], Pin{File: file})
	}
	return out
}

// splitList splits a comma-separated tag, dropping blanks.
func splitList(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
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

// ReadPins looks up in keeper-of-facts the pins of every assertion the
// hints showed. The pins tag cannot stand in: its bare paths belong to the
// whole fire, not to one assertion. It asks once, for the list of every
// assertion, so a scorecard costs keeper-of-facts one request
// (moondb.PeerTimeout) however many assertions it showed. An assertion
// keeper-of-facts no longer holds falls back to the tag in kofStats. nil
// hints, the events service not answering, leave no assertion to look up:
// keeper-of-facts is skipped and says why.
func ReadPins(ctx context.Context, p Peers, hints []HintEvent) (map[string][]Pin, Source) {
	if hints == nil {
		return nil, Source{
			Status: StatusSkipped,
			Detail: "no hints to read assertion ids from: the events service did not answer",
		}
	}
	ids := shownIDs(hints)
	if len(ids) == 0 {
		return nil, Source{
			Status: StatusSkipped,
			Detail: "no assertion to look up: no kof hint was shown",
		}
	}
	if p == nil {
		return nil, Source{Status: StatusUnavailable, Detail: "no keeper-of-facts configured"}
	}
	all, err := p.Assertions(ctx)
	if err != nil {
		return nil, Source{Status: StatusUnavailable, Detail: err.Error()}
	}
	held := make(map[string][]Pin, len(all))
	for _, a := range all {
		held[a.ID] = a.Pins
	}
	pins := make(map[string][]Pin, len(ids))
	var gone []string
	for _, id := range ids {
		ps, ok := held[id]
		if !ok {
			gone = append(gone, id)
			continue
		}
		pins[id] = ps
	}
	if len(gone) > 0 {
		return pins, Source{Status: StatusPartial, Detail: fmt.Sprintf(
			"keeper-of-facts holds no assertion %s", strings.Join(gone, ", "))}
	}
	return pins, Source{Status: StatusOK}
}

// shownIDs lists the assertion ids the hints showed, once each, in the
// order they were first shown.
func shownIDs(hints []HintEvent) []string {
	var ids []string
	for _, h := range hints {
		for _, id := range h.Assertions {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	return ids
}
