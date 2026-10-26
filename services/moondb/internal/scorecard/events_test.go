package scorecard

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// kofMessage is a kof hint's text as belt renders it (tools/belt,
// internal/hint/kofassert.go): the first pin of each assertion, then its
// kof_get id.
const kofMessage = "2 stored assertion(s) for repo:acme/repo/services/x - prior sessions " +
	"derived these; treat stale ones as needing re-verification.\n" +
	"  [fresh] the server binds to loopback only\n" +
	"          services/x/serve.go:40-52 (kof_get id=01792948229235707000-1e1c)\n" +
	"  [stale] the cache holds 512 traces\n" +
	"          services/x/facts.go:10-12 (kof_get id=01792943011031490000-016f)\n" +
	"  Correct one that proves wrong with kof_retract rather than working around it."

func TestParseHint(t *testing.T) {
	tests := []struct {
		name   string
		event  Event
		ok     bool
		want   HintEvent
		pinned map[string][]Pin
	}{
		{
			name: "kof ids from the text",
			event: Event{
				Time: t0, Title: "hinted mcp__csl__csl_search (kof-assertions)", Message: kofMessage,
				Tags: map[string]string{
					"hint": "kof-assertions", "event": "search", "session_id": "s1", "agent_id": "a1",
				},
			},
			ok: true,
			want: HintEvent{Hint: "kof-assertions", At: t0, AgentID: "a1", Assertions: []string{
				"01792948229235707000-1e1c", "01792943011031490000-016f",
			}},
		},
		{
			name: "ids and pins from the tags",
			event: Event{Time: t0, Message: "ignored kof_get id=other", Tags: map[string]string{
				"hint": "kof-consult", "assertions": "id1, id2",
				"pins": "id1=services/x/a.go,id2=services/y/b.go,id1=services/x/c.go",
			}},
			ok:   true,
			want: HintEvent{Hint: "kof-consult", At: t0, Assertions: []string{"id1", "id2"}},
			pinned: map[string][]Pin{
				"id1": {{File: "services/x/a.go"}, {File: "services/x/c.go"}},
				"id2": {{File: "services/y/b.go"}},
			},
		},
		{
			name: "bare pins belong to every assertion",
			event: Event{Time: t0, Tags: map[string]string{
				"hint": "kof-consult", "assertions": "id1,id2", "pins": "services/x/a.go",
			}},
			ok:   true,
			want: HintEvent{Hint: "kof-consult", At: t0, Assertions: []string{"id1", "id2"}},
			pinned: map[string][]Pin{
				"id1": {{File: "services/x/a.go"}}, "id2": {{File: "services/x/a.go"}},
			},
		},
		{
			name: "hint id from the title",
			event: Event{
				Time:    t0,
				Title:   "hinted Bash (prefer-csl)",
				Message: "Prefer: csl_search",
			},
			ok:   true,
			want: HintEvent{Hint: "prefer-csl", At: t0},
		},
		{
			name: "a guard's denial is not a hint",
			event: Event{Title: "blocked Bash (git-discard)", Tags: map[string]string{
				"guard": "git-discard", "session_id": "s1",
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseHint(tt.event)
			if ok != tt.ok {
				t.Fatalf("ok = %t, want %t", ok, tt.ok)
			}
			pins := got.Pins
			got.Pins = nil
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("hint = %+v, want %+v", got, tt.want)
			}
			if fmt.Sprint(pins) != fmt.Sprint(tt.pinned) {
				t.Errorf("pins = %v, want %v", pins, tt.pinned)
			}
		})
	}
}

func TestCountLedger(t *testing.T) {
	var entries []Event
	for event, n := range map[string]int{
		"subagent-start": 3, "subagent-stop": 5, "permission-denied": 1, "pre-compact": 2,
		"post-compact": 2, "stop-failure": 1, "teammate-idle": 4,
	} {
		for range n {
			entries = append(entries, Event{Tags: map[string]string{"event": event}})
		}
	}
	want := Ledger{
		SubagentStart: 3, SubagentStop: 5, PermissionDenied: 1, PreCompact: 2, StopFailure: 1,
	}
	if got := countLedger(entries); got != want {
		t.Errorf("ledger = %+v, want %+v", got, want)
	}
}

// fakePeers answers from fixed entries and assertions, as the events
// service and keeper-of-facts filter them, and counts the assertion
// listings it was asked for.
type fakePeers struct {
	events     map[string][]Event // by source, newest first
	eventsErr  error
	assertions []Assertion
	kofErr     error
	listings   int
}

func (f *fakePeers) Events(_ context.Context, q EventQuery) ([]Event, error) {
	if f.eventsErr != nil {
		return nil, f.eventsErr
	}
	var out []Event
	for _, e := range f.events[q.Source] {
		if q.Before != "" && e.ID >= q.Before {
			continue
		}
		if q.Q != "" && !strings.Contains(e.Message+fmt.Sprint(e.Tags), q.Q) {
			continue
		}
		out = append(out, e)
	}
	return out[:min(len(out), q.Limit)], nil
}

func (f *fakePeers) Assertions(context.Context) ([]Assertion, error) {
	f.listings++
	if f.kofErr != nil {
		return nil, f.kofErr
	}
	return f.assertions, nil
}

// entry is an events entry stamped sec seconds after t0, tagged with the
// session.
func entry(sec int, session string, tags map[string]string) Event {
	when := t0.Add(time.Duration(sec) * time.Second)
	e := Event{ID: eventID(when) + "-0001", Time: when, Tags: map[string]string{}}
	for k, v := range tags {
		e.Tags[k] = v
	}
	if session != "" {
		e.Tags["session_id"] = session
	}
	return e
}

