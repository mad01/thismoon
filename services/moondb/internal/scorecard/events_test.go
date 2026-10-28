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

func TestParseHint(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		ok    bool
		want  HintEvent
	}{
		{
			name: "hint id from the tag",
			event: Event{Time: t0, Title: "ignored", Tags: map[string]string{
				"hint": "em-dash", "event": "edit", "session_id": "s1", "agent_id": "a1",
			}},
			ok:   true,
			want: HintEvent{Hint: "em-dash", At: t0},
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
			if got != tt.want {
				t.Errorf("hint = %+v, want %+v", got, tt.want)
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

// fakePeers answers from fixed entries, as the events service filters
// them.
type fakePeers struct {
	events    map[string][]Event // by source, newest first
	eventsErr error
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
			entry(60, "s1", map[string]string{"hint": "em-dash"}),
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
	if !slices.Equal(hints, []string{"prefer-csl", "em-dash"}) {
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
