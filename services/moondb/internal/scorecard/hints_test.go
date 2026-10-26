package scorecard

import (
	"testing"
	"time"
)

// TestKofStatsJoinsPinsToLaterTouches: a pinned file counts as used when a
// later call of any agent of the session read or edited it.
func TestKofStatsJoinsPinsToLaterTouches(t *testing.T) {
	pin := Pin{Repo: "github.com/acme/repo", File: "services/x/serve.go"}
	tests := []struct {
		name  string
		agent string // whose call it is
		lines []string
		used  bool
	}{
		{
			name: "read later", lines: []string{
				use(
					20,
					"r",
					"Read",
					`{"file_path":"/home/dev/code/acme/repo/services/x/serve.go"}`,
				),
				result(21, "r", str("1\tx"), false, ""),
			}, used: true,
		},
		{
			name: "read later in a worktree", lines: []string{
				use(
					20,
					"r",
					"Read",
					`{"file_path":"/home/dev/.worktrees/repo/fix-x/services/x/serve.go"}`,
				),
				result(21, "r", str("1\tx"), false, ""),
			}, used: true,
		},
		{
			name: "edited later", lines: []string{
				use(
					20,
					"e",
					"Edit",
					`{"file_path":"/w/repo/services/x/serve.go","old_string":"a","new_string":"b"}`,
				),
				result(21, "e", str("The file has been updated."), false, ""),
			}, used: true,
		},
		{
			name: "csl_read later", lines: []string{
				use(
					20,
					"c",
					"mcp__csl__csl_read",
					`{"repo":"acme/repo$","file":"services/x/serve.go"}`,
				),
				result(21, "c", texts("acme/repo/services/x/serve.go lines 1-9 of 9"), false, ""),
			}, used: true,
		},
		{
			name: "read before the hint", lines: []string{
				use(5, "r", "Read", `{"file_path":"/w/repo/services/x/serve.go"}`),
				result(6, "r", str("1\tx"), false, ""),
			},
		},
		{
			name: "same path in another repository", lines: []string{
				use(20, "r", "Read", `{"file_path":"/w/other/services/x/serve.go"}`),
				result(21, "r", str("1\tx"), false, ""),
			},
		},
		{
			name: "a longer file name", lines: []string{
				use(20, "r", "Read", `{"file_path":"/w/repo/services/x/myserve.go"}`),
				result(21, "r", str("1\tx"), false, ""),
			},
		},
		{
			name: "read by another agent", agent: "agent-2", lines: []string{
				use(20, "r", "Read", `{"file_path":"/w/repo/services/x/serve.go"}`),
				result(21, "r", str("1\tx"), false, ""),
			}, used: true,
		},
		{
			name: "edited by a subagent, its input cut to a head", agent: "agent-2", lines: []string{
				use(20, "e", "Edit", str(`{"file_path":"/w/repo/services/x/serve.go",`+
					`"old_string":"func serve() {...[+400 bytes]`)),
				result(21, "e", str("The file has been updated."), false, ""),
			}, used: true,
		},
		{
			name: "csl_read of a repository whose name shares a prefix", lines: []string{
				use(20, "c", "mcp__csl__csl_read",
					`{"repo":"acme/repo-arcade","file":"services/x/serve.go"}`),
				result(21, "c", texts("acme/repo-arcade/services/x/serve.go lines 1-9 of 9"),
					false, ""),
			},
		},
		{
			name: "csl_read naming the repository by name alone", lines: []string{
				use(20, "c", "mcp__csl__csl_read", `{"repo":"repo","file":"services/x/serve.go"}`),
				result(21, "c", texts("acme/repo/services/x/serve.go lines 1-9 of 9"), false, ""),
			},
		},
	}
	hint := HintEvent{
		Hint:       "kof-assertions",
		At:         t0.Add(10 * time.Second),
		Assertions: []string{"id1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := ParseCalls(chunk(tt.lines...), "")
			for i := range calls {
				calls[i].AgentID = tt.agent
			}
			k := kofStats([]HintEvent{hint}, map[string][]Pin{"id1": {pin}}, touched(calls))
			if len(k.Assertions) != 1 || len(k.Assertions[0].Pins) != 1 {
				t.Fatalf("assertions = %+v", k.Assertions)
			}
			a := k.Assertions[0]
			if a.Pins[0].Used != tt.used || a.Used == nil || *a.Used != tt.used {
				t.Errorf("assertion = %+v, want used %t", a, tt.used)
			}
		})
	}
}

