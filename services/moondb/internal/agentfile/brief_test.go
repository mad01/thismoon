package agentfile

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

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
			if got != tc.want || len(more) != tc.more {
				t.Errorf("briefOf = %.120q, %d more; want %.120q, %d", got, len(more), tc.want,
					tc.more)
			}
		})
	}
}

// TestLaterMessages keeps the messages that reach an agent after its
// spawn brief, each with its time and sender: envelopes a later user line
// opens with, two in one line among them, a line a person typed, and the
// envelope after the brief in the brief's own line. A tool result, a
// meta line, a reminder, an interrupt marker and a compaction summary
// that quotes an envelope are none.
func TestLaterMessages(t *testing.T) {
	env := func(summary, body string) string {
		return `<teammate-message teammate_id=\"team-lead\" summary=\"` + summary + `\">\n` +
			body + `\n</teammate-message>`
	}
	user := func(at, content string, extra ...string) string {
		return `{"type":"user","timestamp":"2026-10-25T10:00:` + at + `Z",` +
			strings.Join(append(extra, ""), ",") + `"message":{"content":` + content + `}}`
	}
	toolResult := `[{"type":"tool_result","tool_use_id":"t1","content":"` + env("hit", "x") + `"}]`
	twoInOne := `"` + env("a > b", "Tighten it.") + `\n\n` + env("second", "Land.") + `"`
	spawn := `"` + env("spawn", "Build it.") + `\n\n` + env("queued", "Also docs.") + `"`
	lines := []string{
		user("00", spawn),
		user("01", toolResult),
		user("02", `"<system-reminder>\nagents\n</system-reminder>"`),
		user("03", `"Base directory for this skill"`, `"isMeta":true`),
		user("04", twoInOne),
		user("05", `"This session is being continued from a previous conversation. `+
			env("quoted", "old")+`"`, `"isCompactSummary":true`),
		user("06", `[{"type":"text","text":"`+env("third", "Stop.")+`"}]`),
		user("07", `"[Request interrupted by user]"`),
		user("08", `"leave the rules out"`),
	}
	c, err := readLines(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Brief != "Build it." {
		t.Errorf("brief %q, want %q", c.Brief, "Build it.")
	}
	at := func(sec int) time.Time { return time.Date(2026, 10, 25, 10, 0, sec, 0, time.UTC) }
	want := []Message{
		{At: at(0), From: "team-lead", Text: "Also docs."},
		{At: at(4), From: "team-lead", Text: "Tighten it."},
		{At: at(4), From: "team-lead", Text: "Land."},
		{At: at(6), From: "team-lead", Text: "Stop."},
		{At: at(8), Text: "leave the rules out"},
	}
	if !slices.EqualFunc(c.Messages, want, func(a, b Message) bool {
		return a.At.Equal(b.At) && a.From == b.From && a.Text == b.Text
	}) {
		t.Errorf("messages = %+v\nwant %+v", c.Messages, want)
	}
}

// TestSender reads the teammate_id among an envelope's attributes, not
// one a summary quotes.
func TestSender(t *testing.T) {
	cases := []struct{ attrs, want string }{
		{` teammate_id="team-lead" summary="s"`, "team-lead"},
		{` summary="teammate_id=x" teammate_id="fix-337"`, "fix-337"},
		{` summary="no sender"`, ""},
	}
	for _, c := range cases {
		if got := sender(c.attrs); got != c.want {
			t.Errorf("sender(%q) = %q, want %q", c.attrs, got, c.want)
		}
	}
}

// The lines of TestQueuedMessages, in the shapes Claude Code 2.1.306 and
// 2.1.270 wrote them with their text replaced: a parent's SendMessage to a
// running subagent, queued and shown to it at its next model call, comes
// as an attachment line a few milliseconds after the round's tool result,
// and so does a line a person typed into the agent's busy pane. The
// second tool call of the first response comes after the queued line.
const (
	queuedSpawn = `{"type":"user","timestamp":"2026-10-21T12:00:00.000Z",` +
		`"message":{"role":"user","content":"Build the scaffold."}}`
	queuedCallA = `{"type":"assistant","timestamp":"2026-10-21T12:22:48.790Z",` +
		`"message":{"id":"msg_1","role":"assistant","content":[` +
		`{"type":"tool_use","id":"toolu_a","name":"Bash","input":{"command":"ls"}}]}}`
	queuedResultA = `{"type":"user","timestamp":"2026-10-21T12:22:49.791Z",` +
		`"message":{"role":"user","content":[` +
		`{"type":"tool_result","tool_use_id":"toolu_a","content":"ok"}]}}`
	queuedFromParent = `{"parentUuid":"ab247e8a-6d58-4bf5-8e4d-4b33018780ae","isSidechain":true,` +
		`"agentId":"a542846d3bf4a5bee","attachment":{"type":"queued_command",` +
		`"prompt":"Read the reference from main instead.",` +
		`"source_uuid":"e68c3d76-1251-41ab-a04b-7c38e3f30978","origin":{"kind":"coordinator"},` +
		`"isMeta":true,"reminderId":"32a7ec589e20d36d"},"type":"attachment",` +
		`"uuid":"3560a9a6-8149-4813-a97f-68793414050d","timestamp":"2026-10-21T12:22:49.794Z",` +
		`"rendered":[{"content":"<system-reminder id=\"32a7ec589e20d36d\">\nThe coordinator sent ` +
		`a message while you were working:\nRead the reference from main instead.\n\nAddress ` +
		`this before completing your current task.\n</system-reminder id=\"32a7ec589e20d36d\">"}],` +
		`"renderedRole":"system","userType":"external","entrypoint":"cli","cwd":"/r",` +
		`"sessionId":"5f3a9a42-6a12-4367-b8f5-d25284d072f6","version":"2.1.306","gitBranch":"main"}`
	queuedTokens = `{"type":"attachment","timestamp":"2026-10-21T12:22:49.794Z",` +
		`"attachment":{"type":"total_tokens_reminder"}}`
	queuedCallB = `{"type":"assistant","timestamp":"2026-10-21T12:22:49.800Z",` +
		`"message":{"id":"msg_1","role":"assistant","content":[` +
		`{"type":"tool_use","id":"toolu_b","name":"Edit","input":{}}]}}`
	queuedNotice = `{"type":"attachment","timestamp":"2026-10-21T12:22:50.000Z",` +
		`"attachment":{"type":"queued_command","prompt":"<task-notification>\n` +
		`<task-id>b1</task-id>\n</task-notification>","commandMode":"task-notification",` +
		`"origin":{"kind":"task-notification","producer":"session-task"}}}`
	queuedCallC = `{"type":"assistant","timestamp":"2026-10-21T12:22:54.059Z",` +
		`"message":{"id":"msg_2","role":"assistant","content":[` +
		`{"type":"thinking","thinking":""},{"type":"tool_use","id":"toolu_c","name":"Write","input":{}}]}}`
	queuedFromPerson = `{"isSidechain":true,"agentId":"a6ba2876559e2bf7a","attachment":{` +
		`"type":"queued_command","prompt":"Is there a package for this?",` +
		`"source_uuid":"57694ad7-0000-4000-8000-000000000000","origin":{"kind":"human"},` +
		`"isMeta":true},"type":"attachment","timestamp":"2026-10-21T12:22:55.196Z",` +
		`"version":"2.1.270"}`
)

// TestQueuedMessages keeps a queued message from the parent and one a
// person typed as later messages, at their lines' times, each after every
// tool call of the model response it came in after; a queued task
// notification and another attachment are none.
func TestQueuedMessages(t *testing.T) {
	lines := []string{
		queuedSpawn, queuedCallA, queuedResultA, queuedFromParent, queuedTokens, queuedCallB,
		queuedNotice, queuedCallC, queuedFromPerson,
	}
	c, err := readLines(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Message{
		{
			At:   time.Date(2026, 10, 21, 12, 22, 49, 794e6, time.UTC),
			Text: "Read the reference from main instead.", AfterCalls: []string{"toolu_a", "toolu_b"},
		},
		{
			At:   time.Date(2026, 10, 21, 12, 22, 55, 196e6, time.UTC),
			Text: "Is there a package for this?", AfterCalls: []string{"toolu_c"},
		},
	}
	if !slices.EqualFunc(c.Messages, want, func(a, b Message) bool {
		return a.At.Equal(b.At) && a.From == b.From && a.Text == b.Text &&
			slices.Equal(a.AfterCalls, b.AfterCalls)
	}) {
		t.Errorf("messages = %+v\nwant %+v", c.Messages, want)
	}
	if c.Brief != "Build the scaffold." {
		t.Errorf("brief %q, want the spawn line's", c.Brief)
	}
}

// TestAgentMessages puts each message's text in a chunk and keeps its
// time, sender and the tool calls it came in after beside the key.
func TestAgentMessages(t *testing.T) {
	at := time.Date(2026, 10, 29, 20, 1, 56, 0, time.UTC)
	c := Capture{Messages: []Message{
		{At: at, From: "team-lead", Text: "Go with C.", AfterCalls: []string{"toolu_a"}},
	}}
	chunks := map[string]string{}
	got := c.AgentMessages(func(b []byte) string {
		chunks["k1"] = string(b)
		return "k1"
	})
	if len(got) != 1 || !got[0].At.Equal(at) || got[0].From != "team-lead" ||
		got[0].Chunk != "k1" || chunks["k1"] != "Go with C." ||
		!slices.Equal(got[0].AfterCalls, []string{"toolu_a"}) {
		t.Errorf("AgentMessages = %+v with chunks %v", got, chunks)
	}
	if (Capture{}).AgentMessages(nil) != nil {
		t.Error("a capture without messages gives some")
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
