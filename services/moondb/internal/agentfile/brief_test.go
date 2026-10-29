package agentfile

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb"
)

// TestReadBrief: the brief is the first user line's text, without the
// teammate wrapper the harness put around it, and whole: the agent
// transcript's head cut does not reach it.
func TestReadBrief(t *testing.T) {
	c, err := Read(context.Background(), workerFile, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.Brief, "Build the thing. é") ||
		strings.Contains(c.Brief, "teammate-message") ||
		len(c.Brief) <= moondb.AgentHeadBytes {
		t.Errorf(
			"Brief = %.80q (%d bytes), want the unwrapped message whole",
			c.Brief,
			len(c.Brief),
		)
	}
}

// TestBriefOf covers the shapes a first user line comes in.
func TestBriefOf(t *testing.T) {
	long := strings.Repeat("x", moondb.MaxBriefBytes+10)
	cases := []struct {
		name    string
		content any
		want    string
		more    int
	}{
		{"plain prompt", "Review the diff.\n", "Review the diff.", 0},
		{
			"teammate wrapper",
			"<teammate-message teammate_id=\"team-lead\" summary=\"s\">\nFix it.\n</teammate-message>",
			"Fix it.", 0,
		},
		{
			"a '>' inside the summary",
			"<teammate-message teammate_id=\"team-lead\" summary=\"a > b\">\nFix it.\n</teammate-message>",
			"Fix it.", 0,
		},
		{
			"two envelopes in the line",
			"<teammate-message teammate_id=\"team-lead\" summary=\"one\">\nFix it.\n</teammate-message>\n\n" +
				"<teammate-message teammate_id=\"team-lead\" summary=\"two\">\nAlso this.\n</teammate-message>",
			"Fix it.", 1,
		},
		{
			"fork directive after its spawn's result",
			[]map[string]any{
				{"type": "tool_result", "tool_use_id": "t1", "content": "Fork started"},
				{
					"type": "text",
					"text": "<fork-boilerplate>\nrules\n</fork-boilerplate>\n\nYour directive: build",
				},
			},
			"Your directive: build", 0,
		},
		{
			"text blocks joined",
			[]map[string]any{{"type": "text", "text": "one"}, {"type": "text", "text": "two"}},
			"one\n\ntwo", 0,
		},
		{"tool result only", []map[string]any{{"type": "tool_result", "content": "ok"}}, "", 0},
		{"over the cap", long, long[:moondb.MaxBriefBytes] + "...[+10 bytes]", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, more := briefOf(userLine(t, tc.content))
			if got != tc.want || more != tc.more {
				t.Errorf("briefOf = %.120q, %d; want %.120q, %d", got, more, tc.want, tc.more)
			}
		})
	}
}

// TestLaterMessages counts the messages that reach an agent after its
// spawn brief: envelopes a later user line opens with, two in one line
// among them, and not a tool result, a reminder or a compaction summary
// that quotes one.
func TestLaterMessages(t *testing.T) {
	env := func(summary, body string) string {
		return `<teammate-message teammate_id=\"team-lead\" summary=\"` + summary + `\">\n` +
			body + `\n</teammate-message>`
	}
	user := func(content string) string {
		return `{"type":"user","timestamp":"2026-10-25T10:00:00Z","message":{"content":` + content + `}}`
	}
	toolResult := `[{"type":"tool_result","tool_use_id":"t1","content":"` + env("hit", "x") + `"}]`
	twoInOne := `"` + env("a > b", "Tighten it.") + `\n\n` + env("second", "Land.") + `"`
	lines := []string{
		user(`"` + env("spawn", "Build it.") + `"`),
		user(toolResult),
		user(`"<system-reminder>\nagents\n</system-reminder>"`),
		user(twoInOne),
		user(`"This session is being continued. ` + env("quoted", "old") + `"`),
		user(`[{"type":"text","text":"` + env("third", "Stop.") + `"}]`),
	}
	c, err := readLines(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Brief != "Build it." || c.LaterMessages != 3 {
		t.Errorf("brief %q, later messages %d; want %q, 3", c.Brief, c.LaterMessages, "Build it.")
	}
}

// TestBriefIsTheFirstUserLine: a later user line never stands in for a
// first one that held no text.
func TestBriefIsTheFirstUserLine(t *testing.T) {
	first := `{"type":"user","timestamp":"2026-10-25T10:00:00Z","message":{"content":[` +
		`{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`
	second := `{"type":"user","timestamp":"2026-10-25T10:00:01Z","message":{"content":"later"}}`
	c, err := readLines(context.Background(), strings.NewReader(first+"\n"+second+"\n"))
	if err != nil || c.Brief != "" {
		t.Errorf("readLines = brief %q, %v; want none", c.Brief, err)
	}
}

// userLine is a decoded user transcript line whose message holds content.
func userLine(t *testing.T, content any) fields {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"type": "user", "message": map[string]any{
		"role": "user", "content": content,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var l fields
	if err := json.Unmarshal(raw, &l); err != nil {
		t.Fatal(err)
	}
	return l
}