func TestReadEvents(t *testing.T) {
	older := entry(-3600, "s0", map[string]string{"hint": "prefer-csl"})
	mention := entry(30, "s2", map[string]string{"hint": "prefer-csl"})
	mention.Message = "this message names s1 but belongs to s2"
	peers := &fakePeers{events: map[string][]Event{
		"belt": {
			entry(60, "s1", map[string]string{"hint": "kof-consult", "assertions": "id1"}),
			mention,
			entry(20, "s1", map[string]string{"hint": "prefer-csl"}),
			older,
		},
		"belt-ledger": {
			entry(50, "s1", map[string]string{"event": "pre-compact"}),
			entry(40, "s1", map[string]string{"event": "subagent-start"}),
			older,
		},
	}}
	got := ReadEvents(context.Background(), peers, "s1", t0)
	if got.Source.Status != StatusOK {
		t.Fatalf("source = %+v, want ok", got.Source)
	}
	var hints []string
	for _, h := range got.Hints {
		hints = append(hints, h.Hint)
	}
	if !slices.Equal(hints, []string{"prefer-csl", "kof-consult"}) {
		t.Errorf("hints = %v, want s1's two in time order", hints)
	}
	if got.Ledger == nil || *got.Ledger != (Ledger{SubagentStart: 1, PreCompact: 1}) {
		t.Errorf("ledger = %+v", got.Ledger)
	}

	// Without an entry older than the session's start, the early ones may
	// have been dropped: the source is partial and says which.
	peers.events["belt-ledger"] = peers.events["belt-ledger"][:2]
	got = ReadEvents(context.Background(), peers, "s1", t0)
	if got.Source.Status != StatusPartial || !strings.Contains(got.Source.Detail, "belt-ledger") ||
		strings.Contains(got.Source.Detail, "belt and") {
		t.Errorf("source = %+v, want partial naming belt-ledger only", got.Source)
	}
	if len(got.Hints) != 2 || got.Ledger == nil {
		t.Errorf("a partial source still answers: %+v", got)
	}
}

func TestReadEventsUnavailable(t *testing.T) {
	for name, p := range map[string]Peers{
		"no peer":      nil,
		"service down": &fakePeers{eventsErr: errors.New("events: connection refused")},
	} {
		t.Run(name, func(t *testing.T) {
			got := ReadEvents(context.Background(), p, "s1", t0)
			if got.Source.Status != StatusUnavailable || got.Source.Detail == "" ||
				got.Hints != nil || got.Ledger != nil {
				t.Errorf("got %+v, want unavailable with no hints or ledger", got)
			}
		})
	}
}

func TestReadEventsNoTurnsSkipsTheProbe(t *testing.T) {
	peers := &fakePeers{events: map[string][]Event{}}
	got := ReadEvents(context.Background(), peers, "s1", time.Time{})
	if got.Source.Status != StatusOK || got.Hints == nil {
		t.Errorf("got %+v, want ok with an empty hint list", got)
	}
}

func TestReadPins(t *testing.T) {
	pin := Pin{Repo: "github.com/acme/repo", File: "services/x/a.go"}
	hints := []HintEvent{
		{Hint: "kof-consult", Assertions: []string{"id1", "id2"}},
		{Hint: "kof-assertions", Assertions: []string{"id1", "id3"}},
		{
			Hint: "kof-assertions", Assertions: []string{"tagged"},
			Pins: map[string][]Pin{"tagged": {{File: "b.go"}}},
		},
	}
	held := []Assertion{
		{ID: "id1", Pins: []Pin{pin}}, {ID: "id2"}, {ID: "id3"}, {ID: "unshown"},
	}
	tests := []struct {
		name         string
		peers        *fakePeers
		hints        []HintEvent
		status       Status
		detail       string // a part of the detail
		wantPins     []string
		wantListings int
	}{
		{
			name:  "every assertion found in one listing",
			peers: &fakePeers{assertions: held}, hints: hints,
			status: StatusOK, wantPins: []string{"id1", "id2", "id3"}, wantListings: 1,
		},
		{
			name:  "one assertion gone",
			peers: &fakePeers{assertions: []Assertion{held[0], held[2]}}, hints: hints,
			status: StatusPartial, detail: "no assertion id2",
			wantPins: []string{"id1", "id3"}, wantListings: 1,
		},
		{
			name:  "kof down",
			peers: &fakePeers{kofErr: errors.New("kof: connection refused")}, hints: hints,
			status: StatusUnavailable, detail: "connection refused", wantListings: 1,
		},
		{
			name:   "no kof hint",
			peers:  &fakePeers{assertions: held},
			hints:  []HintEvent{{Hint: "prefer-csl"}, hints[2]},
			status: StatusSkipped, detail: "no assertion to look up",
		},
		{
			name:   "events did not answer",
			peers:  &fakePeers{assertions: held},
			status: StatusSkipped, detail: "the events service did not answer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pins, src := ReadPins(context.Background(), tt.peers, tt.hints)
			if src.Status != tt.status || !strings.Contains(src.Detail, tt.detail) {
				t.Errorf("source = %+v, want %s with %q", src, tt.status, tt.detail)
			}
			var ids []string
			for id := range pins {
				ids = append(ids, id)
			}
			slices.Sort(ids)
			if !slices.Equal(ids, tt.wantPins) || tt.peers.listings != tt.wantListings {
				t.Errorf("pins for %v after %d listings, want %v after %d",
					ids, tt.peers.listings, tt.wantPins, tt.wantListings)
			}
		})
	}
}
