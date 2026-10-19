package hook

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/ledger"
)

// archived captures what a runner would post to the events service.
type archived struct{ events []notify.Event }

func (a *archived) emit(source, level, title, message string, tags map[string]string) {
	a.events = append(a.events, notify.Event{
		Source: source, Level: level, Title: title, Message: message, Tags: tags,
	})
}

// runGuardsOn runs the guard entrypoint with no config file on raw and
// returns the decision it wrote and the events it would archive.
func runGuardsOn(t *testing.T, event, raw string) (decision, []notify.Event) {
	t.Helper()
	var out bytes.Buffer
	var a archived
	runner{load: defaults, emit: a.emit}.guards(event, strings.NewReader(raw), &out)
	var d decision
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatalf("decode decision from %q: %v", out.String(), err)
	}
	return d, a.events
}

// wantPayloadDeny checks a run denied with belt[payload], naming the decode
// error, and archived one warn event tagged with the event and that error.
func wantPayloadDeny(t *testing.T, event, wantErr string, d decision, events []notify.Event) {
	t.Helper()
	out := d.HookSpecificOutput
	if out.PermissionDecision != "deny" {
		t.Errorf("permissionDecision = %q, want deny", out.PermissionDecision)
	}
	for _, want := range []string{"belt[payload]:", "could not read the hook payload", wantErr} {
		if !strings.Contains(out.PermissionDecisionReason, want) {
			t.Errorf("deny reason %q does not mention %q", out.PermissionDecisionReason, want)
		}
	}
	if len(events) != 1 {
		t.Fatalf("archived %d events, want 1: %+v", len(events), events)
	}
	ev := events[0]
	if ev.Source != "belt" || ev.Level != "warn" || ev.Tags["event"] != event ||
		ev.Tags["guard"] != PayloadDenyID || !strings.Contains(ev.Tags[decodeErrorTag], wantErr) {
		t.Errorf("archived %+v, want a belt warn tagged event %s and decode_error %q",
			ev, event, wantErr)
	}
}

// TestRunDeniesAPayloadThatIsNotJSON pins the fail-closed payload rule: a
// payload belt cannot read is denied with belt[payload], never allowed
// silently, because no guard can check a call it cannot see.
func TestRunDeniesAPayloadThatIsNotJSON(t *testing.T) {
	tests := []struct {
		name, raw, wantErr string
	}{
		{"not json", "not json", "not JSON: invalid character"},
		{
			"truncated",
			`{"tool_name":"Bash","tool_input":{"command":"git push`,
			"not JSON: unexpected EOF",
		},
		{"empty stdin", "", "empty payload"},
		{"json null", "null", "not a JSON object"},
		{"json array", `[{"tool_name":"Bash"}]`, "not a JSON object"},
		{"json string", `"git push origin main"`, "not a JSON object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, events := runGuardsOn(t, "bash", tt.raw)
			wantPayloadDeny(t, "bash", tt.wantErr, d, events)
		})
	}
}

// TestRunDeniesAMistypedCoreField: a field the guards read that arrives
// with another JSON type would reach them as "" and let them allow a call
// they never saw. Every such field, top level and per event, denies.
func TestRunDeniesAMistypedCoreField(t *testing.T) {
	tests := []struct {
		name, event, raw, wantErr string
	}{
		{
			"numeric command", "bash",
			`{"tool_name":"Bash","tool_input":{"command":7},"cwd":"/x"}`,
			"field tool_input.command is a JSON number, want a string",
		},
		{
			"array command", "bash",
			`{"tool_name":"Bash","tool_input":{"command":["git","push","origin","main"]}}`,
			"field tool_input.command is a JSON array, want a string",
		},
		{
			"numeric cwd", "bash",
			`{"tool_name":"Bash","tool_input":{"command":"ls"},"cwd":7}`,
			"field cwd is a JSON number, want a string",
		},
		{
			"object tool_name", "bash",
			`{"tool_name":{"n":"Bash"},"tool_input":{"command":"ls"}}`,
			"field tool_name is a JSON object, want a string",
		},
		{
			"string tool_input", "bash",
			`{"tool_name":"Bash","tool_input":"git push origin main"}`,
			"field tool_input is a JSON string, want an object",
		},
		{
			"numeric file_path", "write",
			`{"tool_name":"Write","tool_input":{"file_path":7,"content":"x"}}`,
			"field tool_input.file_path is a JSON number, want a string",
		},
		{
			"array content", "write",
			`{"tool_name":"Write","tool_input":{"file_path":"/a.md","content":["x"]}}`,
			"field tool_input.content is a JSON array, want a string",
		},
		{
			"bool new_string", "write",
			`{"tool_name":"Edit","tool_input":{"file_path":"/a.md","new_string":true}}`,
			"field tool_input.new_string is a JSON bool, want a string",
		},
		{
			"array tool_input on external-text", "external-text",
			`{"tool_name":"mcp__gh_com__create_pull_request","tool_input":["body"]}`,
			"field tool_input is a JSON array, want an object",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, events := runGuardsOn(t, tt.event, tt.raw)
			wantPayloadDeny(t, tt.event, tt.wantErr, d, events)
		})
	}
}

