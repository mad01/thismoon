package store_test

import (
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

func TestNeedsFill(t *testing.T) {
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	base := store.Agent{
		ID: "a1", Type: "general-purpose", EndedAt: since.Add(time.Hour),
		Lines: store.AgentLines{Read: 12, Kept: 8},
	}
	cases := []struct {
		name   string
		change func(a *store.Agent)
		want   bool
	}{
		{"no brief, read lines, ended in the window", func(*store.Agent) {}, true},
		{"ended exactly at since", func(a *store.Agent) { a.EndedAt = since }, true},
		{"has a brief", func(a *store.Agent) { a.BriefChunk = "sha" }, false},
		{"has a brief, counted messages it did not keep", func(a *store.Agent) {
			a.BriefChunk, a.LaterMessages = "sha", 2
			a.Messages = []store.AgentMessage{{Chunk: "m1"}}
		}, true},
		{"has a brief and every message it counted", func(a *store.Agent) {
			a.BriefChunk, a.LaterMessages = "sha", 1
			a.Messages = []store.AgentMessage{{Chunk: "m1"}}
		}, false},
		{"internal agent", func(a *store.Agent) { a.Type = store.InternalAgent }, false},
		{"no line read", func(a *store.Agent) { a.Lines = store.AgentLines{} }, false},
		{"ended before since", func(a *store.Agent) { a.EndedAt = since.Add(-time.Second) }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := base
			c.change(&a)
			if got := store.NeedsFill(a, since); got != c.want {
				t.Errorf("NeedsFill = %v, want %v", got, c.want)
			}
		})
	}
}

// TestLastMessage: the last message is the latest that arrived at or
// before the time asked about, none before the first, and the later of
// two that arrived at once.
func TestLastMessage(t *testing.T) {
	t0 := time.Date(2026, 10, 29, 20, 0, 0, 0, time.UTC)
	a := store.Agent{Messages: []store.AgentMessage{
		{At: t0, From: "team-lead", Chunk: "go-with-c"},
		{At: t0.Add(time.Minute), Chunk: "typed"},
		{At: t0.Add(time.Minute), From: "team-lead", Chunk: "also"},
		{At: t0.Add(2 * time.Minute), From: "team-lead", Chunk: "land"},
	}}
	cases := []struct {
		name string
		at   time.Time
		want string // "" means none
	}{
		{"before the first", t0.Add(-time.Second), ""},
		{"at the first", t0, "go-with-c"},
		{"between", t0.Add(30 * time.Second), "go-with-c"},
		{"two at once", t0.Add(90 * time.Second), "also"},
		{"after the last", t0.Add(time.Hour), "land"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, ok := a.LastMessage(c.at)
			if got := map[bool]string{true: m.Chunk}[ok]; got != c.want {
				t.Errorf("LastMessage = %q (%v), want %q", m.Chunk, ok, c.want)
			}
		})
	}
	if _, ok := (store.Agent{}).LastMessage(t0); ok {
		t.Error("an agent with no messages has a last message")
	}
}
