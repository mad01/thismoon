package store_test

import (
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

func TestNeedsBrief(t *testing.T) {
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
		{"internal agent", func(a *store.Agent) { a.Type = store.InternalAgent }, false},
		{"no line read", func(a *store.Agent) { a.Lines = store.AgentLines{} }, false},
		{"ended before since", func(a *store.Agent) { a.EndedAt = since.Add(-time.Second) }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := base
			c.change(&a)
			if got := store.NeedsBrief(a, since); got != c.want {
				t.Errorf("NeedsBrief = %v, want %v", got, c.want)
			}
		})
	}
}
