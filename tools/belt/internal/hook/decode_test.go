package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
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
	reason := out.PermissionDecisionReason
	if !strings.HasPrefix(reason, "belt[payload]:") {
		t.Errorf("deny reason %q does not start with belt[payload]:", reason)
	}
	for _, want := range []string{
		"could not read the hook payload", "the harness builds the payload", wantErr,
	} {
		if !strings.Contains(reason, want) {
			t.Errorf("deny reason %q does not mention %q", reason, want)
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

// TestRunNamesThePayloadWhenTheConfigIsBrokenToo: with both the payload and
// the config unreadable, the deny is belt[payload]. belt checks the payload
// first, and only a payload it can read goes on to the config load.
func TestRunNamesThePayloadWhenTheConfigIsBrokenToo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("guards: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	load := func() (config.Config, error) {
		return config.LoadFrom(config.Paths{BeltYAML: path})
	}
	if _, err := load(); err == nil {
		t.Fatalf("config %s loaded, want a parse error", path)
	}

	var out bytes.Buffer
	Run("bash", load, strings.NewReader("not json"), &out)

	var d decision
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatalf("decode decision from %q: %v", out.String(), err)
	}
	reason := d.HookSpecificOutput.PermissionDecisionReason
	if d.HookSpecificOutput.PermissionDecision != "deny" ||
		!strings.HasPrefix(reason, "belt[payload]:") {
		t.Errorf("decision = %+v, want a belt[payload] deny", d.HookSpecificOutput)
	}
}

// TestRunAllowsExternalTextWithoutAToolInput: external-text reads no
// tool_input key as a string, so an absent or null tool_input passes the
// payload check and reaches the guards, which find no text to publish and
// allow. The load count shows the run got past the payload check. The
// control case shows the guard is live under this config: the same call
// with a blocked name in it is denied by publish-internal-names.
func TestRunAllowsExternalTextWithoutAToolInput(t *testing.T) {
	cfg := config.Config{Names: config.InternalNames{BlockedWords: []string{"internalco"}}}
	tests := []struct {
		name, input, wantGuard string
	}{
		{"tool_input absent", "", ""},
		{"tool_input null", `,"tool_input":null`, ""},
		{
			"control: a blocked name in the input",
			`,"tool_input":{"description":"internalco notes"}`,
			guard.PublishInternalNamesID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loads := 0
			load := func() (config.Config, error) {
				loads++
				return cfg, nil
			}
			raw := `{"tool_name":"mcp__gh_com__create_gist"` + tt.input + `}`
			var out bytes.Buffer
			var a archived
			runner{load: load, emit: a.emit}.guards("external-text", strings.NewReader(raw), &out)

			if loads != 1 {
				t.Fatalf("config loaded %d times, want 1: the run never reached the guards", loads)
			}
			if tt.wantGuard == "" {
				if out.Len() != 0 || len(a.events) != 0 {
					t.Errorf("wrote %q and archived %+v, want a silent allow",
						out.String(), a.events)
				}
				return
			}
			var d decision
			if err := json.Unmarshal(out.Bytes(), &d); err != nil {
				t.Fatalf("decode decision from %q: %v", out.String(), err)
			}
			if reason := d.HookSpecificOutput.PermissionDecisionReason; !strings.HasPrefix(
				reason, "belt["+tt.wantGuard+"]:",
			) {
				t.Errorf("deny reason = %q, want it from %s", reason, tt.wantGuard)
			}
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
		{
			"numeric read file_path", "read",
			`{"tool_name":"Read","tool_input":{"file_path":7}}`,
			"field tool_input.file_path is a JSON number, want a string",
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

// TestRunHintWritesNothingOnAMalformedPayload pins the exported hint
// entrypoint's wiring: the payload check comes before the config load. With
// a config belt cannot read, a readable payload gets belt[config] advice, so
// no output at all means the payload check stopped the run first. The warn
// event that run leaves is pinned on the runner above
// (TestRunHintStaysSilentOnAnUnreadablePayload): the exported entrypoint
// posts through notify, which sends nothing under go test.
func TestRunHintWritesNothingOnAMalformedPayload(t *testing.T) {
	var out bytes.Buffer
	RunHint("search", failing, strings.NewReader("not json"), &out)
	if out.Len() != 0 {
		t.Errorf("RunHint wrote %q on a malformed payload, want nothing", out.String())
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

// TestRecordDropsAMistypedSessionIDSilently pins what the lenient session
// fields cost the ledger: recordPayload embeds payload, so a numeric
// session_id or an object transcript_path is dropped from the entry, which
// posts at info with no session and no payload warning.
func TestRecordDropsAMistypedSessionIDSilently(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var posted []notify.Event
	rc := recorder{
		load:  defaults,
		build: buildEntry,
		emit:  func(ev notify.Event) { posted = append(posted, ev) },
	}
	rc.run(
		ledger.EventSessionEnd,
		strings.NewReader(`{"session_id":7,"transcript_path":{"p":"/t"},"reason":"clear"}`),
	)
	if len(posted) != 1 {
		t.Fatalf("posted %d entries, want 1", len(posted))
	}
	ev := posted[0]
	if ev.Level != "info" || strings.HasPrefix(ev.Message, "hook payload unreadable") {
		t.Errorf("entry = level %q, message %q, want info with no payload warning",
			ev.Level, ev.Message)
	}
	for _, tag := range []string{"session_id", "transcript"} {
		if v, ok := ev.Tags[tag]; ok {
			t.Errorf("%s tag = %q, want none for a mistyped field", tag, v)
		}
	}
	if ev.Tags["reason"] != "clear" {
		t.Errorf("tags = %v, want the fields beside the mistyped ones kept", ev.Tags)
	}
}
