package hook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

func TestToInputRead(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]any
		ranged bool
	}{
		{"whole file", map[string]any{"file_path": "/repo/a.go"}, false},
		{"offset", map[string]any{"file_path": "/repo/a.go", "offset": float64(10)}, true},
		{"limit", map[string]any{"file_path": "/repo/a.go", "limit": float64(50)}, true},
		{
			"null bounds",
			map[string]any{"file_path": "/repo/a.go", "offset": nil, "limit": nil},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := toInput(guard.EventRead, payload{
				ToolName:       "Read",
				ToolInput:      tt.input,
				TranscriptPath: "/t/s1.jsonl",
				AgentID:        "a1",
			})
			if in.FilePath != "/repo/a.go" || in.Ranged != tt.ranged ||
				in.TranscriptPath != "/t/s1.jsonl" || in.AgentID != "a1" {
				t.Errorf(
					"toInput = %+v, want file /repo/a.go, ranged %v, the transcript and agent",
					in,
					tt.ranged,
				)
			}
		})
	}
}

// TestRunReadDeniesAWholeReadOfALargeIndexedFile drives the read event
// through the guard entrypoint against a hermetic csl index: a whole-file
// Read is denied with belt[large-read] and archived as a warn event, and
// the same Read with a limit writes nothing.
func TestRunReadDeniesAWholeReadOfALargeIndexedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "code", "acme", "widget")
	file := filepath.Join(root, "big.go")
	transcript := filepath.Join(home, "s1.jsonl")
	shard := filepath.Join(home, ".config", "csl", "search-index", "acme%2Fwidget_v16.00000.zoekt")
	for path, content := range map[string]string{
		shard:                               "",
		filepath.Join(root, ".git", "HEAD"): "",
		file:                                strings.Repeat("line\n", 500),
		transcript:                          "",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(extra string) string {
		return fmt.Sprintf(
			`{"session_id":"s1","transcript_path":%q,"tool_name":"Read","tool_input":{"file_path":%q%s}}`,
			transcript,
			file,
			extra,
		)
	}

	var out bytes.Buffer
	var events []string
	rn := runner{load: defaults, emit: func(_, level, title, _ string, _ map[string]string) {
		events = append(events, level+" "+title)
	}}
	rn.guards(guard.EventRead, strings.NewReader(read("")), &out)
	var d decision
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatalf("decode decision from %q: %v", out.String(), err)
	}
	if d.HookSpecificOutput.PermissionDecision != "deny" ||
		!strings.HasPrefix(d.HookSpecificOutput.PermissionDecisionReason, "belt[large-read]: ") {
		t.Errorf("decision = %+v, want a belt[large-read] deny", d.HookSpecificOutput)
	}
	if len(events) != 1 || events[0] != "warn blocked Read (large-read)" {
		t.Errorf("events = %q, want one warn naming the guard", events)
	}

	out.Reset()
	rn.guards(guard.EventRead, strings.NewReader(read(`,"limit":200`)), &out)
	if out.Len() != 0 {
		t.Errorf("ranged read wrote %q, want nothing", out.String())
	}
}
