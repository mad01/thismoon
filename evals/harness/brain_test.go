package main

import (
	"context"
	"strings"
	"testing"
)

func TestBrainCalls(t *testing.T) {
	tests := []struct {
		command string
		want    string // subcommand labels, comma separated
	}{
		{"brain search foo", "search"},
		{"brain --help", "--help"},
		{"brain write --help", "write --help"},
		{"brain search -h", "search --help"},
		{"brain", noSubcommand},
		{"/tmp/bin/brain read notes/x.md", "read"},
		{"BRAIN_CONFIG=/x brain list notes", "list"},
		{"brain search a && brain read b.md", "search,read"},
		{"brain search 'a|b' | head -5", "search"},
		{`brain write --title "T" --body "x; brain is fine"`, "write"},
		{"for f in a b; do brain read $f; done", "read"},
		{"echo $(brain count x)", "count"},
		{"time brain search x", "search"},
		{"ls ~/brain-data", ""},
		{"cat brainstorm.md", ""},
		{"brain-data list", ""},
		{"brain search x\nbrain read y", "search,read"},
	}
	for _, tc := range tests {
		t.Run(tc.command, func(t *testing.T) {
			var got []string
			for _, c := range brainCalls(tc.command) {
				got = append(got, c.SubcommandLabel())
			}
			if strings.Join(got, ",") != tc.want {
				t.Errorf("got %v, want %s", got, tc.want)
			}
		})
	}
}

// brainStream has a brain search, a Read, a denied loop of brain reads and
// a subagent's brain list.
const brainStream = `{"type":"system","subtype":"init","session_id":"s1","model":"m","claude_code_version":"2.1.0"}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"description":"search","command":"brain search 'ledger' --tier notes"}}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"/x"}}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t9","name":"Bash","input":{"command":"for f in a b; do brain read $f; done"}}]}}
{"type":"assistant","parent_tool_use_id":"t0","message":{"content":[{"type":"tool_use","id":"t5","name":"Bash","input":{"command":"brain list notes"}}]}}
{"type":"result","subtype":"success","result":"Port 9090.","num_turns":3,"permission_denials":[{"tool_name":"Bash","tool_use_id":"t9"}]}
`

func TestBrainCallsInTranscript(t *testing.T) {
	tr, err := parseStream(strings.NewReader(brainStream))
	if err != nil {
		t.Fatal(err)
	}
	if tr.PermissionDenials != 1 || tr.ToolCalls[0].Denied || !tr.ToolCalls[2].Denied {
		t.Fatalf("only the loop should be denied: %+v", tr.ToolCalls)
	}
	if got := formatCounts(brainCommandCounts(brainCallsIn(tr.ToolCalls))); got != "list 1, search 1" {
		t.Errorf("all brain calls = %q, want the denied loop left out", got)
	}
	if got := brainCallsIn(tr.Calls(false)); len(got) != 1 || got[0].Subcommand != "search" {
		t.Errorf("main brain calls = %+v", got)
	}
	if brainCommandCounts(nil) != nil {
		t.Error("no brain calls should count as nil")
	}
}

func TestBrainCmdGrader(t *testing.T) {
	tr, err := parseStream(strings.NewReader(brainStream))
	if err != nil {
		t.Fatal(err)
	}
	help := Transcript{ToolCalls: []ToolCall{
		{Name: "Bash", Input: `{"command":"brain write --help"}`, Origin: originMain},
		{Name: "Bash", Input: `{"command":"brain help"}`, Origin: originMain},
	}}
	tests := []struct {
		name string
		g    Grader
		tr   Transcript
		want bool
	}{
		{"search ran", Grader{Pattern: `^search$`}, tr, true},
		{"denied read does not count", Grader{Pattern: `^read$`}, tr, false},
		{"subagent list needs include_agents", Grader{Pattern: `^list$`}, tr, false},
		{"subagent list with agents", Grader{Pattern: `^list$`, IncludeAgents: true}, tr, true},
		{"max 0 forbids", Grader{Pattern: `^search$`, Max: num(0.0)}, tr, false},
		{"max 0 passes when absent", Grader{Pattern: `^promote$`, Max: num(0.0)}, tr, true},
		{"min 2", Grader{Pattern: `^(search|read)$`, Min: num(2)}, tr, false},
		{"help lookups skipped", Grader{Pattern: `write|help`}, help, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.g.Type = gradeBrainCmd
			res, _ := evaluate(
				context.Background(),
				grader(t, tc.g, workspaceBrain),
				Evidence{Transcript: tc.tr},
				nil,
			)
			if res.Pass != tc.want {
				t.Errorf("pass = %v (%s), want %v", res.Pass, res.Detail, tc.want)
			}
		})
	}
}