// TestPayloadDenyCarriesTheIdentity: a type error still decodes the fields
// beside it, so the warning names the session and agent that made the call.
func TestPayloadDenyCarriesTheIdentity(t *testing.T) {
	raw := `{"tool_name":"Bash","tool_input":{"command":7},` +
		`"session_id":"s1","agent_id":"a1","agent_type":"Explore"}`
	_, events := runGuardsOn(t, "bash", raw)
	if len(events) != 1 {
		t.Fatalf("archived %d events, want 1", len(events))
	}
	tags := events[0].Tags
	if tags["session_id"] != "s1" || tags["agent_id"] != "a1" || tags["agent_type"] != "Explore" {
		t.Errorf("tags = %v, want the session and agent identity", tags)
	}
}

// TestDecodePayloadToleratesWhatNoEntrypointReads: null counts as absent,
// the identity and session fields decode leniently, and tool_input keys no
// input mapping reads may carry any type. None of these fails the decode.
func TestDecodePayloadToleratesWhatNoEntrypointReads(t *testing.T) {
	tests := []struct {
		name, raw string
		fields    []string
	}{
		{"null cwd", `{"tool_input":{"command":"ls"},"cwd":null}`, guardInputFields["bash"]},
		{"null command", `{"tool_input":{"command":null}}`, guardInputFields["bash"]},
		{"null tool_input", `{"tool_input":null}`, guardInputFields["write"]},
		{"numeric session_id", `{"session_id":7}`, guardInputFields["bash"]},
		{"object transcript_path", `{"transcript_path":{"p":"/t"}}`, nil},
		{
			"unread tool_input key",
			`{"tool_input":{"command":"ls","timeout":600}}`,
			guardInputFields["bash"],
		},
		{
			"any shape on external-text",
			`{"tool_input":{"body":7,"labels":[1]}}`,
			guardInputFields["external-text"],
		},
		{"unknown top-level field", `{"permission_mode":{"x":1}}`, guardInputFields["bash"]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodePayload(strings.NewReader(tt.raw), tt.fields); err != nil {
				t.Errorf("decodePayload(%s) error = %v, want none", tt.raw, err)
			}
		})
	}
}

// TestRunHintStaysSilentOnAnUnreadablePayload: a hint has no denial path,
// so an unreadable payload writes nothing and leaves a warn event instead.
func TestRunHintStaysSilentOnAnUnreadablePayload(t *testing.T) {
	tests := []struct {
		name, event, raw, wantErr string
	}{
		{"not json", "search", "not json", "not JSON"},
		{"empty stdin", "session-start", "", "empty payload"},
		{
			"numeric bash command", "bash",
			`{"tool_name":"Bash","tool_input":{"command":7}}`,
			"field tool_input.command is a JSON number, want a string",
		},
		{
			"numeric search repo", "search",
			`{"tool_name":"mcp__csl__csl_search","tool_input":{"repo":7}}`,
			"field tool_input.repo is a JSON number, want a string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			var a archived
			runner{load: defaults, emit: a.emit}.hints(tt.event, strings.NewReader(tt.raw), &out)
			if out.Len() != 0 {
				t.Errorf("hint wrote %q on an unreadable payload, want nothing", out.String())
			}
			if len(a.events) != 1 {
				t.Fatalf("archived %d events, want 1: %+v", len(a.events), a.events)
			}
			ev := a.events[0]
			if ev.Source != "belt" || ev.Level != "warn" || ev.Tags["event"] != tt.event ||
				!strings.Contains(ev.Tags[decodeErrorTag], tt.wantErr) {
				t.Errorf("archived %+v, want a belt warn tagged event %s and decode_error %q",
					ev, tt.event, tt.wantErr)
			}
		})
	}
}

// TestRecordLeavesAWarnEntryForAnUnreadablePayload: the record path cannot
// deny or print, so its trace of a payload it could not read is the ledger
// entry itself, posted at warn and naming the decode error.
func TestRecordLeavesAWarnEntryForAnUnreadablePayload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var posted []notify.Event
	rc := recorder{
		load:  defaults,
		build: buildEntry,
		emit:  func(ev notify.Event) { posted = append(posted, ev) },
	}
	got := captureStdio(t, func() {
		rc.run(ledger.EventSubagentStop, strings.NewReader("not json"))
	})
	if got != "" {
		t.Errorf("record wrote %q, want nothing", got)
	}
	if len(posted) != 1 {
		t.Fatalf("posted %d entries, want 1", len(posted))
	}
	if ev := posted[0]; ev.Level != "warn" ||
		!strings.HasPrefix(ev.Message, "hook payload unreadable: ") {
		t.Errorf("entry = level %q, message %q, want a warn naming the decode error",
			ev.Level, ev.Message)
	}
}