func TestKofStatsCounts(t *testing.T) {
	calls := ParseCalls(chunk(
		use(20, "r", "Read", `{"file_path":"/w/repo/a.go"}`),
		result(21, "r", str("1\tx"), false, ""),
	), "")
	hints := []HintEvent{
		{Hint: "kof-consult", At: t0, Assertions: []string{"id1", "id2"}},
		// id1 again, to another agent: listed once, at its first showing.
		{
			Hint: "kof-assertions", At: t0.Add(30 * time.Second), AgentID: "agent-1",
			Assertions: []string{"id1", "id3"},
		},
	}
	pins := map[string][]Pin{
		// A file pinned at two line ranges counts once.
		"id1": {{File: "a.go"}, {File: "a.go"}, {File: "b.go"}},
		"id2": {{File: "c.go"}},
	}
	k := kofStats(hints, pins, touched(calls))
	if len(k.Assertions) != 3 || k.Assertions[0].Hint != "kof-consult" ||
		k.Assertions[0].AgentID != "" {
		t.Fatalf("assertions = %+v, want id1, id2, id3 with id1 from kof-consult", k.Assertions)
	}
	if k.PinnedPaths != 3 || k.UsedPaths != 1 || k.UnknownPins != 1 {
		t.Errorf("counts = %+v, want 3 pinned, 1 used, 1 unknown", k)
	}
	if k.HitRate == nil || *k.HitRate != 1.0/3 {
		t.Errorf("hit rate = %v, want 1/3", k.HitRate)
	}
	unknown := k.Assertions[2]
	if unknown.ID != "id3" || unknown.Pins != nil || unknown.Used != nil {
		t.Errorf("id3 = %+v, want null pins and used", unknown)
	}
}

// TestKofStatsPrefersKofPins: belt's pins tag holds the fire's first pins
// as bare paths, so every assertion of the fire shares them. The pins
// keeper-of-facts gave come first, and the tag serves only an assertion it
// does not hold.
func TestKofStatsPrefersKofPins(t *testing.T) {
	calls := ParseCalls(chunk(
		use(20, "r", "Read", `{"file_path":"/w/repo/c.go"}`),
		result(21, "r", str("1\tx"), false, ""),
	), "")
	shared := []Pin{{File: "a.go"}, {File: "c.go"}}
	hint := HintEvent{
		Hint: "kof-consult", At: t0, Assertions: []string{"id1", "id2", "id3"},
		Pins: map[string][]Pin{"id1": shared, "id2": shared, "id3": shared},
	}
	k := kofStats([]HintEvent{hint}, map[string][]Pin{
		"id1": {{File: "a.go"}}, "id2": {{File: "c.go"}, {File: "d.go"}},
	}, touched(calls))
	if len(k.Assertions) != 3 {
		t.Fatalf("assertions = %+v", k.Assertions)
	}
	id1, id2, id3 := k.Assertions[0], k.Assertions[1], k.Assertions[2]
	if len(id1.Pins) != 1 || id1.Pins[0].Path != "a.go" || *id1.Used {
		t.Errorf("id1 = %+v, want its own pin a.go, unused", id1)
	}
	if len(id2.Pins) != 2 || id2.Pins[0].Path != "c.go" || !*id2.Used {
		t.Errorf("id2 = %+v, want its own pins c.go and d.go, used", id2)
	}
	if len(id3.Pins) != 2 || id3.Pins[0].Path != "a.go" || !*id3.Used {
		t.Errorf("id3 = %+v, want the tag's shared pins, used", id3)
	}
	if k.PinnedPaths != 5 || k.UsedPaths != 2 || k.UnknownPins != 0 {
		t.Errorf("counts = %+v, want 5 pinned, 2 used, 0 unknown", k)
	}
}

func TestKofStatsWithoutPinsHasNoRate(t *testing.T) {
	k := kofStats([]HintEvent{{Hint: "prefer-csl"}}, nil, nil)
	if k.HitRate != nil || k.Assertions == nil || len(k.Assertions) != 0 {
		t.Errorf("kof = %+v, want an empty list and a null rate", k)
	}
}

func TestHintStats(t *testing.T) {
	if hintStats(nil, nil, nil) != nil {
		t.Error("no events answer: want null hints")
	}
	h := hintStats([]HintEvent{
		{Hint: "prefer-csl"}, {Hint: "kof-consult"}, {Hint: "prefer-csl"}, {Hint: "agent-memory"},
	}, nil, nil)
	want := []HintCount{{"prefer-csl", 2}, {"agent-memory", 1}, {"kof-consult", 1}}
	if h.Shown != 4 || len(h.ByHint) != 3 {
		t.Fatalf("hints = %+v", h)
	}
	for i := range want {
		if h.ByHint[i] != want[i] {
			t.Errorf("by hint = %+v, want %+v", h.ByHint, want)
		}
	}
}
